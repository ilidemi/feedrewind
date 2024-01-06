package migrations

import (
	"feedrewind/db/pgw"
	"feedrewind/util/schedule"
)

type CheckDoubleScheduleJob struct{}

func init() {
	registerMigration(&CheckDoubleScheduleJob{})
}

func (m *CheckDoubleScheduleJob) Version() string {
	return "20240105034032"
}

var CheckDoubleScheduleJob_PerformAtFunc func(pgw.Queryable, schedule.Time) error

func (m *CheckDoubleScheduleJob) Up(tx *Tx) {
	err := CheckDoubleScheduleJob_PerformAtFunc(tx.impl, schedule.UTCNow())
	if err != nil {
		panic(err)
	}
}

func (m *CheckDoubleScheduleJob) Down(tx *Tx) {
	tx.MustExec(`delete from delayed_jobs where handler like '%class: CheckDoubleScheduleJob'`)
}
