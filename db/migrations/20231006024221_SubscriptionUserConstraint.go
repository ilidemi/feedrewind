package migrations

type SubscriptionUserConstraint struct{}

func init() {
	registerMigration(&SubscriptionUserConstraint{})
}

func (m *SubscriptionUserConstraint) Version() string {
	return "20231006024221"
}

func (m *SubscriptionUserConstraint) Up(tx *Tx) {
	tx.MustExec(`
		alter table subscriptions
		add constraint subscriptions_refers_to_user check(not(user_id is null and anon_product_user_id is null))
	`)
}

func (m *SubscriptionUserConstraint) Down(tx *Tx) {
	tx.MustExec(`
		alter table subscriptions
		drop constraint subscriptions_refers_to_user
	`)
}
