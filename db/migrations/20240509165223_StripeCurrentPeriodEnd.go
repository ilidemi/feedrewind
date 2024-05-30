package migrations

import (
	"feedrewind/db/pgw"
	"feedrewind/models"
	"fmt"
	"time"

	"github.com/stripe/stripe-go/v78/subscription"
)

type StripeCurrentPeriodEnd struct{}

func init() {
	registerMigration(&StripeCurrentPeriodEnd{})
}

func (m *StripeCurrentPeriodEnd) Version() string {
	return "20240509165223"
}

func (m *StripeCurrentPeriodEnd) Up(tx *Tx) {
	tx.MustExec(`alter table users add column stripe_current_period_end timestamp without time zone`)
	tx.MustUpdateDiscardedViews("users", &pgw.CheckUsersUsage)
	rows, err := tx.impl.Query(`
		select id, stripe_subscription_id from users_with_discarded where stripe_subscription_id is not null
	`)
	if err != nil {
		panic(err)
	}
	type UserSub struct {
		UserId               models.UserId
		StripeSubscriptionId string
	}
	var userSubs []UserSub
	for rows.Next() {
		var us UserSub
		err := rows.Scan(&us.UserId, &us.StripeSubscriptionId)
		if err != nil {
			panic(err)
		}
		userSubs = append(userSubs, us)
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}
	fmt.Printf("Populating stripe_current_period_end for %d users\n", len(userSubs))

	for _, us := range userSubs {
		sub, err := subscription.Get(us.StripeSubscriptionId, nil)
		if err != nil {
			panic(err)
		}
		currentPeriodEnd := time.Unix(sub.CurrentPeriodEnd, 0).UTC()
		tx.MustExec(`
			update users set stripe_current_period_end = $1 where id = $2
		`, currentPeriodEnd, us.UserId)
		fmt.Printf("%d: %v\n", us.UserId, currentPeriodEnd)
	}
}

func (m *StripeCurrentPeriodEnd) Down(tx *Tx) {
	tx.MustExec(`alter table users drop column stripe_current_period_end`)
}
