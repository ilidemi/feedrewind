package migrations

import "feedrewind/db/pgw"

type UpdateUserViews struct{}

func init() {
	registerMigration(&UpdateUserViews{})
}

func (m *UpdateUserViews) Version() string {
	return "20240425220339"
}

func (m *UpdateUserViews) Up(tx *Tx) {
	pgw.CheckUsersUsage = false
	tx.MustExec(`
		create or replace view users_with_discarded as
			select * from users
		with cascaded check option
	`)
	tx.MustExec(`
		create or replace view users_without_discarded as
			select * from users
			where users.discarded_at is null
		with cascaded check option
	`)
	pgw.CheckUsersUsage = true
}

func (m *UpdateUserViews) Down(tx *Tx) {
	pgw.CheckUsersUsage = false
	tx.MustExec(`
		create or replace view users_with_discarded as
			select * from users
		with cascaded check option
	`)
	tx.MustExec(`
		create or replace view users_without_discarded as
			select * from users
			where users.discarded_at is null
		with cascaded check option
	`)
	pgw.CheckUsersUsage = true
}
