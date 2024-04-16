package migrations

import "feedrewind/db/pgw"

type UsersDiscarded struct{}

func init() {
	registerMigration(&UsersDiscarded{})
}

func (m *UsersDiscarded) Version() string {
	return "20240415084139"
}

func (m *UsersDiscarded) Up(tx *Tx) {
	pgw.CheckUsersUsage = false
	tx.MustExec(`
		alter table users add column discarded_at timestamp without time zone
	`)
	tx.MustExec(`
		create view users_with_discarded as
			select * from users
		with cascaded check option
	`)
	tx.MustExec(`
		create view users_without_discarded as
			select * from users
			where users.discarded_at is null
		with cascaded check option
	`)
	pgw.CheckUsersUsage = true
}

func (m *UsersDiscarded) Down(tx *Tx) {
	tx.MustExec(`drop view users_with_discarded`)
	tx.MustExec(`drop view users_without_discarded`)
	tx.MustExec(`alter table users drop column discarded_at`)
}
