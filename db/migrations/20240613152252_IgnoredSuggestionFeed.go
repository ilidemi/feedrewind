package migrations

type IgnoredSuggestionFeed struct{}

func init() {
	registerMigration(&IgnoredSuggestionFeed{})
}

func (m *IgnoredSuggestionFeed) Version() string {
	return "20240613152252"
}

func (m *IgnoredSuggestionFeed) Up(tx *Tx) {
	tx.MustExec(`
		create table ignored_suggestion_feeds (
			feed_url text primary key
		)
	`)
	tx.MustAddTimestamps("ignored_suggestion_feeds")
}

func (m *IgnoredSuggestionFeed) Down(tx *Tx) {
	tx.MustExec(`drop table ignored_suggestion_feeds`)
}
