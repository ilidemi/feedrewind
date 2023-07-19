package migrations

import (
	"feedrewind/db/pgw"
)

type TimestampsUtcNow struct{}

func init() {
	registerMigration(&TimestampsUtcNow{})
}

func (m *TimestampsUtcNow) Version() string {
	return "20230712025648"
}

var tables20230712025648 = []string{
	"admin_telemetries",
	"ar_internal_metadata",
	"blog_canonical_equality_configs",
	"blog_crawl_client_tokens",
	"blog_crawl_progresses",
	"blog_crawl_votes",
	"blog_discarded_feed_entries",
	"blog_missing_from_feed_entries",
	"blog_post_categories",
	"blog_post_category_assignments",
	"blog_post_locks",
	"blog_posts",
	"blogs",
	"delayed_jobs",
	"postmark_bounced_users",
	"postmark_bounces",
	"postmark_messages",
	"product_events",
	"schedules",
	"start_feeds",
	"start_pages",
	"subscription_posts",
	"subscription_rsses",
	"subscriptions",
	"test_singletons",
	"typed_blog_urls",
	"user_rsses",
	"user_settings",
	"users",
}

func (m *TimestampsUtcNow) Up(tx *pgw.Tx) {
	tx.MustExec(`
create or replace function bump_updated_at_utc()
returns trigger as $$
begin
	NEW.updated_at = utc_now();
	return NEW;
end;
$$ language 'plpgsql'
`)

	for _, table := range tables20230712025648 {
		tx.MustExec("alter table " + table + " alter column created_at set default utc_now()")
		tx.MustExec("alter table " + table + " alter column updated_at set default utc_now()")
	}
}

func (m *TimestampsUtcNow) Down(tx *pgw.Tx) {
	panic("Not implemented")
}
