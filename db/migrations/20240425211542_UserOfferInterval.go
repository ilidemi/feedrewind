package migrations

type UserOfferInterval struct{}

func init() {
	registerMigration(&UserOfferInterval{})
}

func (m *UserOfferInterval) Version() string {
	return "20240425211542"
}

func (m *UserOfferInterval) Up(tx *Tx) {
	tx.MustExec(`create type billing_interval as enum('monthly', 'yearly')`)
	tx.MustExec(`alter table users add column billing_interval billing_interval`)
}

func (m *UserOfferInterval) Down(tx *Tx) {
	tx.MustExec(`alter table users drop column billing_interval billing_interval`)
	tx.MustExec(`drop type billing_interval`)
}
