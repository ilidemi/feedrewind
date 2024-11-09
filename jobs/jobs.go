package jobs

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"feedrewind.com/db/migrations"
	"feedrewind.com/db/pgw"
	"feedrewind.com/oops"
	"feedrewind.com/util/schedule"

	"github.com/google/uuid"
)

type JobId int64

type yamlString string

func int64ToYaml(value int64) yamlString {
	return yamlString(fmt.Sprint(value))
}

func strToYaml(str string) yamlString {
	str = strings.ReplaceAll(str, "'", "''")
	str = strings.ReplaceAll(str, "\n", "\\n")
	return yamlString(fmt.Sprintf("'%s'", str))
}

// Assumes top-level array and enmeshes indentation handling with the rest of args serialization
func int64ListToYaml(values []int64) yamlString {
	if len(values) == 0 {
		return yamlString("[]")
	}

	var b strings.Builder
	for i, value := range values {
		if i == 0 {
			fmt.Fprint(&b, "- ")
		} else {
			fmt.Fprint(&b, "\n    - ")
		}
		fmt.Fprint(&b, value)
	}
	return yamlString(b.String())
}

const yamlTimeFormat = "2006-01-02T15:04:05.000000000-07:00"

func timeToYaml(value time.Time) yamlString {
	return yamlString(fmt.Sprintf(
		"_aj_serialized: ActiveJob::Serializers::DateTimeSerializer\n    value: '%s'",
		value.Format(yamlTimeFormat),
	))
}

func boolToYaml(value bool) yamlString {
	return yamlString(fmt.Sprint(value))
}

func performNow(qu pgw.Queryable, class string, queue string, arguments ...yamlString) error {
	return performAt(qu, schedule.UTCNow(), class, queue, arguments...)
}

func performAt(
	qu pgw.Queryable, runAt schedule.Time, class string, queue string, arguments ...yamlString,
) error {
	logger := qu.Logger()
	const format1 = `--- !ruby/object:ActiveJob::QueueAdapters::DelayedJobAdapter::JobWrapper
job_data:
  job_class: %s
  job_id: %s
  provider_job_id: 
  queue_name: default
  priority: 
`
	const format2 = `  executions: 0
  exception_executions: {}
  locale: en
  timezone: UTC
  enqueued_at: '%s'
`
	var handler bytes.Buffer
	fmt.Fprintf(&handler, format1, class, uuid.New().String())
	if len(arguments) == 0 {
		fmt.Fprintln(&handler, "  arguments: []")
	} else {
		fmt.Fprintln(&handler, "  arguments:")
		for _, argument := range arguments {
			fmt.Fprintf(&handler, "  - %s\n", argument)
		}
	}
	fmt.Fprintf(&handler, format2, schedule.UTCNow().Format(time.RFC3339))

	row := qu.QueryRow(`
		insert into delayed_jobs (handler, run_at, queue)
		values ($1, $2, $3)
		returning id
	`, handler.String(), runAt, queue)
	var jobId JobId
	err := row.Scan(&jobId)
	if err != nil {
		logger.Error().Err(err).Msgf("Failed to enqueue job: %s %v", class, arguments)
	} else {
		if len(arguments) > 0 {
			logger.Info().Msgf("Enqueued job %d for %s: %s %v", jobId, runAt, class, arguments)
		} else {
			logger.Info().Msgf("Enqueued job %d for %s: %s", jobId, runAt, class)
		}
	}
	return err
}

func init() {
	migrations.DeleteJobByName = DeleteByName
}

func DeleteByName(qu pgw.Queryable, name string) error {
	result, err := qu.Exec(`delete from delayed_jobs where handler like E'%job_class: ` + name + `\n%'`)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return oops.Newf("No rows deleted for job %s", name)
	}
	return nil
}
