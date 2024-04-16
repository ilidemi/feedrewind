package migrations

import (
	"feedrewind/db/pgw"
	"feedrewind/util/schedule"
)

type DeleteDiscardedUsersJob struct{}

func init() {
	registerMigration(&DeleteDiscardedUsersJob{})
}

func (m *DeleteDiscardedUsersJob) Version() string {
	return "20240415100337"
}

var DeleteDiscardedUsersJob_PerformAtFunc func(tx pgw.Queryable, runAt schedule.Time) error

func (m *DeleteDiscardedUsersJob) Up(tx *Tx) {
	err := DeleteDiscardedUsersJob_PerformAtFunc(tx.impl, schedule.UTCNow())
	if err != nil {
		panic(err)
	}
}

func (m *DeleteDiscardedUsersJob) Down(tx *Tx) {
	tx.MustExec(`delete from delayed_jobs where handler like '%class: DeleteDiscardedUsersJob'`)
}
