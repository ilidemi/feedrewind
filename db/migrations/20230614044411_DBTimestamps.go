package migrations

import (
	"context"
	"feedrewind/db/pgw"
	"fmt"
)

type DBTimestamps struct{}

func init() {
	registerMigration(&DBTimestamps{})
}

func (m *DBTimestamps) Version() string {
	return "20230614044411"
}

var tables = []string{
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

func (m *DBTimestamps) Up(ctx context.Context, tx *pgw.Tx) {
	tx.MustExec(ctx, `
create function bump_updated_at()
returns trigger as $$
begin
	NEW.updated_at = now();
	return NEW;
end;
$$ language 'plpgsql'
`)

	for _, table := range tables {
		tx.MustExec(ctx, "alter table "+table+" alter column created_at set default current_timestamp")
		tx.MustExec(ctx, "alter table "+table+" alter column updated_at set default current_timestamp")

		query := fmt.Sprintf(`create trigger %s_bump_updated_at
	before update on %s
	for each row
	execute procedure bump_updated_at();
`, table, table)
		tx.MustExec(ctx, query)

	}

}

func (m *DBTimestamps) Down(ctx context.Context, tx *pgw.Tx) {
	for _, table := range tables {
		tx.MustExec(ctx, "alter table "+table+" alter column created_at drop default")
		tx.MustExec(ctx, "alter table "+table+" alter column updated_at drop default")
		tx.MustExec(ctx, "drop trigger "+table+"_bump_updated_at on "+table)
	}

	tx.MustExec(ctx, "drop function bump_updated_at")
}
