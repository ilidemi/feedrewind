package migrations

import (
	"feedrewind.com/db/pgw"
	"feedrewind.com/util/schedule"
)

type BackupDbJob struct{}

func init() {
	registerMigration(&BackupDbJob{})
}

func (m *BackupDbJob) Version() string {
	return "20240705122055"
}

var BackupDbJob_PerformAtFunc func(qu pgw.Queryable, runAt schedule.Time) error

func (m *BackupDbJob) Up(tx *Tx) {
	err := BackupDbJob_PerformAtFunc(tx.impl, schedule.UTCNow())
	if err != nil {
		panic(err)
	}
}

func (m *BackupDbJob) Down(tx *Tx) {
	tx.MustDeleteJobByName("BackupDbJob")
}
