package jobs

import (
	"context"
	"feedrewind/db/migrations"
	"feedrewind/db/pgw"
	"feedrewind/oops"
	"feedrewind/util/schedule"
	"strings"
	"time"

	"github.com/stripe/stripe-go/v78"
	"github.com/stripe/stripe-go/v78/subscription"
)

func init() {
	registerJobNameFunc(
		"CheckStaleStripeJob", func(ctx context.Context, conn *pgw.Conn, args []any) error {
			if len(args) != 0 {
				return oops.Newf("Expected 0 args, got %d: %v", len(args), args)
			}

			return CheckStaleStripeJob_Perform(ctx, conn)
		},
	)
	migrations.CheckStaleStripeJob_PerformAtFunc = CheckStaleStripeJob_PerformAt
}

func CheckStaleStripeJob_PerformAt(tx pgw.Queryable, runAt schedule.Time) error {
	return performAt(tx, runAt, "CheckStaleStripeJob", defaultQueue)
}

func CheckStaleStripeJob_Perform(ctx context.Context, conn *pgw.Conn) error {
	logger := conn.Logger()
	utcNow := schedule.UTCNow()

	subscriptionCutoff := utcNow.Add(-24 * time.Hour)
	var subscriptionIds []string
	//nolint:exhaustruct
	iter := subscription.List(&stripe.SubscriptionListParams{
		ListParams: stripe.ListParams{
			Context: ctx,
		},
		Status:       stripe.String("active"),
		CreatedRange: &stripe.RangeQueryParams{LesserThan: subscriptionCutoff.Unix()},
	})
	for iter.Next() {
		sub := iter.Subscription()
		subscriptionIds = append(subscriptionIds, sub.ID)
	}
	if err := iter.Err(); err != nil {
		return oops.Wrap(err)
	}

	var missingSubscriptionIds []string
	batchSize := 100
	for i := 0; i < len(subscriptionIds); i += batchSize {
		j := i + batchSize
		if j > len(subscriptionIds) {
			j = len(subscriptionIds)
		}
		batch := subscriptionIds[i:j]
		batchSet := map[string]bool{}
		for _, subscriptionId := range batch {
			batchSet[subscriptionId] = true
		}
		batchStr := strings.Join(batch, "', '")
		rows, err := conn.Query(`
			select stripe_subscription_id from users_without_discarded
			where stripe_subscription_id in ('` + batchStr + `')
		`)
		if err != nil {
			return err
		}
		for rows.Next() {
			var subscriptionId string
			err := rows.Scan(&subscriptionId)
			if err != nil {
				return err
			}
			delete(batchSet, subscriptionId)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		for subscriptionId := range batchSet {
			missingSubscriptionIds = append(missingSubscriptionIds, subscriptionId)
		}
	}
	if len(missingSubscriptionIds) > 0 {
		logger.Warn().Msgf(
			"Found %d hanging Stripe subscriptions: %v", len(missingSubscriptionIds), missingSubscriptionIds,
		)
	}

	tokenCutoff := utcNow.Add(-7 * 24 * time.Hour)
	row := conn.QueryRow(`select count(1) from stripe_subscription_tokens where created_at < $1`, tokenCutoff)
	var staleCount int
	err := row.Scan(&staleCount)
	if err != nil {
		return err
	}
	if staleCount > 0 {
		logger.Warn().Msgf("Found %d stale stripe_subscription_tokens", staleCount)
	}

	hourFromNow := utcNow.Add(time.Hour)
	runAt := hourFromNow.BeginningOfHour()
	err = CheckStaleStripeJob_PerformAt(conn, runAt)
	if err != nil {
		return err
	}

	return nil
}
