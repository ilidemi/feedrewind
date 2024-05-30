package migrations

import (
	"feedrewind/db/pgw"
	"feedrewind/util/schedule"
)

type PollCustomBlogRequestsJob struct{}

func init() {
	registerMigration(&PollCustomBlogRequestsJob{})
}

func (m *PollCustomBlogRequestsJob) Version() string {
	return "20240517194027"
}

var PollCustomBlogRequestsJob_PerformAtFunc func(pgw.Queryable, schedule.Time) error

func (m *PollCustomBlogRequestsJob) Up(tx *Tx) {
	err := PollCustomBlogRequestsJob_PerformAtFunc(tx.impl, schedule.UTCNow())
	if err != nil {
		panic(err)
	}
}

func (m *PollCustomBlogRequestsJob) Down(tx *Tx) {
	tx.MustDeleteJobByName("PollCustomBlogRequestsJob")
}
