package jobs

import (
	"context"
	"errors"
	"feedrewind/db/pgw"
	"feedrewind/oops"
	"feedrewind/util/schedule"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stripe/stripe-go/v78"
	"github.com/stripe/stripe-go/v78/checkout/session"
	"github.com/stripe/stripe-go/v78/subscription"
)

func init() {
	registerJobNameFunc(
		"CheckStaleStripeJob",
		func(ctx context.Context, id JobId, pool *pgw.Pool, args []any) error {
			if len(args) != 0 {
				return oops.Newf("Expected 0 args, got %d: %v", len(args), args)
			}

			return CheckStaleStripeJob_Perform(ctx, pool)
		},
	)
}

func CheckStaleStripeJob_PerformAt(qu pgw.Queryable, runAt schedule.Time) error {
	return performAt(qu, runAt, "CheckStaleStripeJob", defaultQueue)
}

func CheckStaleStripeJob_Perform(ctx context.Context, pool *pgw.Pool) error {
	logger := pool.Logger()
	utcNow := schedule.UTCNow()

	subscriptionCutoff := utcNow.Add(-24 * time.Hour)
	var subscriptionIds []string
	//nolint:exhaustruct
	subscriptionIter := subscription.List(&stripe.SubscriptionListParams{
		ListParams: stripe.ListParams{
			Context: ctx,
		},
		Status:       stripe.String("active"),
		CreatedRange: &stripe.RangeQueryParams{LesserThan: subscriptionCutoff.Unix()},
	})
	for subscriptionIter.Next() {
		sub := subscriptionIter.Subscription()
		subscriptionIds = append(subscriptionIds, sub.ID)
	}
	if err := subscriptionIter.Err(); err != nil {
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
		rows, err := pool.Query(`
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
	row := pool.QueryRow(`select count(1) from stripe_subscription_tokens where created_at < $1`, tokenCutoff)
	var staleCount int
	err := row.Scan(&staleCount)
	if err != nil {
		return err
	}
	if staleCount > 0 {
		logger.Warn().Msgf("Found %d stale stripe_subscription_tokens", staleCount)
	}

	session.List(&stripe.CheckoutSessionListParams{}) //nolint:exhaustruct

	customBlogSessionCutoff := utcNow.Add(-7 * 24 * time.Hour)
	customBlogSessionTooRecent := utcNow.Add(-1 * time.Hour)
	//nolint:exhaustruct
	customBlogSessionIter := session.List(&stripe.CheckoutSessionListParams{
		CreatedRange: &stripe.RangeQueryParams{
			GreaterThanOrEqual: *stripe.Int64(customBlogSessionCutoff.Unix()),
		},
		Status: stripe.String(string(stripe.CheckoutSessionStatusComplete)),
	})
	var hangingCustomBlogSessionIds []string
	for customBlogSessionIter.Next() {
		sesh := customBlogSessionIter.CheckoutSession()
		if sesh.Created > customBlogSessionTooRecent.Unix() {
			continue
		}
		if _, ok := sesh.Metadata["subscription_id"]; !ok {
			continue
		}
		if sesh.PaymentIntent == nil {
			continue
		}
		row := pool.QueryRow(`
			select 1 from custom_blog_requests
			where stripe_payment_intent_id = $1
		`, sesh.PaymentIntent.ID)
		var one int
		err := row.Scan(&one)
		if errors.Is(err, pgx.ErrNoRows) {
			hangingCustomBlogSessionIds = append(hangingCustomBlogSessionIds, sesh.ID)
		} else if err != nil {
			return err
		}
	}
	if err := customBlogSessionIter.Err(); err != nil {
		return oops.Wrap(err)
	}
	if len(hangingCustomBlogSessionIds) > 0 {
		logger.Warn().Msgf(
			"Found %d hanging custom blog request sessions: %v",
			len(hangingCustomBlogSessionIds), hangingCustomBlogSessionIds,
		)
	}

	hourFromNow := utcNow.Add(time.Hour)
	runAt := hourFromNow.BeginningOfHour()
	err = CheckStaleStripeJob_PerformAt(pool, runAt)
	if err != nil {
		return err
	}

	return nil
}
