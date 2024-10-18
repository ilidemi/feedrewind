package migrations

import (
	"feedrewind/db/pgw"
	"feedrewind/util/schedule"
)

type MaintenanceJob struct{}

func init() {
	registerMigration(&MaintenanceJob{})
}

func (m *MaintenanceJob) Version() string {
	return "20241018150103"
}

var MaintenanceJob_PerformAtFunc func(pgw.Queryable, schedule.Time) error

func (m *MaintenanceJob) Up(tx *Tx) {
	err := MaintenanceJob_PerformAtFunc(tx.impl, schedule.UTCNow())
	if err != nil {
		panic(err)
	}

	err = DeleteJobByName(tx.impl, "CheckDoubleScheduleJob")
	if err != nil {
		panic(err)
	}

	err = DeleteJobByName(tx.impl, "CheckStaleStripeJob")
	if err != nil {
		panic(err)
	}
}

func (m *MaintenanceJob) Down(tx *Tx) {
	panic("Not implemented")
}
