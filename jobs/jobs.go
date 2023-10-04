package jobs

import (
	"bytes"
	"feedrewind/db/pgw"
	"feedrewind/util"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type JobId int64

const runAtFormat = "2006-01-02 15:04:05.000"
const defaultQueue = "default"

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

func timeToYaml(value time.Time) yamlString {
	return yamlString(fmt.Sprintf(
		"_aj_serialized: ActiveJob::Serializers::DateTimeSerializer\n    value: '%s'",
		value.Format("2006-01-02T15:04:05.000000000-07:00"),
	))
}

func boolToYaml(value bool) yamlString {
	return yamlString(fmt.Sprint(value))
}

func performNow(tx pgw.Queryable, class string, queue string, arguments ...yamlString) error {
	return performAt(tx, util.Schedule_UTCNow(), class, queue, arguments...)
}

func performAt(tx pgw.Queryable, runAt time.Time, class string, queue string, arguments ...yamlString) error {
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
	fmt.Fprintf(&handler, format2, runAt.Format(time.RFC3339))

	runAtStr := runAt.Format(runAtFormat)
	_, err := tx.Exec(`
		insert into delayed_jobs (handler, run_at, queue)
		values ($1, $2, $3)
	`, handler.String(), runAtStr, queue)
	return err
}
