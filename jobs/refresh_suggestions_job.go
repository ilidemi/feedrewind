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
	neturl "net/url"
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
		if feedUrl == crawler.HardcodedOurMachinery || feedUrl == crawler.HardcodedSequences {
			continue
		}
		feedUrls = append(feedUrls, feedUrl)
	}
	sort.Slice(feedUrls, func(i, j int) bool {
		return feedUrls[i] < feedUrls[j]
	})

	httpClient := crawler.NewHttpClientImpl(ctx, false)
	for _, feedUrl := range feedUrls {
		if err := ctx.Err(); err != nil {
			return err
		}

		discoverLogger := crawler.NewDummyLogger()
		progressLogger := crawler.NewMockProgressLogger(discoverLogger)
		crawlCtx := crawler.NewCrawlContext(httpClient, nil, &progressLogger)
		discoverFeedsResult := crawler.DiscoverFeedsAtUrl(feedUrl, true, &crawlCtx, discoverLogger)
		discoveredSingleFeed, ok := discoverFeedsResult.(*crawler.DiscoveredSingleFeed)
		if !ok {
			logger.Error().Msgf("Expected DiscoveredSingleFeed, got: %#v (%s)", discoveredSingleFeed, feedUrl)
			discoverLogger.Replay(logger)
			continue
		}

		row := conn.QueryRow(`
			select id, status, start_feed_id from blogs where feed_url = $1 and version = $2
		`, feedUrl, models.BlogLatestVersion)
		var maybeBlogId *models.BlogId
		var maybeBlogStatus *models.BlogStatus
		var maybeStartFeedId *models.StartFeedId
		err := row.Scan(&maybeBlogId, &maybeBlogStatus, &maybeStartFeedId)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			logger.Error().Err(err).Msgf("Error when checking if the feed already exists: %s", feedUrl)
			continue
		}

		if maybeBlogId != nil && maybeBlogStatus != nil && models.BlogFailedStatuses[*maybeBlogStatus] {
			logger.Info().Msgf("Downgrading blog %d (%s)", *maybeBlogId, feedUrl)
			_, err := models.Blog_Downgrade(conn, *maybeBlogId)
			if err != nil {
				logger.Error().Err(err).Msgf("Error when downgrading blog: %s", feedUrl)
				continue
			}
		} else if maybeStartFeedId != nil {
			row = conn.QueryRow(`select content, url from start_feeds where id = $1`, *maybeStartFeedId)
			var content []byte
			var url string
			err := row.Scan(&content, &url)
			if err != nil {
				logger.Error().Err(err).Msgf("Error when checking feed contents: %s", feedUrl)
				continue
			}

			fetchUri, err := neturl.Parse(url)
			if err != nil {
				logger.Error().Err(err).Msgf("Error when parsing feed url: %s", feedUrl)
				continue
			}

			parseLogger := crawler.NewDummyLogger()
			parsedFeed, err := crawler.ParseFeed(string(content), fetchUri, parseLogger)
			if err != nil {
				logger.Error().Err(err).Msgf("Error when parsing feed: %s", feedUrl)
				parseLogger.Replay(logger)
				continue
			}

			existingLinks := parsedFeed.EntryLinks.ToSlice()
			newLinks := discoveredSingleFeed.Feed.ParsedFeed.EntryLinks.ToSlice()
			if len(newLinks) == len(existingLinks) {
				exactMatch := true
				curiEqCfg, err := models.BlogCanonicalEqualityConfig_Get(conn, *maybeBlogId)
				if errors.Is(err, pgx.ErrNoRows) {
					logger.Warn().Msgf("CuriEqCfg not found, using an empty one: %s", feedUrl)
					curiEqCfgVal := crawler.NewCanonicalEqualityConfig()
					curiEqCfg = &curiEqCfgVal
				} else if err != nil {
					logger.Error().Err(err).Msgf("Error when getting curiEqCfg: %s", feedUrl)
					continue
				}
				for i, existingLink := range existingLinks {
					if !crawler.CanonicalUriEqual(newLinks[i].Curi, existingLink.Curi, curiEqCfg) {
						exactMatch = false
						break
					}
				}
				if exactMatch {
					// No need to crawl
					continue
				}
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
