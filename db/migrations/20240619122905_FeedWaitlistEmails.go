package migrations

type FeedWaitlistEmails struct{}

func init() {
	registerMigration(&FeedWaitlistEmails{})
}

func (m *FeedWaitlistEmails) Version() string {
	return "20240619122905"
}

func (m *FeedWaitlistEmails) Up(tx *Tx) {
	tx.MustExec(`
		create table feed_waitlist_emails (
			feed_url text not null,
			email text not null,
			user_id bigint references users(id) on delete cascade,
			notify boolean not null,
			version integer,
			primary key (feed_url, email)
		)
	`)
	tx.MustAddTimestamps(`feed_waitlist_emails`)
}

func (m *FeedWaitlistEmails) Down(tx *Tx) {
	tx.MustExec(`drop table feed_waitlist_emails`)
}
