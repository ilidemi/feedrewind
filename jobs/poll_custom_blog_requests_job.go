package jobs

import (
	"context"
	"fmt"
	"time"

	"feedrewind.com/db/pgw"
	"feedrewind.com/models"
	"feedrewind.com/oops"
	"feedrewind.com/third_party/tzdata"
	"feedrewind.com/util/schedule"
)

func init() {
	registerJobNameFunc(
		"PollCustomBlogRequestsJob",
		func(ctx context.Context, id JobId, pool *pgw.Pool, args []any) error {
			if len(args) != 0 {
				return oops.Newf("Expected 0 args, got %d: %v", len(args), args)
			}

			return PollCustomBlogRequestsJob_Perform(ctx, pool)
		},
	)
}

func PollCustomBlogRequestsJob_PerformAt(qu pgw.Queryable, runAt schedule.Time) error {
	return performAt(qu, runAt, "PollCustomBlogRequestsJob", defaultQueue)
}

func PollCustomBlogRequestsJob_Perform(ctx context.Context, pool *pgw.Pool) error {
	logger := pool.Logger()

	rows, err := pool.Query(`select id from custom_blog_requests where fulfilled_at is null`)
	if err != nil {
		return err
	}
	var ids []models.CustomBlogRequestId
	for rows.Next() {
		var id models.CustomBlogRequestId
		err := rows.Scan(&id)
		if err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	if len(ids) > 0 {
		message := fmt.Sprintf("Found unfulfilled custom blog requests: %d %v", len(ids), ids)
		logger.Warn().Msg(message)
		err = NotifySlackJob_PerformNow(pool, message)
		if err != nil {
			return err
		}
	}

	tomorrow := schedule.UTCNow().Add(24 * time.Hour)
	pst := tzdata.LocationByName["America/Los_Angeles"]
	runAt := tomorrow.BeginningOfDayIn(pst).Add(8 * time.Hour)
	err = PollCustomBlogRequestsJob_PerformAt(pool, runAt)
	if err != nil {
		return err
	}

	return nil
}
