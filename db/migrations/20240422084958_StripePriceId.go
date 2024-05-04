package migrations

type StripePriceId struct{}

func init() {
	registerMigration(&StripePriceId{})
}

func (m *StripePriceId) Version() string {
	return "20240422084958"
}

func (m *StripePriceId) Up(tx *Tx) {
	tx.MustExec(`alter table pricing_offers add column stripe_monthly_price_id text`)
	tx.MustExec(`alter table pricing_offers add column stripe_yearly_price_id text`)
}

func (m *StripePriceId) Down(tx *Tx) {
	tx.MustExec(`alter table pricing_offers drop column stripe_monthly_price_id`)
	tx.MustExec(`alter table pricing_offers drop column stripe_yearly_price_id`)
}
