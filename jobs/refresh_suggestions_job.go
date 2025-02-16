package jobs

import (
	"context"
	"errors"
	"math"
	neturl "net/url"
	"slices"
	"time"

	"feedrewind.com/config"
	"feedrewind.com/crawler"
	"feedrewind.com/db/pgw"
	"feedrewind.com/models"
	"feedrewind.com/oops"
	"feedrewind.com/util"
	"feedrewind.com/util/schedule"

	"github.com/jackc/pgx/v5"
)

func init() {
	registerJobNameFunc(
		"RefreshSuggestionsJob",
		func(ctx context.Context, id JobId, pool *pgw.Pool, args []any) error {
			if len(args) != 0 {
				return oops.Newf("Expected 0 args, got %d: %v", len(args), args)
			}

			return RefreshSuggestionsJob_Perform(ctx, pool)
		},
	)
}

func RefreshSuggestionsJob_PerformAt(qu pgw.Queryable, runAt schedule.Time) error {
	return performAt(qu, runAt, "RefreshSuggestionsJob", defaultQueue)
}

func RefreshSuggestionsJob_Perform(ctx context.Context, pool *pgw.Pool) error {
	logger := pool.Logger()

	feedUrlsSet := map[string]bool{}
	for _, screenshotLink := range util.ScreenshotLinks {
		feedUrlsSet[screenshotLink.FeedUrl] = true
	}
	for _, category := range util.SuggestedCategories {
		for _, blog := range category.Blogs {
			feedUrlsSet[blog.FeedUrl] = true
		}
	}
	for _, blog := range util.MiscellaneousBlogs {
		feedUrlsSet[blog.FeedUrl] = true
	}
	for _, category := range util.HmnCategories {
		for _, blog := range category.Blogs {
			feedUrlsSet[blog.FeedUrl] = true
		}
	}

	rows, err := pool.Query(`
		delete from ignored_suggestion_feeds where created_at < (utc_now() - interval '23:30')
		returning feed_url
	`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var feedUrl string
		err := rows.Scan(&feedUrl)
		if err != nil {
			return err
		}
		logger.Warn().Msgf("Expired previously ignored feed: %s", feedUrl)
	}

	rows, err = pool.Query(`select feed_url from ignored_suggestion_feeds`)
	if err != nil {
		return err
	}
	ignoredFeedUrls := map[string]bool{}
	for rows.Next() {
		var ignoredFeedUrl string
		err := rows.Scan(&ignoredFeedUrl)
		if err != nil {
			return err
		}
		ignoredFeedUrls[ignoredFeedUrl] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}

	var feedUrls []string
	for feedUrl := range feedUrlsSet {
		if feedUrl == crawler.HardcodedOurMachinery || feedUrl == crawler.HardcodedSequences {
			continue
		}
		feedUrls = append(feedUrls, feedUrl)
	}
	slices.Sort(feedUrls)

	httpClient := crawler.NewHttpClientImpl(ctx, nil, false)
feeds:
	for _, feedUrl := range feedUrls {
		if err := ctx.Err(); err != nil {
			return err
		}

		discoverLogger := crawler.NewDummyLogger()
		progressLogger := crawler.NewMockProgressLogger(discoverLogger)
		crawlCtx := crawler.NewCrawlContext(httpClient, nil, progressLogger)
		var discoverFeedsResult crawler.DiscoverFeedsResult
		for attempt := 1; ; attempt++ {
			discoverFeedsResult = crawler.DiscoverFeedsAtUrl(feedUrl, true, &crawlCtx, discoverLogger)
			if _, ok := discoverFeedsResult.(*crawler.DiscoverFeedsErrorCouldNotReach); !ok {
				break
			}
			maxAttempts := 6
			if config.Cfg.Env.IsDevOrTest() {
				maxAttempts = 2
			}
			if attempt >= maxAttempts {
				if ignoredFeedUrls[feedUrl] {
					logger.Info().Msgf(
						"Couldn't reach feed: %s, attempts exhausted (failure already reported).", feedUrl,
					)
				} else {
					logger.Error().Msgf(
						"Couldn't reach feed: %s, attempts exhausted. Silencing further failures", feedUrl,
					)
					_, err := pool.Exec(`
						insert into ignored_suggestion_feeds (feed_url) values ($1)
					`, feedUrl)
					if err != nil {
						return err
					}
				}
				continue feeds
			} else {
				delay := time.Duration(math.Pow(5, float64(attempt-1))) * time.Second
				logger.Info().Msgf("Couldn't reach feed: %s, sleeping for %v", feedUrl, delay)
				err := util.Sleep(ctx, delay)
				if err != nil {
					return err
				}
			}
		}
		discoveredSingleFeed, ok := discoverFeedsResult.(*crawler.DiscoveredSingleFeed)
		if !ok {
			logger.Error().Msgf("Expected DiscoveredSingleFeed, got: %#v (%s)", discoverFeedsResult, feedUrl)
			discoverLogger.Replay(logger)
			continue
		}
		if ignoredFeedUrls[feedUrl] {
			_, err := pool.Exec(`delete from ignored_suggestion_feeds where feed_url = $1`, feedUrl)
			if err != nil {
				return err
			}
			logger.Info().Msgf("Expired previously ignored feed: %s", feedUrl)
		}

		row := pool.QueryRow(`
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
			if *maybeBlogStatus == models.BlogStatusUpdateFromFeedFailed {
				logger.Warn().Msgf("Blog previously failed update from feed, skipping: %s", feedUrl)
				continue
			}

			logger.Info().Msgf("Downgrading blog %d (%s)", *maybeBlogId, feedUrl)
			_, err := models.Blog_Downgrade(pool, *maybeBlogId)
			if err != nil {
				logger.Error().Err(err).Msgf("Error when downgrading blog: %s", feedUrl)
				continue
			}
		} else if maybeStartFeedId != nil {
			row = pool.QueryRow(`select content, url from start_feeds where id = $1`, *maybeStartFeedId)
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
				curiEqCfg, err := models.BlogCanonicalEqualityConfig_Get(pool, *maybeBlogId)
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

		startFeed, err := models.StartFeed_CreateFetched(pool, nil, discoveredSingleFeed.Feed)
		if err != nil {
			return err
		}
		updatedBlog, err := models.Blog_CreateOrUpdate(pool, startFeed, GuidedCrawlingJob_PerformNow)
		if err != nil {
			return err
		}
		if updatedBlog.MaybeStartFeedId == nil {
			logger.Info().Msgf("Registering start feed %d for blog %d", startFeed.Id, updatedBlog.Id)
			_, err := pool.Exec(
				`update blogs set start_feed_id = $1 where id = $2
			`, startFeed.Id, updatedBlog.Id)
			if err != nil {
				return err
			}
		}
		if models.BlogFailedStatuses[updatedBlog.Status] {
			logger.Warn().Msgf(
				"Creating or updating failed (%s) for blog %d (%s)",
				updatedBlog.Status, updatedBlog.Id, feedUrl,
			)
		} else {
			logger.Info().Msgf(
				"Created or updated blog %d (%s): %s",
				updatedBlog.Id, updatedBlog.BestUrl, updatedBlog.Status,
			)
		}
	}

	if config.Cfg.Env.IsDevOrTest() {
		attempts := 0
		for schedule.UTCNow().Before(schedule.NewTime(2024, time.January, 1, 0, 0, 0, 0, time.UTC)) {
			attempts++
			if attempts > 24 {
				return oops.Newf(
					"Time traveling tests have been running for a while, not rescheduling the expensive job",
				)
			}
			logger.Info().Msg("Time travel is active, waiting")
			err := util.Sleep(ctx, 5*time.Minute)
			if err != nil {
				return err
			}
		}
	}

	runAt := schedule.UTCNow().Add(time.Hour).BeginningOfHour().Add(30 * time.Minute)
	err = RefreshSuggestionsJob_PerformAt(pool, runAt)
	if err != nil {
		return err
	}

	return nil
}
