package migrations

import (
	"feedrewind.com/db/pgw"
	"feedrewind.com/util/schedule"
)

type CleanupDbJob struct{}

func init() {
	registerMigration(&CleanupDbJob{})
}

func (m *CleanupDbJob) Version() string {
	return "20240705190748"
}

var CleanupDbJob_PerformAtFunc func(qu pgw.Queryable, runAt schedule.Time) error

func (m *CleanupDbJob) Up(tx *Tx) {
	err := CleanupDbJob_PerformAtFunc(tx.impl, schedule.UTCNow())
	if err != nil {
		panic(err)
	}
	tx.MustDeleteJobByName("DeleteDiscardedSubscriptionsJob")
	tx.MustDeleteJobByName("DeleteDiscardedUsersJob")
}

func (m *CleanupDbJob) Down(tx *Tx) {
	tx.MustDeleteJobByName("CleanupDbJob")
	panic("Subscriptions and users job are missing")
}
