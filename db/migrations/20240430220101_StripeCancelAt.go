package migrations

import "feedrewind/db/pgw"

type StripeCancelAt struct{}

func init() {
	registerMigration(&StripeCancelAt{})
}

func (m *StripeCancelAt) Version() string {
	return "20240430220101"
}

func (m *StripeCancelAt) Up(tx *Tx) {
	tx.MustExec(`alter table users add column stripe_cancel_at timestamp without time zone`)
	tx.MustUpdateDiscardedViews("users", &pgw.CheckUsersUsage)
}

func (m *StripeCancelAt) Down(tx *Tx) {
	tx.MustExec(`alter table users drop column stripe_cancel_at`)
	tx.MustUpdateDiscardedViews("users", &pgw.CheckUsersUsage)
}
