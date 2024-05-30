package migrations

type PatronCreditCheck struct{}

func init() {
	registerMigration(&PatronCreditCheck{})
}

func (m *PatronCreditCheck) Version() string {
	return "20240517110129"
}

func (m *PatronCreditCheck) Up(tx *Tx) {
	tx.MustExec(`alter table patron_credits add constraint count_non_negative check (count > 0)`)
}

func (m *PatronCreditCheck) Down(tx *Tx) {
	tx.MustExec(`alter table patron_credits drop constraint count_non_negative`)
}
