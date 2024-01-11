package migrations

type CreateIndicesOnBlogId struct{}

func init() {
	registerMigration(&CreateIndicesOnBlogId{})
}

func (m *CreateIndicesOnBlogId) Version() string {
	return "20240110082145"
}

func (m *CreateIndicesOnBlogId) Up(tx *Tx) {
	tx.MustExec(`
		CREATE INDEX index_blog_posts_on_blog_id ON public.blog_posts USING btree (blog_id)
	`)
	tx.MustExec(`
		CREATE INDEX index_blog_post_categories_on_blog_id ON public.blog_post_categories USING btree (blog_id)
	`)
}

func (m *CreateIndicesOnBlogId) Down(tx *Tx) {
	tx.MustExec(`
		DROP INDEX index_blog_posts_on_blog_id
	`)
	tx.MustExec(`
		DROP INDEX index_blog_post_categories_on_blog_id
	`)
}
