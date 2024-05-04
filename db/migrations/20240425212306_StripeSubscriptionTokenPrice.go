package migrations

type StripeSubscriptionTokenPrice struct{}

func init() {
	registerMigration(&StripeSubscriptionTokenPrice{})
}

func (m *StripeSubscriptionTokenPrice) Version() string {
	return "20240425212306"
}

func (m *StripeSubscriptionTokenPrice) Up(tx *Tx) {
	tx.MustExec(`alter table stripe_subscription_tokens add column billing_interval billing_interval`)
	tx.MustExec(`update stripe_subscription_tokens set billing_interval = 'monthly'`)
	tx.MustExec(`alter table stripe_subscription_tokens alter column billing_interval set not null`)
}

func (m *StripeSubscriptionTokenPrice) Down(tx *Tx) {
	tx.MustExec(`alter table stripe_subscription_tokens drop column billing_interval`)
}
