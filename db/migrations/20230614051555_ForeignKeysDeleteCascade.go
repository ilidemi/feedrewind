package migrations

import (
	"feedrewind/db/pgw"
	"fmt"
)

type foreignKey20230614051555 struct {
	table        string
	name         string
	column       string
	foreignTable string
}

type ForeignKeysDeleteCascade struct {
	foreignKeys []foreignKey20230614051555
}

func init() {
	registerMigration(&ForeignKeysDeleteCascade{
		foreignKeys: []foreignKey20230614051555{
			{
				table:        "user_rsses",
				name:         "fk_rails_17396fc3a7",
				column:       "user_id",
				foreignTable: "users",
			},
			{
				table:        "subscriptions",
				name:         "fk_rails_416412a06b",
				column:       "user_id",
				foreignTable: "users",
			},
			{
				table:        "subscription_rsses",
				name:         "fk_rails_647dccf03a",
				column:       "subscription_id",
				foreignTable: "subscriptions",
			},
			{
				table:        "blog_crawl_votes",
				name:         "fk_rails_6d5d61b810",
				column:       "blog_id",
				foreignTable: "blogs",
			},
			{
				table:        "blog_discarded_feed_entries",
				name:         "fk_rails_76729800cc",
				column:       "blog_id",
				foreignTable: "blogs",
			},
			{
				table:        "blog_post_category_assignments",
				name:         "fk_rails_77d2d90745",
				column:       "blog_post_id",
				foreignTable: "blog_posts",
			},
			{
				table:        "postmark_messages",
				name:         "fk_rails_897226ae9c",
				column:       "subscription_post_id",
				foreignTable: "subscription_posts",
			},
			{
				table:        "blog_canonical_equality_configs",
				name:         "fk_rails_9b3fd3910a",
				column:       "blog_id",
				foreignTable: "blogs",
			},
			{
				table:        "blog_posts",
				name:         "fk_rails_9d677c923b",
				column:       "blog_id",
				foreignTable: "blogs",
			},
			{
				table:        "blog_post_categories",
				name:         "fk_rails_a7dfb6db3e",
				column:       "blog_id",
				foreignTable: "blogs",
			},
			{
				table:        "blog_missing_from_feed_entries",
				name:         "fk_rails_aecbb1e2bd",
				column:       "blog_id",
				foreignTable: "blogs",
			},
			{
				table:        "schedules",
				name:         "fk_rails_b2b9b40998",
				column:       "subscription_id",
				foreignTable: "subscriptions",
			},
			{
				table:        "subscription_posts",
				name:         "fk_rails_b5e611fa3d",
				column:       "subscription_id",
				foreignTable: "subscriptions",
			},
			{
				table:        "blog_crawl_client_tokens",
				name:         "fk_rails_blogs",
				column:       "blog_id",
				foreignTable: "blogs",
			},
			{
				table:        "blog_crawl_progresses",
				name:         "fk_rails_blogs",
				column:       "blog_id",
				foreignTable: "blogs",
			},
			{
				table:        "blog_post_locks",
				name:         "fk_rails_c1a166e65f",
				column:       "blog_id",
				foreignTable: "blogs",
			},
			{
				table:        "subscriptions",
				name:         "fk_rails_c6353e971b",
				column:       "blog_id",
				foreignTable: "blogs",
			},
			{
				table:        "blog_post_category_assignments",
				name:         "fk_rails_c6de9c562d",
				column:       "category_id",
				foreignTable: "blog_post_categories",
			},
			{
				table:        "postmark_bounced_users",
				name:         "fk_rails_cf5feb15c9",
				column:       "example_bounce_id",
				foreignTable: "postmark_bounces",
			},
			{
				table:        "user_settings",
				name:         "fk_rails_d1371c6356",
				column:       "user_id",
				foreignTable: "users",
			},
			{
				table:        "subscription_posts",
				name:         "fk_rails_d857bf4496",
				column:       "blog_post_id",
				foreignTable: "blog_posts",
			},
			{
				table:        "start_feeds",
				name:         "fk_rails_d91add3512",
				column:       "start_page_id",
				foreignTable: "start_pages",
			},
			{
				table:        "postmark_messages",
				name:         "fk_rails_e906f9105c",
				column:       "subscription_id",
				foreignTable: "subscriptions",
			},
			{
				table:        "blog_crawl_votes",
				name:         "fk_rails_f74b6b39ca",
				column:       "user_id",
				foreignTable: "users",
			},
		},
	})
}

func (m *ForeignKeysDeleteCascade) Version() string {
	return "20230614051555"
}

func (m *ForeignKeysDeleteCascade) Up(tx *pgw.Tx) {
	for _, fKey := range m.foreignKeys {
		tx.MustExec("alter table " + fKey.table + " drop constraint " + fKey.name)

		query := fmt.Sprintf(`alter table %s
			add constraint %s foreign key(%s) references %s(id) on delete cascade`,
			fKey.table, fKey.name, fKey.column, fKey.foreignTable,
		)
		tx.MustExec(query)
	}
}

func (m *ForeignKeysDeleteCascade) Down(tx *pgw.Tx) {
	for _, fKey := range m.foreignKeys {
		tx.MustExec("alter table " + fKey.table + " drop constraint " + fKey.name)

		query := fmt.Sprintf(`alter table %s
			add constraint %s foreign key(%s) references %s(id)`,
			fKey.table, fKey.name, fKey.column, fKey.foreignTable,
		)
		tx.MustExec(query)
	}
}
