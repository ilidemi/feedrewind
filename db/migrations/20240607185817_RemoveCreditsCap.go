package migrations

type RemoveCreditsCap struct{}

func init() {
	registerMigration(&RemoveCreditsCap{})
}

func (m *RemoveCreditsCap) Version() string {
	return "20240607185817"
}

func (m *RemoveCreditsCap) Up(tx *Tx) {
	tx.MustExec(`alter table patron_credits drop column cap`)
}

func (m *RemoveCreditsCap) Down(tx *Tx) {
	tx.MustExec(`alter table patron_credits add column cap integer`)
	tx.MustExec(`update patron_credits set cap = 3`)
	tx.MustExec(`alter table patron_credits alter column cap set not null`)
}
