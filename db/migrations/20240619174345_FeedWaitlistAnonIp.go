package migrations

type FeedWaitlistAnonIp struct{}

func init() {
	registerMigration(&FeedWaitlistAnonIp{})
}

func (m *FeedWaitlistAnonIp) Version() string {
	return "20240619174345"
}

func (m *FeedWaitlistAnonIp) Up(tx *Tx) {
	tx.MustExec(`alter table feed_waitlist_emails add column anon_ip text not null`)
}

func (m *FeedWaitlistAnonIp) Down(tx *Tx) {
	tx.MustExec(`alter table feed_waitlist_emails drop column anon_ip`)
}
