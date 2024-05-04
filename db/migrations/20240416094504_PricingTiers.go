package migrations

import "feedrewind/db/pgw"

type PricingTiers struct{}

func init() {
	registerMigration(&PricingTiers{})
}

func (m *PricingTiers) Version() string {
	return "20240416094504"
}

func (m *PricingTiers) Up(tx *Tx) {
	tx.MustExec(`
		create table pricing_tiers (
			id text primary key
		)
	`)
	tx.MustExec(`
		create table pricing_offers (
			id text primary key,
			monthly_rate money not null,
			yearly_rate money not null,
			stripe_product_id text,
			tier_id text references pricing_tiers(id) not null
		)
	`)
	tx.MustExec(`insert into pricing_tiers (id) values ('free')`)
	tx.MustExec(`insert into pricing_tiers (id) values ('supporter')`)
	tx.MustExec(`
		insert into pricing_offers (id, monthly_rate, yearly_rate, stripe_product_id, tier_id)
		values ('free_2024-04-16', 0, 0, null, 'free')
	`)
	tx.MustExec(`
		insert into pricing_offers (id, monthly_rate, yearly_rate, stripe_product_id, tier_id)
		values ('supporter_2024-04-16', 6, 60, null, 'supporter')
	`)
	tx.MustExec(`
		alter table pricing_tiers add column default_offer_id text references pricing_offers(id)
	`)
	tx.MustExec(`update pricing_tiers set default_offer_id = 'free_2024-04-16' where id = 'free'`)
	tx.MustExec(`update pricing_tiers set default_offer_id = 'supporter_2024-04-16' where id = 'supporter'`)
	tx.MustExec(`alter table pricing_tiers alter column default_offer_id set not null`)

	tx.MustExec(`alter table users add column offer_id text references pricing_offers(id)`)
	tx.MustExec(`update users set offer_id = 'free_2024-04-16'`)
	tx.MustExec(`alter table users alter column offer_id set not null`)

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

func (m *PricingTiers) Down(tx *Tx) {
	tx.MustExec(`alter table users drop column offer_id`)

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

	tx.MustExec(`alter table pricing_tiers drop column default_offer_id`)
	tx.MustExec(`drop table pricing_offers`)
	tx.MustExec(`drop table pricing_tiers`)
}
