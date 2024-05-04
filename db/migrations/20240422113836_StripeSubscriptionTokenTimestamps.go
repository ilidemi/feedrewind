package migrations

type StripeSubscriptionTokenTimestamps struct{}

func init() {
	registerMigration(&StripeSubscriptionTokenTimestamps{})
}

func (m *StripeSubscriptionTokenTimestamps) Version() string {
	return "20240422113836"
}

func (m *StripeSubscriptionTokenTimestamps) Up(tx *Tx) {
	tx.MustExec(`
		alter table stripe_subscription_tokens
		add column created_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL
	`)
	tx.MustExec(`
		alter table stripe_subscription_tokens
		add column updated_at timestamp(6) without time zone DEFAULT public.utc_now() NOT NULL
	`)
}

func (m *StripeSubscriptionTokenTimestamps) Down(tx *Tx) {
	tx.MustExec(`alter table stripe_subscription_tokens drop column created_at`)
	tx.MustExec(`alter table stripe_subscription_tokens drop column updated_at`)
}
