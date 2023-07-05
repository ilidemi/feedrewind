package migrations

import (
	"feedrewind/db/pgw"
	"fmt"
)

type FixDBTimezone struct{}

func init() {
	registerMigration(&FixDBTimezone{})
}

func (m *FixDBTimezone) Version() string {
	return "20230704085733"
}

var tables20230704085733 = []string{
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

func (m *FixDBTimezone) Up(tx *pgw.Tx) {
	tx.MustExec(`
create function bump_updated_at_utc()
returns trigger as $$
begin
	NEW.updated_at = (now() at time zone 'utc');
	return NEW;
end;
$$ language 'plpgsql'
`)

	for _, table := range tables20230704085733 {
		tx.MustExec("alter table " + table + " alter column created_at set default (now() at time zone 'utc')")
		tx.MustExec("alter table " + table + " alter column updated_at set default (now() at time zone 'utc')")

		query := fmt.Sprintf(`drop trigger %s_bump_updated_at on %s`, table, table)
		tx.MustExec(query)

		query = fmt.Sprintf(`create trigger bump_updated_at
			before update on %s
			for each row
			execute procedure bump_updated_at_utc();
		`, table)
		tx.MustExec(query)
	}

	tx.MustExec("drop function bump_updated_at")
}

func (m *FixDBTimezone) Down(tx *pgw.Tx) {
	panic("Not implemented")
}
