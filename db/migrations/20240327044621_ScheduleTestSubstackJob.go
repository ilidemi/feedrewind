package migrations

import (
	"feedrewind/db/pgw"
	"feedrewind/util/schedule"
)

type ScheduleTestSubstackJob struct{}

func init() {
	registerMigration(&ScheduleTestSubstackJob{})
}

var ScheduleTestSubstackJob_PerformAtFunc func(tx pgw.Queryable, runAt schedule.Time) error

func (m *ScheduleTestSubstackJob) Version() string {
	return "20240327044621"
}

func (m *ScheduleTestSubstackJob) Up(tx *Tx) {
	err := ScheduleTestSubstackJob_PerformAtFunc(tx.impl, schedule.UTCNow())
	if err != nil {
		panic(err)
	}
}

func (m *ScheduleTestSubstackJob) Down(tx *Tx) {
	tx.MustExec(`delete from delayed_jobs where handler like '%class: TestSubstackJob'`)
}
