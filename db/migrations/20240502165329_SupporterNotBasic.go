package migrations

type SupporterNotBasic struct{}

func init() {
	registerMigration(&SupporterNotBasic{})
}

func (m *SupporterNotBasic) Version() string {
	return "20240502165329"
}

func (m *SupporterNotBasic) Up(tx *Tx) {
	tx.MustExec(`alter table pricing_offers add constraint pricing_offers_plan_id_fkey foreign key (plan_id) references pricing_plans(id)`)
}

func (m *SupporterNotBasic) Down(tx *Tx) {
	tx.MustExec(`alter table pricing_offers drop constraint pricing_offers_plan_id_fkey`)
}
