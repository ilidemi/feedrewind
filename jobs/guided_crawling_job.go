package jobs

import (
	"context"
	"errors"
	"feedrewind/crawler"
	"feedrewind/db"
	"feedrewind/db/pgw"
	"feedrewind/log"
	"feedrewind/models"
	"feedrewind/oops"
	"feedrewind/publish"
	"feedrewind/util"
	"fmt"
	"time"

	"github.com/goccy/go-json"
	"github.com/jackc/pgx/v5"
)

func init() {
	registerJobNameFunc(
		"GuidedCrawlingJob",
		true,
		func(ctx context.Context, id JobId, conn *pgw.Conn, args []any) error {
			if len(args) != 2 {
				return oops.Newf("Expected 2 args, got %d: %v", len(args), args)
			}

			blogIdInt64, ok := args[0].(int64)
			if !ok {
				blogIdInt, ok := args[0].(int)
				if !ok {
					return oops.Newf("Failed to parse blogId (expected int64 or int): %v", args[0])
				}
				blogIdInt64 = int64(blogIdInt)
			}
			blogId := models.BlogId(blogIdInt64)

			argsJson, ok := args[1].(string)
			if !ok {
				return oops.Newf("Failed to parse args (expected string): %v", args[1])
			}

			return GuidedCrawlingJob_Perform(ctx, id, conn, blogId, argsJson)
		},
	)

	publish.EmailPostsJob_PerformNowFunc = EmailPostsJob_PerformNow
}

type GuidedCrawlingJobArgs struct {
	StartFeedId models.StartFeedId `json:"start_feed_id"`
}

func GuidedCrawlingJob_PerformNow(
	tx pgw.Queryable, blogId models.BlogId, startFeedId models.StartFeedId,
) error {
	args := GuidedCrawlingJobArgs{
		StartFeedId: startFeedId,
	}
	argsJson, err := json.Marshal(&args)
	if err != nil {
		return oops.Wrap(err)
	}
	return performNow(
		tx, "GuidedCrawlingJob", defaultQueue, int64ToYaml(int64(blogId)), strToYaml(string(argsJson)),
	)
}

func GuidedCrawlingJob_Perform(
	ctx context.Context, id JobId, preCrawlConn *pgw.Conn, blogId models.BlogId, argsJson string,
) error {
	startTime := time.Now().UTC()
	logger := preCrawlConn.Logger()
	var args GuidedCrawlingJobArgs
	err := json.Unmarshal([]byte(argsJson), &args)
	if err != nil {
		return oops.Wrap(err)
	}

	row := preCrawlConn.QueryRow(`
		select title, url, final_url, content, start_page_id
		from start_feeds
		where id = $1
	`, args.StartFeedId)
	var startFeed crawler.Feed
	var content []byte
	var maybeStartPageId *models.StartPageId
	err = row.Scan(
		&startFeed.Title, &startFeed.Url, &startFeed.FinalUrl, &content, &maybeStartPageId,
	)
	if err != nil {
		return err
	}
	startFeed.Content = string(content)

	var maybeStartPage *crawler.DiscoveredStartPage
	if maybeStartPageId != nil {
		row := preCrawlConn.QueryRow(`
			select url, final_url, content from start_pages where id = $1
		`, *maybeStartPageId)
		var startPage crawler.DiscoveredStartPage
		var content []byte
		err := row.Scan(&startPage.Url, &startPage.FinalUrl, &content)
		if err != nil {
			return err
		}
		startPage.Content = string(content)
		maybeStartPage = &startPage
	}

	row = preCrawlConn.QueryRow(`select feed_url, name from blogs where id = $1`, blogId)
	var blogFeedUrl string
	var blogName string
	err = row.Scan(&blogFeedUrl, &blogName)
	if errors.Is(err, pgx.ErrNoRows) {
		logger.Info().Msg("Blog not found")
		return nil
	} else if err != nil {
		return err
	}

	row = preCrawlConn.QueryRow(`
		select status from blogs where feed_url = $1 and version != $2 order by version desc limit 1
	`, blogFeedUrl, models.BlogLatestVersion)
	hasPreviouslyFailed := false
	var lastBlogStatus models.BlogStatus
	err = row.Scan(&lastBlogStatus)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	} else if err == nil {
		hasPreviouslyFailed = models.BlogFailedStatuses[lastBlogStatus]
	}
	preCrawlConn.Release()

	checkCancellationFunc := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		conn, err := db.Pool.AcquireBackgroundWithLogger(logger)
		if err != nil {
			logger.Warn().Err(err).Msg("Couldn't acquire a db connection to check if the job was deleted")
			return nil
		}
		defer conn.Release()
		row := conn.QueryRow(`select 1 from delayed_jobs where id = $1`, id)
		var one int
		err = row.Scan(&one)
		if errors.Is(err, pgx.ErrNoRows) {
			return err
		} else if err != nil {
			logger.Warn().Err(err).Msg("Couldn't check if the job was deleted")
			return nil
		}
		return nil
	}
	httpClient := crawler.NewHttpClientImplFunc(checkCancellationFunc, true)
	puppeteerClient := crawler.NewPuppeteerClientImpl()
	progressSaver := NewProgressSaver(blogId, blogFeedUrl, logger)
	progressLogger := crawler.NewProgressLogger(progressSaver)
	crawlCtx := crawler.NewCrawlContext(httpClient, puppeteerClient, &progressLogger)
	zLogger := crawler.ZeroLogger{
		Logger: logger,
	}

	guidedCrawlResult, err := crawler.GuidedCrawl(maybeStartPage, startFeed, &crawlCtx, &zLogger)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if err != nil {
		logger.Info().Err(err).Msg("Guided crawl failed")
		guidedCrawlResult = nil
	} else {
		if guidedCrawlResult.HardcodedError != nil {
			logger.Warn().Err(guidedCrawlResult.HardcodedError).Msg("Guided crawl couldn't do hardcoded")
			// Continue in degraded fashion
		}
		if guidedCrawlResult.HistoricalError != nil {
			logger.Info().Err(guidedCrawlResult.HistoricalError).Msg("Guided crawl failed (historical)")
			guidedCrawlResult = nil
		}
	}

	postCrawlConn, err := db.Pool.Acquire(ctx, logger)
	if err != nil {
		return err
	}
	defer postCrawlConn.Release()
	return util.Tx(postCrawlConn, func(tx *pgw.Tx, postCrawlConn util.Clobber) error {
		var preCrawlConn util.Clobber
		_ = preCrawlConn
		var maybeBlogUrl *string
		var crawlSucceeded bool
		if guidedCrawlResult != nil && guidedCrawlResult.HistoricalResult != nil {
			logger.Info().Msg("Guided crawling succeeded, saving blog")
			historicalResult := guidedCrawlResult.HistoricalResult
			maybeBlogUrl = &historicalResult.BlogLink.Url
			categoriesListByLink := crawler.NewCanonicalUriMap[*[]string](guidedCrawlResult.CuriEqCfg)
			var categories []models.NewBlogPostCategory
			postCuris := crawler.ToCanonicalUris(historicalResult.Links)
			postCurisSet := crawler.NewCanonicalUriSet(postCuris, guidedCrawlResult.CuriEqCfg)
			if len(historicalResult.PostCategories) > 0 {
				for _, category := range historicalResult.PostCategories {
					categoryFilteredLinks := make([]crawler.Link, 0, len(category.PostLinks))
					for _, link := range category.PostLinks {
						if !postCurisSet.Contains(link.Curi) {
							logger.Warn().Msgf(
								"Post from category is not present in the list, skipping: %s", link.Url,
							)
							continue
						}
						categoryFilteredLinks = append(categoryFilteredLinks, link)
					}

					if len(categoryFilteredLinks) > 0 &&
						!(category.IsTop && len(categoryFilteredLinks) == len(historicalResult.Links)) {

						for _, link := range categoryFilteredLinks {
							if linkCategories, ok := categoriesListByLink.Get(link.Curi); ok {
								*linkCategories = append(*linkCategories, category.Name)
							} else {
								categoriesListByLink.Add(link, &[]string{category.Name})
							}
						}

						topStatus := models.BlogPostCategoryCustomOnly
						if category.IsTop {
							topStatus = models.BlogPostCategoryTopOnly
						}
						categories = append(categories, models.NewBlogPostCategory{
							Name:      category.Name,
							Index:     int32(len(categories)),
							TopStatus: topStatus,
						})
					}
				}
			}
			categories = append(categories, models.NewBlogPostCategory{
				Name:      "Everything",
				Index:     int32(len(categories)),
				TopStatus: models.BlogPostCategoryTopOnly,
			})
			crawledBlogPosts := make([]models.CrawledBlogPost, len(historicalResult.Links))
			for i, link := range historicalResult.Links {
				var fullLinkCategories []string
				if linkCategories, ok := categoriesListByLink.Get(link.Curi); ok {
					fullLinkCategories = append(fullLinkCategories, *linkCategories...)
				}
				fullLinkCategories = append(fullLinkCategories, "Everything")
				crawledBlogPosts[i] = models.CrawledBlogPost{
					Url:        link.Url,
					Title:      link.Title.Value,
					Categories: fullLinkCategories,
				}
			}

			blogUpdatedAt, err := models.Blog_InitCrawled(
				tx, blogId, *maybeBlogUrl, crawledBlogPosts, categories,
				historicalResult.DiscardedFeedEntryUrls, guidedCrawlResult.CuriEqCfg,
			)
			if err != nil {
				return err
			}
			err = logCrawlFinished(tx, blogId, blogUpdatedAt, "crawl succeeded")
			if err != nil {
				return err
			}
			crawlSucceeded = true
		} else {
			logger.Info().Msg("Historical links not found")
			maybeBlogUrl = nil
			row := tx.QueryRow(`
				update blogs set status = $1 where id = $2 returning updated_at
			`, models.BlogStatusCrawlFailed, blogId)
			var blogUpdatedAt time.Time
			err := row.Scan(&blogUpdatedAt)
			if err != nil {
				return err
			}
			err = logCrawlFinished(tx, blogId, blogUpdatedAt, "crawl failed")
			if err != nil {
				return err
			}
			crawlSucceeded = false
		}

		elapsedSeconds := time.Since(startTime).Seconds()
		telemetryKey := "guided_crawling_job_success"
		if !crawlSucceeded {
			telemetryKey = "guided_crawling_job_failure"
		}
		err = models.AdminTelemetry_Create(tx, telemetryKey, elapsedSeconds, map[string]any{
			"feed_url": startFeed.Url,
		})
		if err != nil {
			return err
		}

		var slackBlogUrl string
		switch {
		case maybeBlogUrl != nil:
			slackBlogUrl = NotifySlackJob_Escape(*maybeBlogUrl)
		case maybeStartPage != nil:
			slackBlogUrl = NotifySlackJob_Escape(maybeStartPage.Url)
		default:
			slackBlogUrl = NotifySlackJob_Escape(startFeed.Url)
		}
		slackBlogName := NotifySlackJob_Escape(blogName)
		slackVerb := "succeeded"
		if !crawlSucceeded {
			slackVerb = "failed"
		}
		slackText := fmt.Sprintf(
			"Crawling *<%s|%s>* %s in %.1f seconds", slackBlogUrl, slackBlogName, slackVerb, elapsedSeconds,
		)
		err = NotifySlackJob_PerformNow(tx, slackText)
		if err != nil {
			return err
		}

		if crawlCtx.TitleFetchDuration > 0 {
			err := models.AdminTelemetry_Create(
				tx, "crawling_title_fetch_duration", crawlCtx.TitleFetchDuration, map[string]any{
					"feed_url":      startFeed.Url,
					"requests_made": crawlCtx.TitleRequestsMade,
				},
			)
			if err != nil {
				return err
			}
		}
		if crawlCtx.DuplicateFetches > 0 {
			err := models.AdminTelemetry_Create(
				tx, "crawling_duplicate_requests", float64(crawlCtx.DuplicateFetches), map[string]any{
					"feed_url": startFeed.Url,
				},
			)
			if err != nil {
				return err
			}
		}
		if hasPreviouslyFailed {
			value := 1.0
			if !crawlSucceeded {
				value = 0.0
			}
			err := models.AdminTelemetry_Create(tx, "recrawl_status", value, map[string]any{
				"feed_url": startFeed.Url,
			})
			if err != nil {
				return err
			}
		}

		payload := map[string]any{
			"blog_id": fmt.Sprint(blogId),
			"done":    true,
		}
		payloadBytes, err := json.Marshal(&payload)
		if err != nil {
			return oops.Wrap(err)
		}
		_, err = tx.Exec(`select pg_notify($1, $2)`, CrawlProgressChannelName, string(payloadBytes))
		if err != nil {
			return err
		}
		logger.Info().Msgf("%s %d done:true", CrawlProgressChannelName, blogId)
		return nil
	})
}

func logCrawlFinished(tx *pgw.Tx, blogId models.BlogId, blogUpdatedAt time.Time, eventType string) error {
	rows, err := tx.Query(`
		select id, created_at, anon_product_user_id, (
			select product_user_id from users_with_discarded
			where users_with_discarded.id = subscriptions_with_discarded.user_id
		), (
			select coalesce(url, feed_url) from blogs where blogs.id = subscriptions_with_discarded.blog_id
		)
		from subscriptions_with_discarded
		where blog_id = $1
	`, blogId)
	if err != nil {
		return err
	}
	type CrawledSubscription struct {
		Id                     models.SubscriptionId
		CreatedAt              time.Time
		MaybeAnonProductUserId *models.ProductUserId
		MaybeProductUserId     *models.ProductUserId
		BlogUrl                string
	}
	var crawledSubscriptions []CrawledSubscription
	for rows.Next() {
		var s CrawledSubscription
		err := rows.Scan(
			&s.Id, &s.CreatedAt, &s.MaybeAnonProductUserId, &s.MaybeProductUserId, &s.BlogUrl,
		)
		if err != nil {
			return err
		}
		crawledSubscriptions = append(crawledSubscriptions, s)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	batch := tx.NewBatch()
	for _, sub := range crawledSubscriptions {
		var productUserId models.ProductUserId
		switch {
		case sub.MaybeProductUserId != nil:
			productUserId = *sub.MaybeProductUserId
		case sub.MaybeAnonProductUserId != nil:
			productUserId = *sub.MaybeAnonProductUserId
		default:
			return oops.New("both product_user_id and anon_product_user_id are null")
		}
		models.ProductEvent_EmitToBatch(batch, productUserId, eventType, map[string]any{
			"subscription_id": sub.Id,
			"blog_url":        sub.BlogUrl,
			"wait_duration":   blogUpdatedAt.Sub(sub.CreatedAt).Seconds(),
		}, nil)
	}
	err = tx.SendBatch(batch).Close()
	if err != nil {
		return err
	}

	return nil
}

const CrawlProgressChannelName = "crawl_progress"

type ProgressSaver struct {
	BlogId             models.BlogId
	FeedUrl            string
	Logger             log.Logger
	LastEpochTimestamp time.Time
}

func NewProgressSaver(blogId models.BlogId, feedUrl string, logger log.Logger) *ProgressSaver {
	return &ProgressSaver{
		BlogId:             blogId,
		FeedUrl:            feedUrl,
		Logger:             logger,
		LastEpochTimestamp: time.Now().UTC(),
	}
}

func (s *ProgressSaver) SaveStatusAndCount(status string, maybeCount *int) {
	conn, err := db.Pool.AcquireBackgroundWithLogger(s.Logger)
	if err != nil {
		s.Logger.Warn().Err(err).Msg("Couldn't acquire a db connection to save status and count")
		return
	}
	defer conn.Release()
	err = util.Tx(conn, func(tx *pgw.Tx, conn util.Clobber) error {
		row := tx.QueryRow(`
			update blog_crawl_progresses
			set progress = $1, count = $2, epoch = epoch + 1
			where blog_id = $3
			returning epoch, epoch_times
		`, status, maybeCount, s.BlogId)
		var newEpoch int32
		var maybeEpochTimes *string
		err := row.Scan(&newEpoch, &maybeEpochTimes)
		if err != nil {
			return err
		}

		err = s.updateEpochTimes(tx, maybeEpochTimes)
		if err != nil {
			return err
		}

		payload := map[string]any{
			"blog_id": fmt.Sprint(s.BlogId),
			"epoch":   newEpoch,
			"status":  status,
			"count":   maybeCount,
		}
		payloadBytes, err := json.Marshal(&payload)
		if err != nil {
			return oops.Wrap(err)
		}

		_, err = tx.Exec(`select pg_notify($1, $2)`, CrawlProgressChannelName, string(payloadBytes))
		if err != nil {
			return err
		}

		tx.Logger().Info().Msgf(
			"%s %d epoch: %d status: %s count: %s",
			CrawlProgressChannelName, s.BlogId, newEpoch, status, crawler.SprintIntPtr(maybeCount),
		)
		return nil
	})
	if err != nil {
		s.Logger.Warn().Err(err).Msg("Couldn't save status and count")
	}
}

func (s *ProgressSaver) SaveStatus(status string) {
	conn, err := db.Pool.AcquireBackgroundWithLogger(s.Logger)
	if err != nil {
		s.Logger.Warn().Err(err).Msg("Couldn't acquire a db connection to save status")
		return
	}
	defer conn.Release()
	err = util.Tx(conn, func(tx *pgw.Tx, conn util.Clobber) error {
		row := tx.QueryRow(`
			update blog_crawl_progresses
			set progress = $1, epoch = epoch + 1
			where blog_id = $2
			returning epoch, epoch_times
		`, status, s.BlogId)
		var newEpoch int32
		var maybeEpochTimes *string
		err := row.Scan(&newEpoch, &maybeEpochTimes)
		if err != nil {
			return err
		}

		err = s.updateEpochTimes(tx, maybeEpochTimes)
		if err != nil {
			return err
		}

		payload := map[string]any{
			"blog_id": fmt.Sprint(s.BlogId),
			"epoch":   newEpoch,
			"status":  status,
		}
		payloadBytes, err := json.Marshal(&payload)
		if err != nil {
			return oops.Wrap(err)
		}

		_, err = tx.Exec(`select pg_notify($1, $2)`, CrawlProgressChannelName, string(payloadBytes))
		if err != nil {
			return err
		}

		tx.Logger().Info().Msgf(
			"%s %d epoch: %d status: %s", CrawlProgressChannelName, s.BlogId, newEpoch, status,
		)
		return nil
	})
	if err != nil {
		s.Logger.Warn().Err(err).Msg("Couldn't save status")
	}
}

func (s *ProgressSaver) SaveCount(maybeCount *int) {
	conn, err := db.Pool.AcquireBackgroundWithLogger(s.Logger)
	if err != nil {
		s.Logger.Warn().Err(err).Msg("Couldn't acquire a db connection to save count")
		return
	}
	defer conn.Release()
	err = util.Tx(conn, func(tx *pgw.Tx, conn util.Clobber) error {
		row := tx.QueryRow(`
			update blog_crawl_progresses
			set count = $1, epoch = epoch + 1
			where blog_id = $2
			returning epoch, epoch_times
		`, maybeCount, s.BlogId)
		var newEpoch int32
		var maybeEpochTimes *string
		err := row.Scan(&newEpoch, &maybeEpochTimes)
		if err != nil {
			return err
		}

		err = s.updateEpochTimes(tx, maybeEpochTimes)
		if err != nil {
			return err
		}

		payload := map[string]any{
			"blog_id": fmt.Sprint(s.BlogId),
			"epoch":   newEpoch,
			"count":   maybeCount,
		}
		payloadBytes, err := json.Marshal(&payload)
		if err != nil {
			return oops.Wrap(err)
		}

		_, err = tx.Exec(`select pg_notify($1, $2)`, CrawlProgressChannelName, string(payloadBytes))
		if err != nil {
			return err
		}

		tx.Logger().Info().Msgf(
			"%s %d epoch: %d count: %s",
			CrawlProgressChannelName, s.BlogId, newEpoch, crawler.SprintIntPtr(maybeCount),
		)
		return nil
	})
	if err != nil {
		s.Logger.Warn().Err(err).Msg("Couldn't save count")
	}
}

func (s *ProgressSaver) EmitTelemetry(regressions string, extra map[string]any) {
	fullExtra := map[string]any{
		"blog_id":     fmt.Sprint(s.BlogId),
		"feed_url":    s.FeedUrl,
		"regressions": regressions,
	}
	for key, value := range extra {
		fullExtra[key] = value
	}
	conn, err := db.Pool.AcquireBackgroundWithLogger(s.Logger)
	if err != nil {
		s.Logger.Warn().Err(err).Msg("Couldn't acquire a db connection to emit telemetry")
		return
	}
	defer conn.Release()
	err = models.AdminTelemetry_Create(conn, "progress_regression", 1, fullExtra)
	if err != nil {
		s.Logger.Warn().Err(err).Msg("Couldn't emit telemetry")
	}
}

func (s *ProgressSaver) updateEpochTimes(tx *pgw.Tx, maybeEpochTimes *string) error {
	newEpochTimestamp := time.Now().UTC()
	newEpochTime := newEpochTimestamp.Sub(s.LastEpochTimestamp)
	newEpochTimeStr := fmt.Sprintf("%.3f", newEpochTime.Seconds())
	var newEpochTimes string
	if maybeEpochTimes != nil {
		newEpochTimes = fmt.Sprintf("%s;%s", *maybeEpochTimes, newEpochTimeStr)
	} else {
		newEpochTimes = newEpochTimeStr
	}
	_, err := tx.Exec(`
		update blog_crawl_progresses set epoch_times = $1 where blog_id = $2
	`, newEpochTimes, s.BlogId)
	if err != nil {
		return err
	}
	s.LastEpochTimestamp = newEpochTimestamp
	return nil
}
