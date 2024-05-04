package migrations

type PlanNotTier struct{}

func init() {
	registerMigration(&PlanNotTier{})
}

func (m *PlanNotTier) Version() string {
	return "20240501133956"
}

func (m *PlanNotTier) Up(tx *Tx) {
	tx.MustExec(`alter table pricing_tiers rename to pricing_plans`)
	tx.MustExec(`alter table pricing_plans rename constraint pricing_tiers_pkey to pricing_plans_pkey`)
	tx.MustExec(`
		alter table pricing_plans
		rename constraint pricing_tiers_default_offer_id_fkey to pricing_plans_default_offer_id_fkey
	`)

	tx.MustExec(`alter table pricing_offers add column plan_id text`)
	tx.MustExec(`update pricing_offers set plan_id = tier_id`)
	tx.MustExec(`alter table pricing_offers alter column plan_id set not null`)
	tx.MustExec(`alter table pricing_offers drop column tier_id`)
}

func (m *PlanNotTier) Down(tx *Tx) {
	tx.MustExec(`alter table pricing_plans rename to pricing_tiers`)
	tx.MustExec(`alter table pricing_tiers rename constraint pricing_plans_pkey to pricing_tiers_pkey`)
	tx.MustExec(`
		alter table pricing_tiers
		rename constraint pricing_plans_default_offer_id_fkey to pricing_tiers_default_offer_id_fkey
	`)

	tx.MustExec(`alter table pricing_offers add column tier_id text`)
	tx.MustExec(`update pricing_offers set tier_id = plan_id`)
	tx.MustExec(`alter table pricing_offers alter column tier_id set not null`)
	tx.MustExec(`alter table pricing_offers drop column plan_id`)
}
