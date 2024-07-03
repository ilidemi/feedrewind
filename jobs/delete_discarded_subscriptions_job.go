package jobs

import (
	"context"
	"feedrewind/db/pgw"
	"feedrewind/oops"
	"feedrewind/util/schedule"
	"time"
)

func init() {
	registerJobNameFunc(
		"DeleteDiscardedSubscriptionsJob",
		func(ctx context.Context, id JobId, conn *pgw.Conn, args []any) error {
			if len(args) != 0 {
				return oops.Newf("Expected 0 args, got %d: %v", len(args), args)
			}

			return DeleteDiscardedSubscriptionsJob_Perform(ctx, conn)
		},
	)
}

func DeleteDiscardedSubscriptionsJob_PerformAt(tx pgw.Queryable, runAt schedule.Time) error {
	return performAt(tx, runAt, "DeleteDiscardedSubscriptionsJob", defaultQueue)
}

func DeleteDiscardedSubscriptionsJob_Perform(ctx context.Context, conn *pgw.Conn) error {
	logger := conn.Logger()
	utcNow := schedule.UTCNow()
	cutoffTime := utcNow.Add(-45 * 24 * time.Hour)

	result, err := conn.Exec(`delete from subscriptions_with_discarded where discarded_at < $1`, cutoffTime)
	if err != nil {
		return err
	}

	logger.Info().Msgf("Deleted %d subscriptions", result.RowsAffected())

	tomorrow := utcNow.Add(24 * time.Hour)
	runAt := tomorrow.BeginningOfDayIn(time.UTC)
	err = DeleteDiscardedSubscriptionsJob_PerformAt(conn, runAt)
	if err != nil {
		return err
	}

	return nil
}
