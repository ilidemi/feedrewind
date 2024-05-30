package migrations

type PatronTables struct{}

func init() {
	registerMigration(&PatronTables{})
}

func (m *PatronTables) Version() string {
	return "20240508225255"
}

func (m *PatronTables) Up(tx *Tx) {
	tx.MustExec(`
		create table patron_invoices (
			id text primary key
		)
	`)
	tx.MustAddTimestamps("patron_invoices")
	tx.MustExec(`
		create table patron_credits (
			user_id bigint primary key references users(id) on delete cascade,
			count int not null,
			cap int not null
		)
	`)
	tx.MustAddTimestamps("patron_credits")
	tx.MustExec(`create type custom_blog_request_status as enum ('pending', 'fulfilled')`)
	tx.MustExec(`
		create table custom_blog_requests (
			id bigint primary key,
			user_id bigint not null references users(id) on delete cascade,
			name text not null,
			stripe_invoice_id text,
			status custom_blog_request_status not null
		)
	`)
	tx.MustAddTimestamps("custom_blog_requests")
	tx.MustExec(`
		create index index_custom_blog_requests_on_user_id ON custom_blog_requests USING btree (user_id);
	`)
}

func (m *PatronTables) Down(tx *Tx) {
	tx.MustExec(`drop table patron_invoices`)
	tx.MustExec(`drop table patron_credits`)
	tx.MustExec(`drop table custom_blog_requests`)
	tx.MustExec(`drop type custom_blog_request_status`)
}
