package migrations

type StripeSubscriptionToken struct{}

func init() {
	registerMigration(&StripeSubscriptionToken{})
}

func (m *StripeSubscriptionToken) Version() string {
	return "20240422100424"
}

func (m *StripeSubscriptionToken) Up(tx *Tx) {
	tx.MustExec(`
		create table stripe_subscription_tokens (
			id int8 primary key,
			offer_id text not null references pricing_offers(id),
			stripe_subscription_id text not null,
			stripe_customer_id text not null
		)
	`)
}

func (m *StripeSubscriptionToken) Down(tx *Tx) {
	tx.MustExec(`drop table stripe_subscription_tokens`)
}
