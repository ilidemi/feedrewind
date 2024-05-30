package migrations

type PatronCreditsAllowZero struct{}

func init() {
	registerMigration(&PatronCreditsAllowZero{})
}

func (m *PatronCreditsAllowZero) Version() string {
	return "20240520160130"
}

func (m *PatronCreditsAllowZero) Up(tx *Tx) {
	tx.MustExec(`alter table patron_credits drop constraint count_non_negative`)
	tx.MustExec(`alter table patron_credits add constraint count_non_negative check (count >= 0)`)
}

func (m *PatronCreditsAllowZero) Down(tx *Tx) {
	tx.MustExec(`alter table patron_credits drop constraint count_non_negative`)
	tx.MustExec(`alter table patron_credits add constraint count_non_negative check (count > 0)`)
}
