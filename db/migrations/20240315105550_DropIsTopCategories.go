package migrations

type DropIsTopCategories struct{}

func init() {
	registerMigration(&DropIsTopCategories{})
}

func (m *DropIsTopCategories) Version() string {
	return "20240315105550"
}

func (m *DropIsTopCategories) Up(tx *Tx) {
	tx.MustExec(`alter table blog_post_categories drop column is_top`)
}

func (m *DropIsTopCategories) Down(tx *Tx) {
	tx.MustExec(`alter table blog_post_categories add column is_top boolean`)
	tx.MustExec(`update blog_post_categories set is_top = true where top_status = 'top_only'`)
	tx.MustExec(`update blog_post_categories set is_top = false where top_status != 'top_only'`)
	tx.MustExec(`alter table blog_post_categories alter column is_top set not null`)
}
