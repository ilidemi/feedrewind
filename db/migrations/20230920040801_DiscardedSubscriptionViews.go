package migrations

import "feedrewind/db/pgw"

type DiscardedSubscriptionViews struct{}

func init() {
	registerMigration(&DiscardedSubscriptionViews{})
}

func (m *DiscardedSubscriptionViews) Version() string {
	return "20230920040801"
}

func (m *DiscardedSubscriptionViews) Up(tx *Tx) {
	pgw.CheckSubscriptionsUsage = false
	tx.MustExec(`
		create view subscriptions_with_discarded as
			select * from subscriptions
		with cascaded check option
	`)
	tx.MustExec(`
		create view subscriptions_without_discarded as
			select * from subscriptions
			where discarded_at is null
		with cascaded check option
	`)
	pgw.CheckSubscriptionsUsage = true
}

func (m *DiscardedSubscriptionViews) Down(tx *Tx) {
	tx.MustExec("drop view subscriptions_with_discarded")
	tx.MustExec("drop view subscriptions_without_discarded")
}
