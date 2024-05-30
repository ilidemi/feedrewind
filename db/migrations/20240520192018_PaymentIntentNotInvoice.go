package migrations

type PaymentIntentNotInvoice struct{}

func init() {
	registerMigration(&PaymentIntentNotInvoice{})
}

func (m *PaymentIntentNotInvoice) Version() string {
	return "20240520192018"
}

func (m *PaymentIntentNotInvoice) Up(tx *Tx) {
	tx.MustExec(`
		alter table custom_blog_requests rename column stripe_invoice_id to stripe_payment_intent_id
	`)
}

func (m *PaymentIntentNotInvoice) Down(tx *Tx) {
	tx.MustExec(`
		alter table custom_blog_requests rename column stripe_payment_intent_id to stripe_invoice_id
	`)
}
