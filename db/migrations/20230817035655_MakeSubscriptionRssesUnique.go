package migrations

type MakeSubscriptionRssesUnique struct{}

func init() {
	registerMigration(&MakeSubscriptionRssesUnique{})
}

func (m *MakeSubscriptionRssesUnique) Version() string {
	return "20230817035655"
}

func (m *MakeSubscriptionRssesUnique) Up(tx *Tx) {
	tx.MustExec(`
		alter table subscription_rsses
		add constraint subscription_rsses_subscription_id_unique unique (subscription_id)
	`)
}

func (m *MakeSubscriptionRssesUnique) Down(tx *Tx) {
	tx.MustExec(`
		alter table subscription_rsses
		drop constraint subscription_rsses_subscription_id_unique
	`)
}
