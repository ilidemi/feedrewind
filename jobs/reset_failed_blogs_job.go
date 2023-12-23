package jobs

import (
	"context"
	"feedrewind/db/pgw"
	"feedrewind/models"
	"feedrewind/oops"
	"feedrewind/util/schedule"
	"strings"
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
	logger := conn.Logger()
	utcNow := schedule.UTCNow()
	cutoffTime := utcNow.Add(-30 * 24 * time.Hour)

	var sb strings.Builder
	for status := range models.BlogFailedStatuses {
		if sb.Len() > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString("'")
		sb.WriteString(string(status))
		sb.WriteString("'")
	}
	rows, err := conn.Query(`
		select id, feed_url from blogs
		where status in (`+sb.String()+`) and
			version = $1 and
			status_updated_at < $2
	`, models.BlogLatestVersion, cutoffTime)
	if err != nil {
		return err
	}

	var blogIds []models.BlogId
	var feedUrls []string
	for rows.Next() {
		var blogId models.BlogId
		var feedUrl string
		err := rows.Scan(&blogId, &feedUrl)
		if err != nil {
			return err
		}
		blogIds = append(blogIds, blogId)
		feedUrls = append(feedUrls, feedUrl)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	logger.Info().Msgf("Resetting %d failed blogs", len(blogIds))
	for i, blogId := range blogIds {
		newVersion, err := models.Blog_Downgrade(conn, blogId)
		if err != nil {
			return err
		}

		logger.Info().Msgf("Blog %d (%s) -> new version %d", blogId, feedUrls[i], newVersion)
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
