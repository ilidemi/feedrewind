package migrations

type Debug struct{}

func init() {
	registerMigration(&Debug{})
}

func (m *Debug) Version() string {
	return "20241002143639"
}

func (m *Debug) Up(tx *Tx) {
	tx.MustExec(`
		create table debug (
			id serial primary key,
			key text not null,
			value bytea not null
		)
	`)
	tx.MustAddTimestamps("debug")
}

func (m *Debug) Down(tx *Tx) {
	tx.MustExec(`drop table debug`)
}
