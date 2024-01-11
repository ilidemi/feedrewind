package migrations

type CreateIndicesOnBlogId2 struct{}

func init() {
	registerMigration(&CreateIndicesOnBlogId2{})
}

func (m *CreateIndicesOnBlogId2) Version() string {
	return "20240110082440"
}

func (m *CreateIndicesOnBlogId2) Up(tx *Tx) {
	tx.MustExec(`
		CREATE INDEX index_subscriptions_on_blog_id ON public.subscriptions USING btree (blog_id)
	`)
	tx.MustExec(`
		CREATE INDEX index_blog_post_category_assignments_on_blog_post_id ON public.blog_post_category_assignments USING btree (blog_post_id)
	`)
	tx.MustExec(`
		CREATE INDEX index_subscription_posts_on_blog_post_id ON public.subscription_posts USING btree (blog_post_id)
	`)
}

func (m *CreateIndicesOnBlogId2) Down(tx *Tx) {
	tx.MustExec(`
		DROP INDEX index_subscriptions_on_blog_id
	`)
	tx.MustExec(`
		DROP INDEX index_blog_post_category_assignments_on_blog_post_id
	`)
	tx.MustExec(`
		DROP INDEX index_subscription_posts_on_blog_post_id
	`)
}
