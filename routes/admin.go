package routes

import (
	"cmp"
	"feedrewind/crawler"
	"feedrewind/db/pgw"
	"feedrewind/jobs"
	"feedrewind/models"
	"feedrewind/models/mutil"
	"feedrewind/oops"
	"feedrewind/routes/rutil"
	"feedrewind/templates"
	"feedrewind/util"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/goccy/go-json"
	"github.com/jackc/pgx/v5"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

func Admin_AddBlog(w http.ResponseWriter, r *http.Request) {
	type Result struct {
		Title   string
		Session *util.Session
	}
	templates.MustWrite(w, "admin/add_blog", Result{
		Title:   "Add blog",
		Session: rutil.Session(r),
	})
}

func Admin_PostBlog(w http.ResponseWriter, r *http.Request) {
	logger := rutil.Logger(r)
	type BlogWithCounts struct {
		Name            string
		FeedUrl         string
		PostsCount      int
		CategoriesCount int
	}
	postBlogImpl := func() (blog *BlogWithCounts, err error) {
		name := util.EnsureParamStr(r, "name")

		feedUrl := util.EnsureParamStr(r, "feed_url")
		if !strings.HasPrefix(feedUrl, "http://") && !strings.HasPrefix(feedUrl, "https://") {
			return nil, oops.Newf("Feed url is supposed to be full: %s", feedUrl)
		}

		blogUrl := util.EnsureParamStr(r, "url")
		direction := util.EnsureParamStr(r, "direction")

		postUrlsTitles, err := admin_ParseUrlsLabels(util.EnsureParamStr(r, "posts"))
		if err != nil {
			return nil, err
		}
		if direction == "newest_first" {
			for i, j := 0, len(postUrlsTitles)-1; i < j; i, j = i+1, j-1 {
				postUrlsTitles[i], postUrlsTitles[j] = postUrlsTitles[j], postUrlsTitles[i]
			}
		}
		postUrlsSet := make(map[string]bool)
		for _, post := range postUrlsTitles {
			postUrlsSet[post.Url] = true
		}

		postUrlsCategories, err := admin_ParseUrlsLabels(util.EnsureParamStr(r, "post_categories"))
		if err != nil {
			return nil, err
		}
		for _, urlCategory := range postUrlsCategories {
			if !postUrlsSet[urlCategory.Url] {
				return nil, oops.Newf("Unknown categorized url: %s", urlCategory.Url)
			}
		}

		topCategoriesStr := util.EnsureParamStr(r, "top_categories")
		var topCategories []string
		if topCategoriesStr != "" {
			topCategories = strings.Split(topCategoriesStr, ";")
		}
		topAndCustomCategoriesStr := util.EnsureParamStr(r, "top_and_custom_categories")
		var topAndCustomCategories []string
		if topAndCustomCategoriesStr != "" {
			topAndCustomCategories = strings.Split(topAndCustomCategoriesStr, ";")
			topCategoriesSet := map[string]bool{}
			for _, categoryName := range topCategories {
				if topCategoriesSet[categoryName] {
					return nil, oops.Newf("Duplicate category name: %s", categoryName)
				}
				topCategoriesSet[categoryName] = true
			}
			topAndCustomCategoriesSet := map[string]bool{}
			for _, categoryName := range topAndCustomCategories {
				if topCategoriesSet[categoryName] || topAndCustomCategoriesSet[categoryName] {
					return nil, oops.Newf("Duplicate category name: %s", categoryName)
				}
				topAndCustomCategoriesSet[categoryName] = true
			}
		}
		postCategories := slices.Concat(topCategories, topAndCustomCategories)
		topStatusByCategoryName := map[string]models.BlogPostCategoryTopStatus{}
		postCategoriesSet := make(map[string]bool)
		for _, topCategory := range topCategories {
			topStatusByCategoryName[topCategory] = models.BlogPostCategoryTopOnly
			postCategoriesSet[topCategory] = true
		}
		for _, topAndCustomCategory := range topAndCustomCategories {
			topStatusByCategoryName[topAndCustomCategory] = models.BlogPostCategoryTopAndCustom
			postCategoriesSet[topAndCustomCategory] = true
		}
		for _, urlCategory := range postUrlsCategories {
			if postCategoriesSet[urlCategory.Label] {
				continue
			}
			topStatusByCategoryName[urlCategory.Label] = models.BlogPostCategoryCustomOnly
			postCategories = append(postCategories, urlCategory.Label)
			postCategoriesSet[urlCategory.Label] = true
		}

		topStatusByCategoryName["Everything"] = models.BlogPostCategoryTopOnly
		postCategories = append(postCategories, "Everything")
		postCategoriesSet["Everything"] = true
		for postUrl := range postUrlsSet {
			postUrlsCategories = append(postUrlsCategories, urlLabel{
				Url:   postUrl,
				Label: "Everything",
			})
		}

		sameHosts := strings.Split(util.EnsureParamStr(r, "same_hosts"), "\n")
		sameHostsSet := make(map[string]bool)
		for i := range sameHosts {
			sameHostsSet[strings.TrimSpace(sameHosts[i])] = true
		}
		expectTumblrPaths := util.EnsureParamStr(r, "expect_tumblr_paths") == "1"
		curiEqCfg := crawler.CanonicalEqualityConfig{
			SameHosts:         sameHostsSet,
			ExpectTumblrPaths: expectTumblrPaths,
		}

		updateAction := models.BlogUpdateAction(util.EnsureParamStr(r, "update_action"))

		var postLinks []*crawler.Link
		var postCuris []crawler.CanonicalUri
		zlogger := crawler.ZeroLogger{Logger: logger, MaybeLogBlob: nil}
		for _, urlTitle := range postUrlsTitles {
			postLink, ok := crawler.ToCanonicalLink(urlTitle.Url, &zlogger, nil)
			if !ok {
				return nil, oops.Newf("Bad post link: %s", urlTitle.Url)
			}
			postLinks = append(postLinks, postLink)
			postCuris = append(postCuris, postLink.Curi)
		}
		postCurisSet := crawler.NewCanonicalUriSet(postCuris, &curiEqCfg)

		var discardedFromFeedEntryUrls []string
		var missingFromFeedEntryUrls []string
		if util.EnsureParamStr(r, "skip_feed_validation") != "1" {
			httpClient := crawler.NewHttpClientImplCtx(r.Context(), false)
			progressLogger := crawler.NewMockProgressLogger(&zlogger)
			crawlCtx := crawler.NewCrawlContext(httpClient, nil, progressLogger)
			feedResult := crawler.FetchFeedAtUrl(feedUrl, false, &crawlCtx, &zlogger)
			feedPage, ok := feedResult.(*crawler.FetchedPage)
			if !ok {
				return nil, oops.Newf("Couldn't fetch feed: %s", feedUrl)
			}

			feedLink, ok := crawler.ToCanonicalLink(feedUrl, &zlogger, nil)
			if !ok {
				return nil, oops.Newf("Bad feed url: %s", feedUrl)
			}

			parsedFeed, err := crawler.ParseFeed(feedPage.Page.Content, feedLink.Uri, &zlogger)
			if err != nil {
				return nil, err
			}

			feedEntryLinks := parsedFeed.EntryLinks.ToSlice()
			for _, entryLink := range feedEntryLinks {
				if !postCurisSet.Contains(entryLink.Curi) {
					discardedFromFeedEntryUrls = append(discardedFromFeedEntryUrls, entryLink.Url)
				}
			}

			var feedCuris []crawler.CanonicalUri
			for _, entryLink := range feedEntryLinks {
				feedCuris = append(feedCuris, entryLink.Curi)
			}
			feedCurisSet := crawler.NewCanonicalUriSet(feedCuris, &curiEqCfg)

			oldestFeedPostIndex := 0
			oldestFeedEntryCuri := feedEntryLinks[len(feedEntryLinks)-1].Curi
			for i, postLink := range postLinks {
				if crawler.CanonicalUriEqual(postLink.Curi, oldestFeedEntryCuri, &curiEqCfg) {
					oldestFeedPostIndex = i
				}
			}
			for _, postLink := range postLinks[oldestFeedPostIndex:] {
				if !feedCurisSet.Contains(postLink.Curi) {
					missingFromFeedEntryUrls = append(missingFromFeedEntryUrls, postLink.Url)
				}
			}
		}

		pool := rutil.DBPool(r)
		tx, err := pool.Begin()
		if err != nil {
			return nil, err
		}
		defer util.CommitOrRollbackErr(tx, &err)

		oldBlog, err := models.Blog_GetLatestByFeedUrl(tx, feedUrl)
		if !errors.Is(err, models.ErrBlogNotFound) && err != nil {
			return nil, err
		} else if err == nil {
			_, err = models.Blog_Downgrade(tx, oldBlog.Id)
			if err != nil {
				return nil, err
			}
		}

		blogIdInt, err := mutil.RandomId(tx, "blogs")
		if err != nil {
			return nil, err
		}
		blogId := models.BlogId(blogIdInt)
		_, err = tx.Exec(`
			insert into blogs(id, name, feed_url, url, status, status_updated_at, version, update_action)
			values ($1, $2, $3, $4, $5, utc_now(), $6, $7)
		`, blogId, name, feedUrl, blogUrl, models.BlogStatusManuallyInserted, models.BlogLatestVersion,
			updateAction)
		if err != nil {
			return nil, err
		}

		batch := tx.NewBatch()
		type CreatedBlogPost struct {
			Id  models.BlogPostId
			Url string
		}
		var createdPosts []CreatedBlogPost
		for i, urlTitle := range postUrlsTitles {
			batch.
				Queue(`
					insert into blog_posts(blog_id, index, url, title)
					values ($1, $2, $3, $4)
					returning id, url
				`, blogId, int32(i), urlTitle.Url, urlTitle.Label).
				QueryRow(func(row pgw.Row) error {
					var p CreatedBlogPost
					err := row.Scan(&p.Id, &p.Url)
					if err != nil {
						return err
					}

					createdPosts = append(createdPosts, p)
					return nil
				})
		}
		err = tx.SendBatch(batch).Close()
		if err != nil {
			return nil, err
		}

		blogPostIdsByUrl := make(map[string][]models.BlogPostId)
		for _, post := range createdPosts {
			blogPostIdsByUrl[post.Url] = append(blogPostIdsByUrl[post.Url], post.Id)
		}

		var newCategories []models.NewBlogPostCategory
		for i, name := range postCategories {
			newCategories = append(newCategories, models.NewBlogPostCategory{
				Name:      name,
				Index:     int32(i),
				TopStatus: topStatusByCategoryName[name],
			})
		}
		batch = tx.NewBatch()
		categoryIdsByName := make(map[string]models.BlogPostCategoryId)
		for _, category := range newCategories {
			batch.
				Queue(`
					insert into blog_post_categories(blog_id, name, index, top_status)
					values ($1, $2, $3, $4)
					returning id, name
				`, blogId, category.Name, category.Index, category.TopStatus).
				QueryRow(func(row pgw.Row) error {
					var id models.BlogPostCategoryId
					var name string
					err := row.Scan(&id, &name)
					if err != nil {
						return err
					}

					categoryIdsByName[name] = id
					return nil
				})
		}
		err = tx.SendBatch(batch).Close()
		if err != nil {
			return nil, err
		}

		batch = tx.NewBatch()
		for _, urlCategory := range postUrlsCategories {
			for _, blogPostId := range blogPostIdsByUrl[urlCategory.Url] {
				batch.Queue(`
					insert into blog_post_category_assignments(blog_post_id, category_id)
					values ($1, $2)
				`, blogPostId, categoryIdsByName[urlCategory.Label])
			}
		}
		err = tx.SendBatch(batch).Close()
		if err != nil {
			return nil, err
		}

		_, err = tx.Exec(`insert into blog_post_locks (blog_id) values ($1)`, blogId)
		if err != nil {
			return nil, err
		}

		_, err = tx.Exec(`
			insert into blog_canonical_equality_configs(blog_id, same_hosts, expect_tumblr_paths)
			values ($1, $2, $3)
		`, blogId, sameHosts, expectTumblrPaths)
		if err != nil {
			return nil, err
		}

		batch = tx.NewBatch()
		for _, url := range discardedFromFeedEntryUrls {
			batch.Queue(`
				insert into blog_discarded_feed_entries (blog_id, url)
				values ($1, $2)
			`, blogId, url)
		}
		err = tx.SendBatch(batch).Close()
		if err != nil {
			return nil, err
		}

		batch = tx.NewBatch()
		for _, url := range missingFromFeedEntryUrls {
			batch.Queue(`
				insert into blog_missing_from_feed_entries (blog_id, url)
				values ($1, $2)
			`, blogId, url)
		}
		err = tx.SendBatch(batch).Close()
		if err != nil {
			return nil, err
		}

		var resultName string
		var resultFeedUrl string
		var postsCount int
		var categoriesCount int
		row := tx.QueryRow(`
			select
				name,
				feed_url,
				(select count(1) from blog_posts where blog_id = $1),
				(select count(1) from blog_post_categories where blog_id = $1)
			from blogs
			where id = $1
		`, blogId)
		err = row.Scan(&resultName, &resultFeedUrl, &postsCount, &categoriesCount)
		if err != nil {
			return nil, err
		}

		return &BlogWithCounts{
			Name:            resultName,
			FeedUrl:         resultFeedUrl,
			PostsCount:      postsCount,
			CategoriesCount: categoriesCount,
		}, nil
	}

	var message string
	var title string
	blog, err := postBlogImpl()
	if err != nil {
		if oopsErr, ok := err.(*oops.Error); ok {
			message = oopsErr.FullString()
		} else {
			message = err.Error()
		}
		title = "Blog not added"
	} else {
		message = fmt.Sprintf(
			"Created %q (%s) with %d posts and %d categories",
			blog.Name, blog.FeedUrl, blog.PostsCount, blog.CategoriesCount,
		)
		title = "Blog added"
	}

	type Result struct {
		Title   string
		Session *util.Session
		Message string
	}
	templates.MustWrite(w, "admin/post_blog", Result{
		Title:   title,
		Session: rutil.Session(r),
		Message: message,
	})
}

type urlLabel struct {
	Url   string
	Label string
}

func admin_ParseUrlsLabels(text string) ([]urlLabel, error) {
	if text == "" {
		return nil, nil
	}

	lines := strings.Split(text, "\n")
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	var urlsLabels []urlLabel
	for i, line := range lines {
		if !strings.HasPrefix(line, "http://") && !strings.HasPrefix(line, "https://") {
			return nil, oops.Newf("Line %d doesn't start with a full url: %s", i+1, line)
		}
		if !strings.Contains(line, " ") {
			return nil, oops.Newf("Line %d doesn't have a space between url and title: %s", i+1, line)
		}
		tokens := strings.SplitN(line, " ", 2)
		urlsLabels = append(urlsLabels, urlLabel{
			Url:   tokens[0],
			Label: tokens[1],
		})
	}
	return urlsLabels, nil
}

func Admin_Dashboard(w http.ResponseWriter, r *http.Request) {
	pool := rutil.DBPool(r)

	type JobItem struct {
		IsName                bool
		Name                  string
		Id                    int64
		NegativeStartPercent  float64
		NegativeLengthPercent float64
		PositiveStartPercent  float64
		PositiveLengthPercent float64
		NegativeValue         float64
		PositiveValue         float64
		NegativeValueStr      string
		PositiveValueStr      string
		Hover                 string
		ShowDeleteButton      bool
		DeletePath            string
	}
	type JobTick struct {
		Percent float64
		Label   string
		IsZero  bool
	}
	row := pool.QueryRow(`
		select
			max(utc_now() - locked_at),
			max(coalesce(locked_at, utc_now()) - run_at),
			sum((locked_at is not null)::int),
			sum((utc_now() > run_at)::int),
			count(1)
		from delayed_jobs
	`)
	var maybeMaxRunning, maybeMaxWaiting *time.Duration
	var jobsRunning, jobsWaiting, jobsScheduled int
	err := row.Scan(&maybeMaxRunning, &maybeMaxWaiting, &jobsRunning, &jobsWaiting, &jobsScheduled)
	if err != nil {
		panic(err)
	}
	var jobItems []JobItem
	var jobTicks []JobTick
	if maybeMaxRunning != nil || maybeMaxWaiting != nil {
		var maxRunning float64
		if maybeMaxRunning != nil {
			maxRunning = maybeMaxRunning.Seconds()
		}
		var maxWaiting float64
		if maybeMaxWaiting != nil {
			maxWaiting = maybeMaxWaiting.Seconds()
		}
		maxDuration := maxRunning
		if maxWaiting > maxDuration {
			maxDuration = maxWaiting
		}
		var scaleMax float64
		maxDuration10 := math.Pow10(int(math.Ceil(math.Log10(maxDuration))))
		switch {
		case maxDuration10/maxDuration >= 5:
			scaleMax = maxDuration10 / 5
		case maxDuration10/maxDuration >= 2:
			scaleMax = maxDuration10 / 2
		default:
			scaleMax = maxDuration10
		}
		smallerSide := maxWaiting
		if maxRunning < maxWaiting {
			smallerSide = maxRunning
		}
		var smallerSideTicks int
		var smallerSideScale float64
		switch {
		case smallerSide < 1:
			smallerSideScale = scaleMax / 10
			smallerSideTicks = 1
		case scaleMax/smallerSide >= 5:
			smallerSideScale = scaleMax / 5
			smallerSideTicks = 2
		case scaleMax/smallerSide >= 2:
			smallerSideScale = scaleMax / 2
			smallerSideTicks = 5
		default:
			smallerSideScale = scaleMax
			smallerSideTicks = 10
		}

		twoSideScale := smallerSideScale + scaleMax
		var tickCount = 11 + smallerSideTicks
		minTick, maxTick := -smallerSideScale, scaleMax
		zeroPercent := smallerSideScale * 100 / twoSideScale
		zeroTick := smallerSideTicks
		if maxWaiting > maxRunning {
			minTick, maxTick = -scaleMax, smallerSideScale
			zeroPercent = scaleMax * 100 / twoSideScale
			zeroTick = 10
		}
		for i := range tickCount {
			fraction := float64(i) / float64(tickCount-1)
			value := maxTick*fraction + minTick*(1.0-fraction)
			jobTicks = append(jobTicks, JobTick{
				Percent: fraction * 100,
				Label:   fmt.Sprintf("%.0f", value),
				IsZero:  i == zeroTick,
			})
		}

		type JobRow struct {
			Id            int64
			Handler       string
			RunAt         time.Time
			Attempts      int
			MaybeLockedAt *time.Time
			MaybeLockedBy *string
			UtcNow        time.Time
		}
		rows, err := pool.Query(`
			select id, handler, run_at, attempts, locked_at, locked_by, utc_now() from delayed_jobs
			where locked_at is not null or run_at < utc_now()
		`)
		if err != nil {
			panic(err)
		}
		var jobRows []JobRow
		for rows.Next() {
			var r JobRow
			err := rows.Scan(
				&r.Id, &r.Handler, &r.RunAt, &r.Attempts, &r.MaybeLockedAt, &r.MaybeLockedBy, &r.UtcNow,
			)
			if err != nil {
				panic(err)
			}
			jobRows = append(jobRows, r)
		}
		if err := rows.Err(); err != nil {
			panic(err)
		}
		for _, jobRow := range jobRows {
			var handler jobs.Handler
			err = yaml.Unmarshal([]byte(jobRow.Handler), &handler)
			if err != nil {
				panic(err)
			}

			name := handler.Job_Data.Job_Class
			waitingTime := jobRow.UtcNow.Sub(jobRow.RunAt)
			var runningTime time.Duration
			positiveValue := 0.0
			positiveLengthPercent := 0.0
			positiveStartPercent := zeroPercent
			const minLengthPercent = 5
			if jobRow.MaybeLockedAt != nil {
				waitingTime = jobRow.MaybeLockedAt.Sub(jobRow.RunAt)
				runningTime = jobRow.UtcNow.Sub(*jobRow.MaybeLockedAt)
				positiveValue = runningTime.Seconds()
				positiveLengthPercent = positiveValue * 100 / twoSideScale
				if positiveLengthPercent < minLengthPercent {
					positiveLengthPercent = minLengthPercent
				}

			}
			negativeValue := waitingTime.Seconds()
			negativeLengthPercent := negativeValue * 100 / twoSideScale
			negativeStartPercent := zeroPercent - negativeLengthPercent
			if negativeLengthPercent < minLengthPercent {
				negativeStartPercent -= (minLengthPercent - negativeLengthPercent)
				negativeLengthPercent = minLengthPercent
			}

			var hover strings.Builder
			fmt.Fprintf(&hover, "%s\n", name)
			fmt.Fprintf(&hover, "waiting time: %v\n", waitingTime)
			if runningTime > 0 {
				fmt.Fprintf(&hover, "running time: %v\n", runningTime)
			}
			if jobRow.MaybeLockedBy != nil {
				fmt.Fprintf(&hover, "locked by: %s\n", *jobRow.MaybeLockedBy)
			}
			if name == "GuidedCrawlingJob" {
				blogId, ok := handler.Job_Data.Arguments[0].(int64)
				if !ok {
					blogIdInt, ok := handler.Job_Data.Arguments[0].(int)
					if !ok {
						panic(oops.Newf(
							"Failed to parse blogId (expected int64 or int): %v",
							handler.Job_Data.Arguments[0],
						))
					}
					blogId = int64(blogIdInt)
				}
				row := pool.QueryRow(`select feed_url from blogs where id = $1`, blogId)
				var feedUrl string
				err := row.Scan(&feedUrl)
				if err != nil {
					panic(err)
				}
				fmt.Fprintf(&hover, "feed url: %s\n", feedUrl)
			}
			if jobRow.Attempts > 0 {
				fmt.Fprintf(&hover, "attempts: %d\n", jobRow.Attempts)
			}
			fmt.Fprintf(&hover, "id: %d\n", jobRow.Id)
			fmt.Fprintf(&hover, "\n%s", jobRow.Handler)

			jobItems = append(jobItems, JobItem{
				IsName:                false,
				Name:                  name,
				Id:                    jobRow.Id,
				NegativeStartPercent:  negativeStartPercent,
				NegativeLengthPercent: negativeLengthPercent,
				PositiveStartPercent:  positiveStartPercent,
				PositiveLengthPercent: positiveLengthPercent,
				NegativeValue:         negativeValue,
				PositiveValue:         positiveValue,
				NegativeValueStr:      fmt.Sprintf("%.1f", negativeValue),
				PositiveValueStr:      fmt.Sprintf("%.1f", positiveValue),
				Hover:                 hover.String(),
				ShowDeleteButton:      name == "GuidedCrawlingJob",
				DeletePath:            fmt.Sprintf("/admin/job/%d/delete", jobRow.Id),
			})
		}

		type NameSortKey struct {
			MaxPositiveValue float64
			MaxNegativeValue float64
			MinId            int64
		}
		sortKeysByName := map[string]*NameSortKey{}
		for _, jobItem := range jobItems {
			if _, ok := sortKeysByName[jobItem.Name]; !ok {
				sortKeysByName[jobItem.Name] = &NameSortKey{
					MaxPositiveValue: 0,
					MaxNegativeValue: 0,
					MinId:            math.MaxInt64,
				}
			}
			sortKey := sortKeysByName[jobItem.Name]
			sortKey.MaxPositiveValue = max(sortKey.MaxPositiveValue, jobItem.PositiveValue)
			sortKey.MaxNegativeValue = max(sortKey.MaxNegativeValue, jobItem.NegativeValue)
			sortKey.MinId = max(sortKey.MinId, jobItem.Id)
		}
		slices.SortFunc(jobItems, func(a, b JobItem) int {
			// Name
			sortKeyA := sortKeysByName[a.Name]
			sortKeyB := sortKeysByName[b.Name]
			if n := cmp.Compare(sortKeyA.MaxPositiveValue, sortKeyB.MaxPositiveValue); n != 0 {
				return -n
			}
			if n := cmp.Compare(sortKeyA.MaxNegativeValue, sortKeyB.MaxNegativeValue); n != 0 {
				return -n
			}
			if n := cmp.Compare(sortKeyA.MinId, sortKeyB.MinId); n != 0 {
				return n
			}
			// Running time descending
			if n := cmp.Compare(a.PositiveValue, b.PositiveValue); n != 0 {
				return -n
			}
			// Waiting time descending
			if n := cmp.Compare(a.NegativeValue, b.NegativeValue); n != 0 {
				return -n
			}
			// id
			return cmp.Compare(a.Id, b.Id)
		})

		itemIdx := 0
		lastName := ""
		for itemIdx < len(jobItems) {
			jobItem := jobItems[itemIdx]
			if jobItem.Name != lastName {
				jobItems = slices.Insert(jobItems, itemIdx, JobItem{
					IsName:                true,
					Name:                  jobItem.Name,
					Id:                    0,
					NegativeStartPercent:  0,
					NegativeLengthPercent: 0,
					PositiveStartPercent:  0,
					PositiveLengthPercent: 0,
					NegativeValue:         0,
					PositiveValue:         0,
					NegativeValueStr:      "",
					PositiveValueStr:      "",
					Hover:                 "",
					ShowDeleteButton:      false,
					DeletePath:            "",
				})
				lastName = jobItem.Name
				itemIdx++
			}
			itemIdx++
		}
	}

	type AdminTelemetry struct {
		Key       string
		Value     float64
		Extra     map[string]any
		CreatedAt time.Time
	}
	year, month, day := time.Now().UTC().AddDate(0, 0, -6).Date()
	weekAgo := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	rows, err := pool.Query(`
		select key, value, extra, created_at from admin_telemetries
		where created_at > $1
		order by created_at asc
	`, weekAgo)
	if err != nil {
		panic(err)
	}

	var telemetries []AdminTelemetry
	for rows.Next() {
		var t AdminTelemetry
		err := rows.Scan(&t.Key, &t.Value, &t.Extra, &t.CreatedAt)
		if err != nil {
			panic(err)
		}

		telemetries = append(telemetries, t)
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}

	telemetriesByKey := make(map[string][]AdminTelemetry)
	for _, telemetry := range telemetries {
		telemetriesByKey[telemetry.Key] = append(telemetriesByKey[telemetry.Key], telemetry)
	}

	type Tick struct {
		TopPercent int
		Label      string
	}
	type Item struct {
		IsDate       bool
		DateStr      string
		ValuePercent float64
		Value        string
		Hover        string
	}
	type Dashboard struct {
		Key   string
		Ticks []Tick
		Items []Item
	}
	priorityKeys := []string{"guided_crawling_job_success", "guided_crawling_job_failure"}
	var sortedKeys []string
	for key := range telemetriesByKey {
		sortedKeys = append(sortedKeys, key)
	}
	slices.SortFunc(sortedKeys, func(a, b string) int {
		for _, priorityKey := range priorityKeys {
			if a == priorityKey {
				return -1
			} else if b == priorityKey {
				return 0
			}
		}
		return strings.Compare(a, b)
	})

	var dashboards []Dashboard
	for _, key := range sortedKeys {
		keyTelemetries := telemetriesByKey[key]

		yMax := 0.0
		for _, telemetry := range keyTelemetries {
			if telemetry.Value > yMax {
				yMax = telemetry.Value
			}
		}
		yScaleMax := 1.0
		if yMax > 0 {
			yMax10 := math.Pow10(int(math.Ceil(math.Log10(yMax))))
			switch {
			case yMax10/yMax >= 5:
				yScaleMax = yMax10 / 5
			case yMax10/yMax >= 2:
				yScaleMax = yMax10 / 2
			default:
				yScaleMax = yMax10
			}
		}

		var ticks []Tick
		for i := 0; i <= 10; i++ {
			ticks = append(ticks, Tick{
				TopPercent: i * 10,
				Label:      fmt.Sprint(yScaleMax * float64(10-i) / 10),
			})
		}

		var items []Item
		var prevDate time.Time
		var prevDateStr string
		for _, telemetry := range keyTelemetries {
			dateStr := telemetry.CreatedAt.Format("2006-01-02")
			if dateStr != prevDateStr {
				if prevDateStr == "" {
					prevDate = telemetry.CreatedAt
					prevDateStr = dateStr
					items = append(items, Item{ //nolint:exhaustruct
						IsDate:  true,
						DateStr: prevDateStr,
					})
				} else {
					for prevDateStr != dateStr {
						prevDate = prevDate.AddDate(0, 0, 1)
						prevDateStr = prevDate.Format("2006-01-02")
						items = append(items, Item{ //nolint:exhaustruct
							IsDate:  true,
							DateStr: prevDateStr,
						})
					}
				}
			}

			formattedValue := strconv.FormatFloat(telemetry.Value, 'f', -1, 64)
			valuePercent := 5.0
			if telemetry.Value >= 0 {
				valuePercent = telemetry.Value * 100 / yScaleMax
			}

			var hover strings.Builder
			fmt.Fprintf(&hover, "value: %s", formattedValue)
			fmt.Fprintf(&hover, "\ntimestamp: %s", telemetry.CreatedAt.Format("15:04:05 MST"))
			var extraKeys []string
			for extraKey := range telemetry.Extra {
				extraKeys = append(extraKeys, extraKey)
			}
			slices.Sort(extraKeys)
			for _, extraKey := range extraKeys {
				fmt.Fprintf(&hover, "\n%s: %v", extraKey, telemetry.Extra[extraKey])
			}

			items = append(items, Item{
				IsDate:       false,
				DateStr:      "",
				ValuePercent: valuePercent,
				Value:        formattedValue,
				Hover:        hover.String(),
			})
		}

		dashboards = append(dashboards, Dashboard{
			Key:   key,
			Ticks: ticks,
			Items: items,
		})
	}

	type Result struct {
		Title         string
		Session       *util.Session
		JobItems      []JobItem
		JobTicks      []JobTick
		JobsRunning   int
		JobsWaiting   int
		JobsScheduled int
		Dashboards    []Dashboard
	}
	templates.MustWrite(w, "admin/dashboard", Result{
		Title:         "Dashboard",
		Session:       rutil.Session(r),
		JobItems:      jobItems,
		JobTicks:      jobTicks,
		JobsRunning:   jobsRunning,
		JobsWaiting:   jobsWaiting,
		JobsScheduled: jobsScheduled,
		Dashboards:    dashboards,
	})
}

func Admin_DeleteJob(w http.ResponseWriter, r *http.Request) {
	pool := rutil.DBPool(r)
	logger := rutil.Logger(r)
	session := rutil.Session(r)
	jobId, ok := util.URLParamInt64(r, "id")
	if !ok {
		panic(oops.Newf("Bad job id: %d", jobId))
	}
	type Result struct {
		Title   string
		Session *util.Session
		Message string
	}
	result, err := util.TxReturn(pool, func(tx *pgw.Tx, pool util.Clobber) (*Result, error) {
		row := tx.QueryRow(`select handler from delayed_jobs where id = $1`, jobId)
		var handlerStr string
		err := row.Scan(&handlerStr)
		if errors.Is(err, pgx.ErrNoRows) {
			logger.Info().Msgf("Job already deleted")
			return &Result{
				Title:   "Job already deleted",
				Session: session,
				Message: fmt.Sprintf("Job %d is already deleted", jobId),
			}, nil
		} else if err != nil {
			return nil, err
		}

		var handler jobs.Handler
		err = yaml.Unmarshal([]byte(handlerStr), &handler)
		if err != nil {
			return nil, err
		}

		if handler.Job_Data.Job_Class != "GuidedCrawlingJob" {
			return nil, oops.New("Can only delete GuidedCrawlingJobs")
		}

		_, err = tx.Exec(`delete from delayed_jobs where id = $1`, jobId)
		if err != nil {
			return nil, err
		}
		logger.Info().Msgf("Deleted job %d", jobId)

		var blogId models.BlogId
		blogIdInt64, ok := handler.Job_Data.Arguments[0].(int64)
		if !ok {
			blogIdInt, ok := handler.Job_Data.Arguments[0].(int)
			if !ok {
				return nil, oops.Newf(
					"Failed to parse blogId (expected int64 or int): %v",
					handler.Job_Data.Arguments[0],
				)
			}
			blogId = models.BlogId(blogIdInt)
		} else {
			blogId = models.BlogId(blogIdInt64)
		}
		_, err = tx.Exec(`update blogs set status = $1 where id = $2`, models.BlogStatusCrawlFailed, blogId)
		if err != nil {
			return nil, err
		}

		payload := map[string]any{
			"blog_id": fmt.Sprint(blogId),
			"done":    true,
		}
		payloadBytes, err := json.Marshal(&payload)
		if err != nil {
			return nil, oops.Wrap(err)
		}
		_, err = tx.Exec(`select pg_notify($1, $2)`, jobs.CrawlProgressChannelName, string(payloadBytes))
		if err != nil {
			return nil, err
		}

		return &Result{
			Title:   "Job deleted",
			Session: session,
			Message: fmt.Sprintf("Deleted job %d, marked blog %d as crawl failed", jobId, blogId),
		}, nil
	})
	if err != nil {
		panic(err)
	}

	templates.MustWrite(w, "admin/delete_job", *result)
}
