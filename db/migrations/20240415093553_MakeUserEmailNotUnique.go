package migrations

type MakeUserEmailNotUnique struct{}

func init() {
	registerMigration(&MakeUserEmailNotUnique{})
}

func (m *MakeUserEmailNotUnique) Version() string {
	return "20240415093553"
}

func (m *MakeUserEmailNotUnique) Up(tx *Tx) {
	tx.MustExec("alter table users drop constraint users_email_unique")
	tx.MustExec(`
		create unique index users_email_without_discarded on users (email)
		where discarded_at is null;
	`)
}

func (m *MakeUserEmailNotUnique) Down(tx *Tx) {
	tx.MustExec("alter table users add constraint users_email_unique unique (email)")
	tx.MustExec("drop index users_email_without_discarded")
}
