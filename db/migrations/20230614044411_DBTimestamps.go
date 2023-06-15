package migrations

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
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

func (m *DBTimestamps) Up(ctx context.Context, tx pgx.Tx) {
	_, err := tx.Exec(ctx, `
create function bump_updated_at()
returns trigger as $$
begin
	NEW.updated_at = now();
	return NEW;
end;
$$ language 'plpgsql'
`)
	if err != nil {
		panic(err)
	}

	for _, table := range tables {
		_, err := tx.Exec(
			ctx, "alter table "+table+" alter column created_at set default current_timestamp",
		)
		if err != nil {
			panic(err)
		}

		_, err = tx.Exec(
			ctx, "alter table "+table+" alter column updated_at set default current_timestamp",
		)
		if err != nil {
			panic(err)
		}

		query := fmt.Sprintf(`create trigger %s_bump_updated_at
	before update on %s
	for each row
	execute procedure bump_updated_at();
`, table, table)
		_, err = tx.Exec(ctx, query)
		if err != nil {
			panic(err)
		}
	}

}

func (m *DBTimestamps) Down(ctx context.Context, tx pgx.Tx) {
	for _, table := range tables {
		_, err := tx.Exec(ctx, "alter table "+table+" alter column created_at drop default")
		if err != nil {
			panic(err)
		}

		_, err = tx.Exec(ctx, "alter table "+table+" alter column updated_at drop default")
		if err != nil {
			panic(err)
		}
		_, err = tx.Exec(ctx, "drop trigger "+table+"_bump_updated_at on "+table)
		if err != nil {
			panic(err)
		}
	}

	_, err := tx.Exec(ctx, "drop function bump_updated_at")
	if err != nil {
		panic(err)
	}
}
