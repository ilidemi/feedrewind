package jobs

import (
	"bytes"
	"feedrewind/db/pgw"
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

func mustPerformNow(tx pgw.Queryable, class string, queue string, arguments ...yamlString) {
	mustPerformAt(tx, time.Now().UTC(), class, queue, arguments...)
}

func mustPerformAt(tx pgw.Queryable, runAt time.Time, class string, queue string, arguments ...yamlString) {
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
	tx.MustExec(`
		insert into delayed_jobs (handler, run_at, queue)
		values ($1, $2, $3)
	`, handler.String(), runAtStr, queue)
}
