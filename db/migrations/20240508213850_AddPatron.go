package migrations

type AddPatron struct{}

func init() {
	registerMigration(&AddPatron{})
}

func (m *AddPatron) Version() string {
	return "20240508213850"
}

func (m *AddPatron) Up(tx *Tx) {
	tx.MustExec(`
		insert into pricing_offers (id, monthly_rate, yearly_rate, plan_id)
		values ('patron_2024-04-16', '160.00', '1600.00', 'free')
	`)
	tx.MustExec(`
		insert into pricing_plans (id, default_offer_id)
		values ('patron', 'patron_2024-04-16')
	`)
	tx.MustExec(`
		update pricing_offers set plan_id = 'patron' where id = 'patron_2024-04-16'
	`)
}

func (m *AddPatron) Down(tx *Tx) {
	tx.MustExec(`update pricing_offers set plan_id = 'free' where id = 'patron_2024-04-16'`)
	tx.MustExec(`delete from pricing_plans where id = 'patron'`)
	tx.MustExec(`delete from pricing_offers where id = 'patron_2024-04-16`)
}
