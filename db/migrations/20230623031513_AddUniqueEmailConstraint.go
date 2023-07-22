package migrations

type AddUniqueEmailConstraint struct{}

func init() {
	registerMigration(&AddUniqueEmailConstraint{})
}

func (m *AddUniqueEmailConstraint) Version() string {
	return "20230623031513"
}

func (m *AddUniqueEmailConstraint) Up(tx *Tx) {
	tx.MustExec("alter table users add constraint users_email_unique unique (email)")
}

func (m *AddUniqueEmailConstraint) Down(tx *Tx) {
	tx.MustExec("alter table users drop constraint users_email_unique")
}
