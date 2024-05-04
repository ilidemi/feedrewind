package migrations

type StripeEventId struct{}

func init() {
	registerMigration(&StripeEventId{})
}

func (m *StripeEventId) Version() string {
	return "20240501214712"
}

func (m *StripeEventId) Up(tx *Tx) {
	tx.MustExec(`drop table stripe_webhook_events`)
	tx.MustExec(`
		create table stripe_webhook_events (
			id text primary key,
			payload bytea not null
		)
	`)
	tx.MustAddTimestamps("stripe_webhook_events")
}

func (m *StripeEventId) Down(tx *Tx) {
	tx.MustExec(`drop table stripe_webhook_events`)
	tx.MustExec(`
		create table stripe_webhook_events (
			id bigserial primary key,
			payload bytea not null
		)
	`)
	tx.MustAddTimestamps("stripe_webhook_events")
}
