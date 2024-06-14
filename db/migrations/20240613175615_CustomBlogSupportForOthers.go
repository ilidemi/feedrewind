package migrations

type CustomBlogSupportForOthers struct{}

func init() {
	registerMigration(&CustomBlogSupportForOthers{})
}

func (m *CustomBlogSupportForOthers) Version() string {
	return "20240613175615"
}

func (m *CustomBlogSupportForOthers) Up(tx *Tx) {
	tx.MustExec(`alter table custom_blog_requests add column enable_for_others bool`)
	tx.MustExec(`update custom_blog_requests set enable_for_others = true`)
	tx.MustExec(`alter table custom_blog_requests alter column enable_for_others set not null`)
}

func (m *CustomBlogSupportForOthers) Down(tx *Tx) {
	tx.MustExec(`alter table custom_blog_requests drop column enable_for_others`)
}
