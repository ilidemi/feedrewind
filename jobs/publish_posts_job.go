package jobs

import (
	"context"
	"errors"
	"feedrewind/db/pgw"
	"feedrewind/log"
	"feedrewind/models"
	"feedrewind/oops"
	"feedrewind/publish"
	"feedrewind/third_party/tzdata"
	"feedrewind/util"
	"feedrewind/util/schedule"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

func init() {
	registerJobNameFunc(
		"PublishPostsJob",
		func(ctx context.Context, id JobId, pool *pgw.Pool, args []any) error {
			if len(args) != 3 && len(args) != 4 {
				return oops.Newf("Expected 3 or 4 args, got %d: %v", len(args), args)
			}

			userIdInt64, ok := args[0].(int64)
			if !ok {
				userIdInt, ok := args[0].(int)
				if !ok {
					return oops.Newf("Failed to parse userId (expected int64 or int): %v", args[0])
				}
				userIdInt64 = int64(userIdInt)
			}
			userId := models.UserId(userIdInt64)

			dateStr, ok := args[1].(string)
			if !ok {
				return oops.Newf("Failed to parse date (expected string): %v", args[1])
			}
			date := schedule.Date(dateStr)

			scheduledForStr, ok := args[2].(string)
			if !ok {
				return oops.Newf("Failed to parse scheduledForStr (expected string): %v", args[2])
			}

			isManual := false
			if len(args) == 4 {
				var ok bool
				isManual, ok = args[3].(bool)
				if !ok {
					return oops.Newf("Failed to parse isManual (expected boolean): %v", args[3])
				}
			}

			return PublishPostsJob_Perform(ctx, pool, userId, date, scheduledForStr, isManual)
		},
	)
}

func PublishPostsJob_PerformAt(
	qu pgw.Queryable, runAt schedule.Time, userId models.UserId, date schedule.Date, scheduledForStr string,
	isManual bool,
) error {
	return performAt(
		qu, runAt, "PublishPostsJob", defaultQueue, int64ToYaml(int64(userId)), strToYaml(string(date)),
		strToYaml(scheduledForStr), boolToYaml(isManual),
	)
}

func PublishPostsJob_Perform(
	ctx context.Context, pool *pgw.Pool, userId models.UserId, date schedule.Date, scheduledForStr string,
	isManual bool,
) error {
	logger := pool.Logger()
	userSettings, err := models.UserSettings_Get(pool, userId)
	if errors.Is(err, pgx.ErrNoRows) {
		logger.Info().Msg("User not found")
		return nil
	} else if err != nil {
		return err
	}

	if !isManual {
		row := pool.QueryRow(`
			select count(*) from subscriptions_without_discarded
			join (
				select subscription_id, count(*) from subscription_posts
				where published_at_local_date = $1
				group by subscription_id
			) as subscription_posts on subscriptions_without_discarded.id = subscription_posts.subscription_id
			where user_id = $2 and subscription_posts.count > 0
		`, date, userId)
		var publishedSubscriptionsCount int
		err := row.Scan(&publishedSubscriptionsCount)
		if err != nil {
			return err
		}

		if publishedSubscriptionsCount > 0 {
			logger.Warn().Msg(
				"Already published posts for this user today, looks like double schedule? Breaking the cycle.",
			)
			return nil
		}
	}

	err = util.Tx(pool, func(tx *pgw.Tx, pool util.Clobber) error {
		utcNow := schedule.UTCNow()
		location := tzdata.LocationByName[userSettings.Timezone]
		localTime := utcNow.In(location)
		localDate := localTime.Date()
		if date >= localDate {
			productUserId, err := models.User_GetProductUserId(tx, userId)
			if err != nil {
				return err
			}

			err = publish.PublishForUser(
				tx, userId, productUserId, *userSettings.MaybeDeliveryChannel, utcNow, localTime, localDate,
				scheduledForStr,
			)
			if err != nil {
				return err
			}
		} else {
			logger.Warn().Msgf(
				"Today's local date was supposed to be %s but it's %s (%s). Skipping today's update.",
				date, localDate, localTime,
			)
		}

		if !isManual {
			hourOfDay := PublishPostsJob_GetHourOfDay(*userSettings.MaybeDeliveryChannel)
			nextDate := date.NextDay()
			jobTime, err := nextDate.TimeIn(location)
			if err != nil {
				return err
			}
			nextRun := jobTime.Add(time.Duration(hourOfDay) * time.Hour).UTC()
			daysSkipped := 0
			for nextRun.Before(utcNow) {
				daysSkipped++
				nextDate = nextDate.NextDay()
				jobTime, err := nextDate.TimeIn(location)
				if err != nil {
					return err
				}
				nextRun = jobTime.Add(time.Duration(hourOfDay) * time.Hour).UTC()
			}
			if daysSkipped > 0 {
				logger.Warn().Msgf("Skipping %d days", daysSkipped)
			}

			nextScheduledFor := nextRun.MustUTCString()
			err = PublishPostsJob_PerformAt(tx, nextRun, userId, nextDate, nextScheduledFor, false)
			if err != nil {
				return err
			}
		}

		return nil
	})

	return err
}

func PublishPostsJob_ScheduleInitial(
	qu pgw.Queryable, userId models.UserId, userSettings *models.UserSettings, isManual bool,
) error {
	utcNow := schedule.UTCNow()
	location := tzdata.LocationByName[userSettings.Timezone]
	date := utcNow.BeginningOfDayIn(location).AddDate(0, 0, -1)
	hourOfDay := PublishPostsJob_GetHourOfDay(*userSettings.MaybeDeliveryChannel)
	nextRun := date.Add(time.Duration(hourOfDay) * time.Hour).UTC()
	for nextRun.Before(utcNow) {
		date = date.AddDate(0, 0, 1)
		nextRun = date.Add(time.Duration(hourOfDay) * time.Hour).UTC()
	}
	nextRunDate := nextRun.MustUTCString()

	return PublishPostsJob_PerformAt(qu, nextRun, userId, date.Date(), nextRunDate, isManual)
}

func PublishPostsJob_Delete(
	ctx context.Context, qu pgw.Queryable, userId models.UserId, logger log.Logger,
) error {
	var attempt int
	for attempt = 0; attempt < 3; attempt++ {
		tx, err := qu.Begin()
		if err != nil {
			return err
		}

		result, err := tx.Exec(`
			delete from delayed_jobs
			where (handler like concat(E'%class: PublishPostsJob\n%'))
				and handler like concat(E'%arguments:\n  - ', $1::text, E'\n%')
		`, fmt.Sprint(userId))
		if err != nil {
			if err := tx.Rollback(); err != nil {
				return err
			}
			return err
		}
		jobsDeleted := result.RowsAffected()
		if jobsDeleted > 1 {
			logger.Info().Msgf("Expected to delete 0-1 PublishPostsJob, got %d; retrying", jobsDeleted)
			if err := tx.Rollback(); err != nil {
				return err
			}

			if err := util.Sleep(ctx, time.Second); err != nil {
				return err
			}
			continue
		}

		if err := tx.Commit(); err != nil {
			return err
		}
		logger.Info().Msgf("Deleted PublishPostsJob for user %d", userId)
		return nil
	}

	return oops.Newf("Couldn't delete PublishPostsJob after %d attempts", attempt)
}

type LockedPublishPostsJob struct {
	Id       JobId
	LockedBy string
}

func PublishPostsJob_Lock(qu pgw.Queryable, userId models.UserId) ([]LockedPublishPostsJob, error) {
	rows, err := qu.Query(`
		select id, locked_by
		from delayed_jobs
		where (handler like concat(E'%class: PublishPostsJob\n%'))
			and handler like concat(E'%arguments:\n  - ', $1::text, E'\n%')
		for update
	`, fmt.Sprint(userId))
	if err != nil {
		return nil, err
	}

	var locks []LockedPublishPostsJob
	for rows.Next() {
		var lock LockedPublishPostsJob
		var lockedBy *string
		err := rows.Scan(&lock.Id, &lockedBy)
		if err != nil {
			return nil, err
		}
		if lockedBy != nil {
			lock.LockedBy = *lockedBy
		}
		locks = append(locks, lock)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return locks, nil
}

// Returns zero value if the job was not found
func PublishPostsJob_GetNextScheduledDate(qu pgw.Queryable, userId models.UserId) (schedule.Date, error) {
	row := qu.QueryRow(`
		select (regexp_match(handler, concat(E'arguments:\n  - ', $1::text, E'\n  - ''([0-9-]+)''')))[1]
		from delayed_jobs
		where handler like concat(E'%class: PublishPostsJob\n%') and
			handler like concat(E'%arguments:\n  - ', $1::text, E'\n%')
		order by run_at desc
	`, fmt.Sprint(userId))
	var date schedule.Date
	err := row.Scan(&date)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	} else if err != nil {
		return "", err
	}

	return date, nil
}

func PublishPostsJob_GetHourOfDay(deliveryChannel models.DeliveryChannel) int {
	switch deliveryChannel {
	case models.DeliveryChannelEmail:
		return 5
	case models.DeliveryChannelMultipleFeeds, models.DeliveryChannelSingleFeed:
		return 2
	default:
		panic(fmt.Errorf("UnknownDeliveryChannel: %s", deliveryChannel))
	}
}

func PublishPostsJob_UpdateRunAt(qu pgw.Queryable, jobId JobId, runAt schedule.Time) error {
	_, err := qu.Exec(`
		update delayed_jobs set run_at = $1 where id = $2
	`, runAt, jobId)
	return err
}

func PublishPostsJob_IsScheduledForDate(
	qu pgw.Queryable, userId models.UserId, date schedule.Date,
) (bool, error) {
	row := qu.QueryRow(`
		select count(1)
		from delayed_jobs
		where handler like concat(E'%class: PublishPostsJob\n%') and
			handler like concat(E'%arguments:\n  - ', $1::text, E'\n  - ''', $2::text, '''%')
	`, fmt.Sprint(userId), date)
	var jobsCount int
	err := row.Scan(&jobsCount)
	if err != nil {
		return false, err
	}
	return jobsCount == 1, nil
}
