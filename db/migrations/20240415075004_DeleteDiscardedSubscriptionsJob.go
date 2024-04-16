package migrations

import (
	"feedrewind/db/pgw"
	"feedrewind/util/schedule"
)

type DeleteDiscardedSubscriptionsJob struct{}

func init() {
	registerMigration(&DeleteDiscardedSubscriptionsJob{})
}

var DeleteDiscardedSubscriptionsJob_PerformAtFunc func(tx pgw.Queryable, runAt schedule.Time) error

func (m *DeleteDiscardedSubscriptionsJob) Version() string {
	return "20240415075004"
}

func (m *DeleteDiscardedSubscriptionsJob) Up(tx *Tx) {
	err := DeleteDiscardedSubscriptionsJob_PerformAtFunc(tx.impl, schedule.UTCNow())
	if err != nil {
		panic(err)
	}
}

func (m *DeleteDiscardedSubscriptionsJob) Down(tx *Tx) {
	tx.MustExec(`delete from delayed_jobs where handler like '%class: DeleteDiscardedSubscriptionsJob'`)
}
