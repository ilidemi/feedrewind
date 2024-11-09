package migrations

import (
	"feedrewind.com/db/pgw"
	"feedrewind.com/util/schedule"
)

type CodeMaintenanceJob struct{}

func init() {
	registerMigration(&CodeMaintenanceJob{})
}

func (m *CodeMaintenanceJob) Version() string {
	return "20240627193236"
}

var CodeMaintenanceJob_PerformAtFunc func(pgw.Queryable, schedule.Time) error

func (m *CodeMaintenanceJob) Up(tx *Tx) {
	err := CodeMaintenanceJob_PerformAtFunc(tx.impl, schedule.UTCNow())
	if err != nil {
		panic(err)
	}
}

func (m *CodeMaintenanceJob) Down(tx *Tx) {
	tx.MustDeleteJobByName("CodeMaintenanceJob")
}
