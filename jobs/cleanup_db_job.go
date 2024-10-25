package jobs

import (
	"context"
	"feedrewind/db/migrations"
	"feedrewind/db/pgw"
	"feedrewind/models"
	"feedrewind/oops"
	"feedrewind/util"
	"feedrewind/util/schedule"
	"time"
)

func init() {
	registerJobNameFunc(
		"CleanupDbJob",
		func(ctx context.Context, id JobId, pool *pgw.Pool, args []any) error {
			if len(args) != 0 {
				return oops.Newf("Expected 0 args, got %d: %v", len(args), args)
			}

			return CleanupDbJob_Perform(ctx, pool)
		},
	)
	migrations.CleanupDbJob_PerformAtFunc = CleanupDbJob_PerformAt
}

func CleanupDbJob_PerformAt(qu pgw.Queryable, runAt schedule.Time) error {
	return performAt(qu, runAt, "CleanupDbJob", defaultQueue)
}

func CleanupDbJob_Perform(ctx context.Context, pool *pgw.Pool) error {
	logger := pool.Logger()
	utcNow := schedule.UTCNow()
	subscriptionsCutoff := utcNow.Add(-45 * 24 * time.Hour)

	{
		result, err := pool.Exec(`
			delete from subscriptions_with_discarded where discarded_at < $1
		`, subscriptionsCutoff)
		if err != nil {
			return err
		}
		logger.Info().Msgf("Deleted %d stale discarded subscriptions", result.RowsAffected())
	}

	err := util.Tx(pool, func(tx *pgw.Tx, pool util.Clobber) error {
		rows, err := tx.Query(`
			select id from users_with_discarded where discarded_at < $1
		`, subscriptionsCutoff)
		if err != nil {
			return err
		}
		var userIds []models.UserId
		for rows.Next() {
			var userId models.UserId
			err := rows.Scan(&userId)
			if err != nil {
				return err
			}
			userIds = append(userIds, userId)
		}
		if err := rows.Err(); err != nil {
			return err
		}

		if len(userIds) > 0 {
			logger.Info().Msgf("Deleting %d stale discarded users", len(userIds))
		}
		for _, userId := range userIds {
			err := PublishPostsJob_Delete(ctx, tx, userId, logger)
			if err != nil {
				return err
			}
			_, err = tx.Exec(`delete from users_with_discarded where id = $1`, userId)
			if err != nil {
				return err
			}
			logger.Info().Msgf("Deleted user %d", userId)
		}
		logger.Info().Msgf("Deleted %d stale discarded users", len(userIds))
		return nil
	})
	if err != nil {
		return err
	}

	startFeedCutoff := utcNow.Add(-7 * 24 * time.Hour)
	{
		result, err := pool.Exec(`
			delete from start_feeds
			where created_at < $1 and
				id not in (select start_feed_id from blogs where start_feed_id is not null)
		`, startFeedCutoff)
		if err != nil {
			return err
		}
		logger.Info().Msgf("Deleted %d stale start feeds", result.RowsAffected())
	}

	{
		result, err := pool.Exec(`
			delete from start_pages
			where created_at < $1 and
				id not in (select start_page_id from start_feeds where start_page_id is not null)
		`, startFeedCutoff)
		if err != nil {
			return err
		}
		logger.Info().Msgf("Deleted %d stale start pages", result.RowsAffected())
	}

	tomorrow := utcNow.Add(24 * time.Hour)
	runAt := tomorrow.BeginningOfDayIn(time.UTC)
	err = CleanupDbJob_PerformAt(pool, runAt)
	if err != nil {
		return err
	}

	return nil
}
