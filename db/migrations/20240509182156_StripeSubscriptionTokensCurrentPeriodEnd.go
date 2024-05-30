package migrations

type StripeSubscriptionTokensCurrentPeriodEnd struct{}

func init() {
	registerMigration(&StripeSubscriptionTokensCurrentPeriodEnd{})
}

func (m *StripeSubscriptionTokensCurrentPeriodEnd) Version() string {
	return "20240509182156"
}

func (m *StripeSubscriptionTokensCurrentPeriodEnd) Up(tx *Tx) {
	tx.MustExec(`
		alter table stripe_subscription_tokens
		add column current_period_end timestamp without time zone not null
	`)
}

func (m *StripeSubscriptionTokensCurrentPeriodEnd) Down(tx *Tx) {
	tx.MustExec(`alter table stripe_subscription_tokens drop column current_period_end`)
}
