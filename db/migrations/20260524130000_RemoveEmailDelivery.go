package migrations

type RemoveEmailDelivery struct{}

func init() {
	registerMigration(&RemoveEmailDelivery{})
}

func (m *RemoveEmailDelivery) Version() string {
	return "20260524130000"
}

func (m *RemoveEmailDelivery) Up(tx *Tx) {
	tx.MustExec(`
		update user_settings set delivery_channel = 'multiple_feeds' where delivery_channel = 'email'
	`)

	tx.MustExec(`
		update subscription_posts
		set publish_status = 'rss_published'
		where publish_status in ('email_pending', 'email_skipped', 'email_sent')
	`)
	tx.MustExec(`
		update subscriptions
		set initial_item_publish_status = 'rss_published'
		where initial_item_publish_status in ('email_pending', 'email_skipped', 'email_sent')
	`)
	tx.MustExec(`
		update subscriptions
		set final_item_publish_status = 'rss_published'
		where final_item_publish_status in ('email_pending', 'email_skipped', 'email_sent')
	`)
	// Keep the legacy enum values to avoid recreating the enum types.
	// New code no longer writes email-related statuses or delivery channels.

	tx.MustExec(`drop table if exists postmark_messages`)
	tx.MustExec(`drop table if exists postmark_bounced_users`)
	tx.MustExec(`drop table if exists postmark_bounces`)
}

func (m *RemoveEmailDelivery) Down(tx *Tx) {
	panic("Not implemented")
}
