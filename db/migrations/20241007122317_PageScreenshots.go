package migrations

type PageScreenshots struct{}

func init() {
	registerMigration(&PageScreenshots{})
}

func (m *PageScreenshots) Version() string {
	return "20241007122317"
}

func (m *PageScreenshots) Up(tx *Tx) {
	tx.MustExec(`
		create table page_screenshots (
			id serial primary key,
			url text not null,
			source text not null,
			data bytea not null
		)
	`)
	tx.MustAddTimestamps("page_screenshots")
}

func (m *PageScreenshots) Down(tx *Tx) {
	tx.MustExec(`drop table screenshots`)
}
