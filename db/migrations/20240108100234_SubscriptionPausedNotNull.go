package migrations

type SubscriptionPausedNotNull struct{}

func init() {
	registerMigration(&SubscriptionPausedNotNull{})
}

func (m *SubscriptionPausedNotNull) Version() string {
	return "20240108100234"
}

func (m *SubscriptionPausedNotNull) Up(tx *Tx) {
	tx.MustExec(`update subscriptions_with_discarded set is_paused = false where is_paused is null`)
	tx.MustExec(`alter table subscriptions alter column is_paused set not null`)
}

func (m *SubscriptionPausedNotNull) Down(tx *Tx) {
	tx.MustExec(`alter table subscriptions alter column is_paused drop not null`)
}
