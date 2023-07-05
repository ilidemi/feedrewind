package migrations

import (
	"feedrewind/db/pgw"
)

type AddUniqueEmailConstraint struct{}

func init() {
	registerMigration(&AddUniqueEmailConstraint{})
}

func (m *AddUniqueEmailConstraint) Version() string {
	return "20230623031513"
}

func (m *AddUniqueEmailConstraint) Up(tx *pgw.Tx) {
	tx.MustExec("alter table users add constraint users_email_unique unique (email)")
}

func (m *AddUniqueEmailConstraint) Down(tx *pgw.Tx) {
	tx.MustExec("alter table users drop constraint users_email_unique")
}
