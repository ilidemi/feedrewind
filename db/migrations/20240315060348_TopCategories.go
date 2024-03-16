package migrations

type TopCategories struct{}

func init() {
	registerMigration(&TopCategories{})
}

func (m *TopCategories) Version() string {
	return "20240315060348"
}

func (m *TopCategories) Up(tx *Tx) {
	tx.MustExec(`
		create type blog_post_category_top_status as enum ('top_only', 'top_and_custom', 'custom_only')
	`)
	tx.MustExec(`
		alter table blog_post_categories
		add column top_status blog_post_category_top_status
	`)
	tx.MustExec(`update blog_post_categories set top_status = 'top_only' where is_top = true`)
	tx.MustExec(`update blog_post_categories set top_status = 'custom_only' where is_top = false`)
	tx.MustExec(`alter table blog_post_categories alter column top_status set not null`)
}

func (m *TopCategories) Down(tx *Tx) {
	tx.MustExec(`alter table blog_post_categories drop column top_status`)
	tx.MustExec(`drop type blog_post_category_top_status`)
}
