package jobs

import (
	"context"
	"feedrewind/db/pgw"
	"feedrewind/models"
	"feedrewind/oops"
	"feedrewind/util/schedule"
	"time"
)

func init() {
	registerJobNameFunc("ResetFailedBlogsJob", func(ctx context.Context, conn *pgw.Conn, args []any) error {
		if len(args) != 1 {
			return oops.Newf("Expected 1 arg, got %d: %v", len(args), args)
		}

		enqueueNext, ok := args[0].(bool)
		if !ok {
			return oops.Newf("Failed to parse enqueueNext (expected boolean): %v", args[0])
		}

		return ResetFailedBlogsJob_Perform(ctx, conn, enqueueNext)
	})
}

func ResetFailedBlogsJob_PerformNow(tx pgw.Queryable, enqueueNext bool) error {
	return performNow(tx, "ResetFailedBlogsJob", "reset_failed_blogs", boolToYaml(enqueueNext))
}

func ResetFailedBlogsJob_PerformAt(tx pgw.Queryable, runAt schedule.Time, enqueueNext bool) error {
	return performAt(tx, runAt, "ResetFailedBlogsJob", "reset_failed_blogs", boolToYaml(enqueueNext))
}

func ResetFailedBlogsJob_Perform(ctx context.Context, conn *pgw.Conn, enqueueNext bool) error {
	utcNow := schedule.UTCNow()
	cutoffTime := utcNow.Add(-30 * 24 * time.Hour)

	err := models.Blog_ResetFailed(conn, cutoffTime)
	if err != nil {
		return err
	}

	if enqueueNext {
		tomorrow := utcNow.Add(24 * time.Hour)
		runAt := tomorrow.BeginningOfDayIn(time.UTC)
		err := ResetFailedBlogsJob_PerformAt(conn, runAt, true)
		if err != nil {
			return err
		}
	}

	return nil
}
