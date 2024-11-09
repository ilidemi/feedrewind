package jobs

import (
	"context"
	"errors"
	"strings"
	"time"

	"feedrewind.com/config"
	"feedrewind.com/db/migrations"
	"feedrewind.com/db/pgw"
	"feedrewind.com/models"
	"feedrewind.com/oops"
	"feedrewind.com/util/schedule"

	"github.com/jackc/pgx/v5"
	"github.com/stripe/stripe-go/v78"
	"github.com/stripe/stripe-go/v78/checkout/session"
	"github.com/stripe/stripe-go/v78/subscription"
)

func init() {
	registerJobNameFunc(
		"MaintenanceJob",
		func(ctx context.Context, id JobId, pool *pgw.Pool, args []any) error {
			if len(args) != 0 {
				return oops.Newf("Expected 0 args, got %d: %v", len(args), args)
			}

			return MaintenanceJob_Perform(ctx, pool)
		},
	)
	migrations.MaintenanceJob_PerformAtFunc = MaintenanceJob_PerformAt
}

func MaintenanceJob_PerformAt(qu pgw.Queryable, runAt schedule.Time) error {
	return performAt(qu, runAt, "MaintenanceJob", defaultQueue)
}

var warnedActiveUserIds = map[models.UserId]bool{}

func MaintenanceJob_Perform(ctx context.Context, pool *pgw.Pool) error {
	logger := pool.Logger()

	//
	// Duplicated PublishPostsJob
	//
	rows, err := pool.Query(`
		select array_agg(id) as ids, (
			select regexp_matches(handler, E'arguments:\n  - ([0-9]+)')
		)[1] as user_id
		from delayed_jobs
		where handler like '%PublishPostsJob%'
		group by user_id
		having count(*) > 1
	`)
	if err != nil {
		return err
	}

	for rows.Next() {
		var ids []int64
		var userId string
		err := rows.Scan(&ids, &userId)
		if err != nil {
			return err
		}
		logger.Warn().Msgf("User %s has duplicated PublishPostsJob: %v", userId, ids)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	//
	// Missing PublishPostsJob
	//
	rows, err = pool.Query(`
		select user_id from user_settings
		where delivery_channel is not null
			and user_id not in (
				select (regexp_matches(handler, E'arguments:\n  - ([0-9]+)'))[1]::bigint from delayed_jobs
				where handler like '%PublishPostsJob%'
			)
	`)
	if err != nil {
		return err
	}

	for rows.Next() {
		var userId string
		err := rows.Scan(&userId)
		if err != nil {
			return err
		}
		logger.Warn().Msgf("User %s doesn't have a PublishPostsJob", userId)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	//
	// Check stale stripe
	//
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
	err = row.Scan(&staleCount)
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

	//
	// Check for overactive users
	//
	rows, err = pool.Query(`
		select user_id, (select email from users_without_discarded where id = user_id), count(1) as count
		from subscriptions_with_discarded
		where user_id is not null
		group by user_id, email
		having count(1) >= 30
		order by count desc
	`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var userId models.UserId
		var email string
		var count int64
		err := rows.Scan(&userId, &email, &count)
		if err != nil {
			return err
		}
		if strings.HasSuffix(email, "@feedrewind.com") ||
			strings.HasSuffix(email, "@bounce-testing.postmarkapp.com") ||
			config.Cfg.AdminUserIds[int64(userId)] ||
			warnedActiveUserIds[userId] {
			continue
		}

		logger.Warn().Msgf("User %d has many subscriptions: %d", userId, count)
		warnedActiveUserIds[userId] = true
	}

	hourFromNow := schedule.UTCNow().Add(time.Hour)
	// Stagger from the PublishPostsJobs that run on the hour
	runAt := hourFromNow.BeginningOfHour().Add(30 * time.Minute)
	err = MaintenanceJob_PerformAt(pool, runAt)
	if err != nil {
		return err
	}

	return nil
}
