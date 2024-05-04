package migrations

type StripeWebhookEvents struct{}

func init() {
	registerMigration(&StripeWebhookEvents{})
}

func (m *StripeWebhookEvents) Version() string {
	return "20240429215829"
}

func (m *StripeWebhookEvents) Up(tx *Tx) {
	tx.MustExec(`
		create table stripe_webhook_events (
			id bigserial primary key,
			payload bytea not null
		)
	`)
	tx.MustAddTimestamps("stripe_webhook_events")
}

func (m *StripeWebhookEvents) Down(tx *Tx) {
	tx.MustExec(`drop table stripe_webhook_events`)
}
