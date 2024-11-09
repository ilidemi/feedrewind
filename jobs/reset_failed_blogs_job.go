package jobs

import (
	"context"
	"strings"
	"time"

	"feedrewind.com/db/pgw"
	"feedrewind.com/models"
	"feedrewind.com/oops"
	"feedrewind.com/util/schedule"
)

func init() {
	registerJobNameFunc(
		"ResetFailedBlogsJob",
		func(ctx context.Context, id JobId, pool *pgw.Pool, args []any) error {
			if len(args) != 1 {
				return oops.Newf("Expected 1 arg, got %d: %v", len(args), args)
			}

			enqueueNext, ok := args[0].(bool)
			if !ok {
				return oops.Newf("Failed to parse enqueueNext (expected boolean): %v", args[0])
			}

			return ResetFailedBlogsJob_Perform(ctx, pool, enqueueNext)
		},
	)
}

func ResetFailedBlogsJob_PerformNow(qu pgw.Queryable, enqueueNext bool) error {
	return performNow(qu, "ResetFailedBlogsJob", defaultQueue, boolToYaml(enqueueNext))
}

func ResetFailedBlogsJob_PerformAt(qu pgw.Queryable, runAt schedule.Time, enqueueNext bool) error {
	return performAt(qu, runAt, "ResetFailedBlogsJob", defaultQueue, boolToYaml(enqueueNext))
}

func ResetFailedBlogsJob_Perform(ctx context.Context, pool *pgw.Pool, enqueueNext bool) error {
	logger := pool.Logger()
	utcNow := schedule.UTCNow()
	cutoffTime := utcNow.Add(-30 * 24 * time.Hour)

	var sb strings.Builder
	for status := range models.BlogFailedAutoStatuses {
		if sb.Len() > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString("'")
		sb.WriteString(string(status))
		sb.WriteString("'")
	}
	rows, err := pool.Query(`
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
		newVersion, err := models.Blog_Downgrade(pool, blogId)
		if err != nil {
			return err
		}

		logger.Info().Msgf("Blog %d (%s) -> new version %d", blogId, feedUrls[i], newVersion)
	}

	if enqueueNext {
		tomorrow := utcNow.Add(24 * time.Hour)
		runAt := tomorrow.BeginningOfDayIn(time.UTC)
		err := ResetFailedBlogsJob_PerformAt(pool, runAt, true)
		if err != nil {
			return err
		}
	}

	return nil
}
