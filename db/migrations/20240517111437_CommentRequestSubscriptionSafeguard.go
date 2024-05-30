package migrations

type CommentRequestSubscriptionSafeguard struct{}

func init() {
	registerMigration(&CommentRequestSubscriptionSafeguard{})
}

func (m *CommentRequestSubscriptionSafeguard) Version() string {
	return "20240517111437"
}

func (m *CommentRequestSubscriptionSafeguard) Up(tx *Tx) {
	tx.MustExec(`
		comment on column custom_blog_requests.subscription_id is
		'Protects unfulfilled subscriptions from accidental deletion. Set to null when a request is fulfilled.'
	`)
}

func (m *CommentRequestSubscriptionSafeguard) Down(tx *Tx) {
	tx.MustExec(`comment on column custom_blog_requests.subscription_id is null`)
}
