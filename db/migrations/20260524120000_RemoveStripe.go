package migrations

import "feedrewind.com/db/pgw"

type RemoveStripe struct{}

func init() {
	registerMigration(&RemoveStripe{})
}

func (m *RemoveStripe) Version() string {
	return "20260524120000"
}

func (m *RemoveStripe) Up(tx *Tx) {
	tx.MustExec(`
		update users
		set offer_id = (select default_offer_id from pricing_plans where id = 'free')
	`)

	tx.MustExec(`
		update subscriptions
		set status = 'waiting_for_blog'
		where status = 'custom_blog_requested'
	`)
	// Keep the legacy subscription_status enum value to avoid recreating the enum type.
	// New code no longer writes custom_blog_requested.

	tx.MustExec(`drop table if exists custom_blog_requests`)
	tx.MustExec(`drop table if exists patron_credits`)
	tx.MustExec(`drop table if exists patron_invoices`)
	tx.MustExec(`drop table if exists stripe_subscription_tokens`)
	tx.MustExec(`drop table if exists stripe_webhook_events`)

	tx.MustExec(`drop view users_without_discarded`)
	tx.MustExec(`drop view users_with_discarded`)

	tx.MustExec(`
		alter table users
			drop column stripe_subscription_id,
			drop column stripe_customer_id,
			drop column stripe_cancel_at,
			drop column stripe_current_period_end,
			drop column billing_interval
	`)

	tx.MustExec(`
		alter table pricing_offers
			drop column stripe_product_id,
			drop column stripe_monthly_price_id,
			drop column stripe_yearly_price_id
	`)

	tx.MustExec(`delete from pricing_offers where plan_id != 'free'`)
	tx.MustExec(`delete from pricing_plans where id != 'free'`)

	tx.MustUpdateDiscardedViews("users", &pgw.CheckUsersUsage)
}

func (m *RemoveStripe) Down(tx *Tx) {
	panic("Not implemented")
}
