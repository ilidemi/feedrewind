package migrations

type KnownBadBlogs struct{}

func init() {
	registerMigration(&KnownBadBlogs{})
}

func (m *KnownBadBlogs) Version() string {
	return "20241016184250"
}

func (m *KnownBadBlogs) Up(tx *Tx) {
	tx.MustExec(`alter type blog_status add value 'known_bad'`)
}

func (m *KnownBadBlogs) Down(tx *Tx) {
	panic("Not implemented")
}
