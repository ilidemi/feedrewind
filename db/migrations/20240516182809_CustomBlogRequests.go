package migrations

import "feedrewind/db/pgw"

type CustomBlogRequests struct{}

func init() {
	registerMigration(&CustomBlogRequests{})
}

func (m *CustomBlogRequests) Version() string {
	return "20240516182809"
}

func (m *CustomBlogRequests) Up(tx *Tx) {
	tx.MustExec(`drop table custom_blog_requests`)
	tx.MustExec(`drop type custom_blog_request_status`)
	tx.MustExec(`
		create table custom_blog_requests (
			id bigserial primary key,
			blog_url text,
			feed_url text not null,
			stripe_invoice_id text,
			user_id bigint references users (id) on delete set null,
			subscription_id bigint references subscriptions (id) on delete no action,
			blog_id bigint references blogs (id) on delete set null,
			fulfilled_at timestamp without time zone
		)
	`)
	tx.MustAddTimestamps("custom_blog_requests")
	tx.MustExec(`alter type subscription_status add value 'custom_blog_requested'`)
}

func (m *CustomBlogRequests) Down(tx *Tx) {
	tx.MustExec(`drop table custom_blog_requests`)

	tx.MustExec(`alter type subscription_status rename to subscription_status_old`)
	tx.MustExec(`create type subscription_status as enum ('waiting_for_blog', 'setup', 'live')`)
	tx.MustExec(`alter table subscriptions rename column status to status_old`)
	tx.MustExec(`alter table subscriptions add column status subscription_status`)
	pgw.CheckSubscriptionsUsage = false
	tx.MustExec(`update subscriptions set status = status_old::text::subscription_status`)
	pgw.CheckSubscriptionsUsage = true
	tx.MustExec(`alter table subscriptions alter column status set not null`)
	tx.MustExec(`drop view subscriptions_with_discarded`)
	tx.MustExec(`drop view subscriptions_without_discarded`)
	tx.MustExec(`alter table subscriptions drop column status_old`)
	tx.MustExec(`drop type subscription_status_old`)
	tx.MustUpdateDiscardedViews("subscriptions", &pgw.CheckSubscriptionsUsage)

	tx.MustExec(`create type custom_blog_request_status as enum ('pending', 'fulfilled')`)
	tx.MustExec(`
		create table custom_blog_requests (
			id bigint primary key,
			user_id bigint not null references users(id) on delete cascade,
			name text not null,
			stripe_invoice_id text,
			status custom_blog_request_status not null
		)
	`)
	tx.MustAddTimestamps("custom_blog_requests")
	tx.MustExec(`
		create index index_custom_blog_requests_on_user_id ON custom_blog_requests USING btree (user_id);
	`)
}
