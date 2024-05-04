package migrations

import (
	"feedrewind/db/pgw"
	"feedrewind/util/schedule"
)

type CheckStaleStripe struct{}

func init() {
	registerMigration(&CheckStaleStripe{})
}

func (m *CheckStaleStripe) Version() string {
	return "20240422112841"
}

var CheckStaleStripeJob_PerformAtFunc func(tx pgw.Queryable, runAt schedule.Time) error

func (m *CheckStaleStripe) Up(tx *Tx) {
	err := CheckStaleStripeJob_PerformAtFunc(tx.impl, schedule.UTCNow())
	if err != nil {
		panic(err)
	}
}

func (m *CheckStaleStripe) Down(tx *Tx) {
	tx.MustExec(`delete from delayed_jobs where handler like '%job_class: CheckStaleStripeJob%'`)
}
