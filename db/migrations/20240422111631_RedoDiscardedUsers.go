package migrations

import "feedrewind/db/pgw"

type RedoDiscardedUsers struct{}

func init() {
	registerMigration(&RedoDiscardedUsers{})
}

func (m *RedoDiscardedUsers) Version() string {
	return "20240422111631"
}

func (m *RedoDiscardedUsers) Up(tx *Tx) {
	pgw.CheckUsersUsage = false
	tx.MustExec(`alter table users add column stripe_subscription_id text`)
	tx.MustExec(`alter table users add column stripe_customer_id text`)
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

func (m *RedoDiscardedUsers) Down(tx *Tx) {
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
	tx.MustExec(`alter table users drop column stripe_subscription_id`)
	tx.MustExec(`alter table users drop column stripe_customer_id`)
	pgw.CheckUsersUsage = true
}
