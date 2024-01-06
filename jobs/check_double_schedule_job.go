package jobs

import (
	"context"
	"feedrewind/db/migrations"
	"feedrewind/db/pgw"
	"feedrewind/oops"
	"feedrewind/util/schedule"
	"time"
)

func init() {
	registerJobNameFunc("CheckDoubleScheduleJob",
		func(ctx context.Context, conn *pgw.Conn, args []any) error {
			if len(args) != 0 {
				return oops.Newf("Expected 0 args, got %d: %v", len(args), args)
			}

			return CheckDoubleScheduleJob_Perform(ctx, conn)
		},
	)

	migrations.CheckDoubleScheduleJob_PerformAtFunc = CheckDoubleScheduleJob_PerformAt
}

func CheckDoubleScheduleJob_PerformAt(tx pgw.Queryable, runAt schedule.Time) error {
	return performAt(tx, runAt, "CheckDoubleScheduleJob", defaultQueue)
}

func CheckDoubleScheduleJob_Perform(ctx context.Context, conn *pgw.Conn) error {
	logger := conn.Logger()

	rows, err := conn.Query(`
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

	hourFromNow := schedule.UTCNow().Add(time.Hour)
	// Stagger from the PublishPostsJobs that run on the hour
	runAt := hourFromNow.BeginningOfHour().Add(30 * time.Minute)
	err = CheckDoubleScheduleJob_PerformAt(conn, runAt)
	if err != nil {
		return err
	}

	return nil
}
