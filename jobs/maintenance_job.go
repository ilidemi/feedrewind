package jobs

import (
	"context"
	"strings"
	"time"

	"feedrewind.com/config"
	"feedrewind.com/db/migrations"
	"feedrewind.com/db/pgw"
	"feedrewind.com/models"
	"feedrewind.com/oops"
	"feedrewind.com/util/schedule"
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
