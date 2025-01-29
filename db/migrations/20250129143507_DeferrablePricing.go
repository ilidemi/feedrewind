package migrations

type DeferrablePricing struct{}

func init() {
	registerMigration(&DeferrablePricing{})
}

func (m *DeferrablePricing) Version() string {
	return "20250129143507"
}

func (m *DeferrablePricing) Up(tx *Tx) {
	tx.MustExec(`alter table pricing_plans drop constraint pricing_plans_default_offer_id_fkey`)
	tx.MustExec(`
		alter table pricing_plans
		add constraint pricing_plans_default_offer_id_fkey
		foreign key (default_offer_id) references public.pricing_offers(id)
		deferrable initially deferred
	`)
}

func (m *DeferrablePricing) Down(tx *Tx) {
	tx.MustExec(`alter table pricing_plans drop constraint pricing_plans_default_offer_id_fkey`)
	tx.MustExec(`
		alter table pricing_plans
		add constraint pricing_plans_default_offer_id_fkey
		foreign key (default_offer_id) references public.pricing_offers(id)
	`)
}
