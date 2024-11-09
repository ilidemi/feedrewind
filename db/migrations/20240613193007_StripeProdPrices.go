package migrations

import "feedrewind.com/config"

type StripeProdPrices struct{}

func init() {
	registerMigration(&StripeProdPrices{})
}

func (m *StripeProdPrices) Version() string {
	return "20240613193007"
}

func (m *StripeProdPrices) Up(tx *Tx) {
	if config.Cfg.IsHeroku {
		tx.MustExec(`
			update pricing_offers
			set stripe_product_id = 'prod_QHzrACMEblgxo1',
				stripe_monthly_price_id = 'price_1PRPs804wuefaPiWq9wjns6A',
				stripe_yearly_price_id = 'price_1PRPs804wuefaPiWqiPuzVb0'
			where id = 'patron_2024-04-16'		
		`)
		tx.MustExec(`
			update pricing_offers
			set stripe_product_id = 'prod_PwcHGegDbjkRz2',
				stripe_monthly_price_id = 'price_1P6j3604wuefaPiWZss8RlIR',
				stripe_yearly_price_id = 'price_1P6j3604wuefaPiWdT68ZCua'
			where id = 'supporter_2024-04-16'		
		`)
	}
}

func (m *StripeProdPrices) Down(tx *Tx) {
	if config.Cfg.IsHeroku {
		tx.MustExec(`
			update pricing_offers
			set stripe_product_id = null,
				stripe_monthly_price_id = null,
				stripe_yearly_price_id = null
			where id = 'patron_2024-04-16'		
		`)
		tx.MustExec(`
			update pricing_offers
			set stripe_product_id = null,
				stripe_monthly_price_id = null,
				stripe_yearly_price_id = null
			where id = 'supporter_2024-04-16'		
		`)
	}
}
