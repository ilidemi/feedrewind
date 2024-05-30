package migrations

type CustomBlogRequestWhy struct{}

func init() {
	registerMigration(&CustomBlogRequestWhy{})
}

func (m *CustomBlogRequestWhy) Version() string {
	return "20240528135354"
}

func (m *CustomBlogRequestWhy) Up(tx *Tx) {
	tx.MustExec(`alter table custom_blog_requests add column why text`)
	tx.MustExec(`update custom_blog_requests set why = ''`)
	tx.MustExec(`alter table custom_blog_requests alter column why set not null`)
}

func (m *CustomBlogRequestWhy) Down(tx *Tx) {
	tx.MustExec(`alter table custom_blog_requests drop column why`)
}
