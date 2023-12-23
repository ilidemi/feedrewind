package routes

import (
	"feedrewind/crawler"
	"feedrewind/db/pgw"
	"feedrewind/models"
	"feedrewind/models/mutil"
	"feedrewind/oops"
	"feedrewind/routes/rutil"
	"feedrewind/templates"
	"feedrewind/util"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
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
		postCategories := make([]string, len(topCategories))
		copy(postCategories, topCategories)
		topCategoriesSet := make(map[string]bool)
		postCategoriesSet := make(map[string]bool)
		for _, topCategory := range topCategories {
			topCategoriesSet[topCategory] = true
			postCategoriesSet[topCategory] = true
		}
		for _, urlCategory := range postUrlsCategories {
			if postCategoriesSet[urlCategory.Label] {
				continue
			}
			postCategories = append(postCategories, urlCategory.Label)
			postCategoriesSet[urlCategory.Label] = true
		}

		topCategoriesSet["Everything"] = true
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
		zlogger := crawler.ZeroLogger{Logger: logger}
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
			httpClient := crawler.NewHttpClientImpl(r.Context(), false)
			progressLogger := crawler.NewMockProgressLogger(&zlogger)
			crawlCtx := crawler.NewCrawlContext(httpClient, nil, &progressLogger)
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

		conn := rutil.DBConn(r)
		tx, err := conn.Begin()
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
				Name:  name,
				Index: int32(i),
				IsTop: topCategoriesSet[name],
			})
		}
		batch = tx.NewBatch()
		categoryIdsByName := make(map[string]models.BlogPostCategoryId)
		for _, category := range newCategories {
			batch.
				Queue(`
					insert into blog_post_categories(blog_id, name, index, is_top)
					values ($1, $2, $3, $4)
					returning id, name
				`, blogId, category.Name, category.Index, category.IsTop).
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
		message = err.Error()
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
	conn := rutil.DBConn(r)
	type AdminTelemetry struct {
		Key       string
		Value     float64
		Extra     map[string]any
		CreatedAt time.Time
	}
	year, month, day := time.Now().UTC().AddDate(0, 0, -6).Date()
	weekAgo := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	rows, err := conn.Query(`
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
	sort.Slice(sortedKeys, func(i, j int) bool {
		for _, priorityKey := range priorityKeys {
			if sortedKeys[i] == priorityKey {
				return true
			} else if sortedKeys[j] == priorityKey {
				return false
			}
		}
		return strings.Compare(sortedKeys[i], sortedKeys[j]) == -1
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
			if yMax10/yMax >= 5 {
				yScaleMax = yMax10 / 5
			} else if yMax10/yMax >= 2 {
				yScaleMax = yMax10 / 2
			} else {
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
			sort.Strings(extraKeys)
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
		Title      string
		Session    *util.Session
		Dashboards []Dashboard
	}
	templates.MustWrite(w, "admin/dashboard", Result{
		Title:      "Dashboard",
		Session:    rutil.Session(r),
		Dashboards: dashboards,
	})
}
