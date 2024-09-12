package jobs

import (
	"context"
	"feedrewind/db/pgw"
	"feedrewind/models"
	"feedrewind/oops"
	"feedrewind/third_party/tzdata"
	"feedrewind/util/schedule"
	"fmt"
	"time"
)

func init() {
	registerJobNameFunc(
		"PollCustomBlogRequestsJob",
		false,
		func(ctx context.Context, id JobId, conn *pgw.Conn, args []any) error {
			if len(args) != 0 {
				return oops.Newf("Expected 0 args, got %d: %v", len(args), args)
			}

			return PollCustomBlogRequestsJob_Perform(ctx, conn)
		},
	)
}

func PollCustomBlogRequestsJob_PerformAt(tx pgw.Queryable, runAt schedule.Time) error {
	return performAt(tx, runAt, "PollCustomBlogRequestsJob", defaultQueue)
}

func PollCustomBlogRequestsJob_Perform(ctx context.Context, conn *pgw.Conn) error {
	logger := conn.Logger()

	rows, err := conn.Query(`select id from custom_blog_requests where fulfilled_at is null`)
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
		err = NotifySlackJob_PerformNow(conn, message)
		if err != nil {
			return err
		}
	}

	tomorrow := schedule.UTCNow().Add(24 * time.Hour)
	pst := tzdata.LocationByName["America/Los_Angeles"]
	runAt := tomorrow.BeginningOfDayIn(pst).Add(8 * time.Hour)
	err = PollCustomBlogRequestsJob_PerformAt(conn, runAt)
	if err != nil {
		return err
	}

	return nil
}
