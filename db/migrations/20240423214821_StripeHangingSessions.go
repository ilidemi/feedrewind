package migrations

type StripeHangingSessions struct{}

func init() {
	registerMigration(&StripeHangingSessions{})
}

func (m *StripeHangingSessions) Version() string {
	return "20240423214821"
}

func (m *StripeHangingSessions) Up(tx *Tx) {
	tx.MustExec(`
		create table stripe_hanging_sessions (
			stripe_session_id text not null primary key,
			stripe_subscription_id text not null,
			stripe_customer_id text not null
		)
	`)
	tx.MustAddTimestamps("stripe_hanging_sessions")
}

func (m *StripeHangingSessions) Down(tx *Tx) {
	tx.MustExec(`drop table stripe_hanging_sessions`)
}
