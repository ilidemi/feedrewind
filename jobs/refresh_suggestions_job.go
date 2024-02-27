package jobs

import (
	"context"
	"errors"
	"feedrewind/crawler"
	"feedrewind/db/migrations"
	"feedrewind/db/pgw"
	"feedrewind/models"
	"feedrewind/oops"
	"feedrewind/util"
	"feedrewind/util/schedule"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
)

func init() {
	registerJobNameFunc("RefreshSuggestionsJob",
		func(ctx context.Context, conn *pgw.Conn, args []any) error {
			if len(args) != 0 {
				return oops.Newf("Expected 0 args, got %d: %v", len(args), args)
			}

			return RefreshSuggestionsJob_Perform(ctx, conn)
		},
	)

	migrations.RefreshSuggestionsJob_PerformAtFunc = RefreshSuggestionsJob_PerformAt
}

func RefreshSuggestionsJob_PerformAt(tx pgw.Queryable, runAt schedule.Time) error {
	return performAt(tx, runAt, "RefreshSuggestionsJob", defaultQueue)
}

func RefreshSuggestionsJob_Perform(ctx context.Context, conn *pgw.Conn) error {
	logger := conn.Logger()

	feedUrlsSet := map[string]bool{}
	for _, screenshotLink := range util.ScreenshotLinks {
		feedUrlsSet[screenshotLink.FeedUrl] = true
	}
	for _, category := range util.SuggestedCategories {
		for _, blog := range category.Blogs {
			feedUrlsSet[blog.FeedUrl] = true
		}
	}
	for _, miscBlog := range util.MiscellaneousBlogs {
		feedUrlsSet[miscBlog.FeedUrl] = true
	}
	var feedUrls []string
	for feedUrl := range feedUrlsSet {
		if feedUrl == crawler.HardcodedOurMachinery {
			continue
		}
		feedUrls = append(feedUrls, feedUrl)
	}
	sort.Slice(feedUrls, func(i, j int) bool {
		return feedUrls[i] < feedUrls[j]
	})

	httpClient := crawler.NewHttpClientImpl(ctx, false)
	dlogger := crawler.DummyLogger{}
	progressLogger := crawler.NewMockProgressLogger(&dlogger)
	crawlCtx := crawler.NewCrawlContext(httpClient, nil, &progressLogger)
	for _, feedUrl := range feedUrls {
		if err := ctx.Err(); err != nil {
			return err
		}

		discoverFeedsResult := crawler.DiscoverFeedsAtUrl(feedUrl, true, &crawlCtx, &dlogger)
		discoveredSingleFeed, ok := discoverFeedsResult.(*crawler.DiscoveredSingleFeed)
		if !ok {
			logger.Error().Msgf("Expected DiscoveredSingleFeed, got: %#v", discoveredSingleFeed)
			continue
		}

		row := conn.QueryRow(`
			select start_feed_id from blogs where feed_url = $1 and version = $2
		`, feedUrl, models.BlogLatestVersion)
		var maybeStartFeedId *models.StartFeedId
		err := row.Scan(&maybeStartFeedId)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			logger.Error().Err(err).Msgf("Error when checking if the feed already exists: %s", feedUrl)
			continue
		}

		if maybeStartFeedId != nil {
			row = conn.QueryRow(`select content from start_feeds where id = $1`, *maybeStartFeedId)
			var content []byte
			err := row.Scan(&content)
			if err != nil {
				logger.Error().Err(err).Msgf("Error when checking feed contents: %s", feedUrl)
				continue
			}
			if discoveredSingleFeed.Feed.Content == string(content) {
				// Exact match, definitely no need to crawl
				continue
			}
		}

		startFeed, err := models.StartFeed_CreateFetched(conn, nil, discoveredSingleFeed.Feed)
		if err != nil {
			return err
		}
		updatedBlog, err := models.Blog_CreateOrUpdate(conn, startFeed, GuidedCrawlingJob_PerformNow)
		if err != nil {
			return err
		}
		if updatedBlog.MaybeStartFeedId == nil {
			logger.Info().Msgf("Registering start feed %d for blog %d", startFeed.Id, updatedBlog.Id)
			_, err := conn.Exec(
				`update blogs set start_feed_id = $1 where id = $2
			`, startFeed.Id, updatedBlog.Id)
			if err != nil {
				return err
			}
		}
		logger.Info().Msgf("Created or updated blog %d: %s", updatedBlog.Id, updatedBlog.Status)
	}

	runAt := schedule.UTCNow().Add(time.Hour).BeginningOfHour().Add(30 * time.Minute)
	err := RefreshSuggestionsJob_PerformAt(conn, runAt)
	if err != nil {
		return err
	}

	return nil
}
