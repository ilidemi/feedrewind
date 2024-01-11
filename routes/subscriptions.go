package routes

import (
	"bytes"
	"feedrewind/crawler"
	"feedrewind/db"
	"feedrewind/db/pgw"
	"feedrewind/jobs"
	"feedrewind/models"
	"feedrewind/publish"
	"feedrewind/routes/rutil"
	"feedrewind/templates"
	"feedrewind/third_party/tzdata"
	"feedrewind/util"
	"feedrewind/util/schedule"
	"fmt"
	"html/template"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/goccy/go-json"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pkg/errors"
)

func Subscriptions_Index(w http.ResponseWriter, r *http.Request) {
	conn := rutil.DBConn(r)
	currentUser := rutil.CurrentUser(r)

	rows, err := conn.Query(`
		with user_subscriptions as (
			select id, name, status, is_paused, finished_setup_at, created_at
			from subscriptions_without_discarded
			where user_id = $1
		)
		select id, name, status, is_paused, published_count, total_count
		from user_subscriptions
		left join (
			select subscription_id,
				count(published_at) as published_count,
				count(1) as total_count
			from subscription_posts
			where subscription_id in (select id from user_subscriptions)
			group by subscription_id
		) as post_counts on subscription_id = id
		order by finished_setup_at desc, created_at desc
	`, currentUser.Id)
	if err != nil {
		panic(err)
	}
	type Subscription struct {
		Id             models.SubscriptionId
		Name           string
		Status         models.SubscriptionStatus
		IsPaused       bool
		PublishedCount int
		TotalCount     int
	}
	var subscriptions []Subscription
	for rows.Next() {
		var s Subscription
		var publishedCount, totalCount *int
		err := rows.Scan(&s.Id, &s.Name, &s.Status, &s.IsPaused, &publishedCount, &totalCount)
		if err != nil {
			panic(err)
		}
		if publishedCount != nil {
			s.PublishedCount = *publishedCount
		}
		if totalCount != nil {
			s.TotalCount = *totalCount
		}
		subscriptions = append(subscriptions, s)
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}

	var settingUpSubscriptions []Subscription
	var activeSubscriptions []Subscription
	var finishedSubscriptions []Subscription
	for _, subscription := range subscriptions {
		if subscription.Status != models.SubscriptionStatusLive {
			settingUpSubscriptions = append(settingUpSubscriptions, subscription)
		} else if subscription.PublishedCount < subscription.TotalCount {
			activeSubscriptions = append(activeSubscriptions, subscription)
		} else {
			finishedSubscriptions = append(finishedSubscriptions, subscription)
		}
	}

	type DashboardSubscription struct {
		Id             models.SubscriptionId
		Name           string
		IsPaused       bool
		SetupPath      string
		DeletePath     string
		ShowPath       string
		PublishedCount int
		TotalCount     int
	}

	createDashboardSubscription := func(s Subscription) DashboardSubscription {
		return DashboardSubscription{
			Id:             s.Id,
			Name:           s.Name,
			IsPaused:       s.IsPaused,
			SetupPath:      rutil.SubscriptionSetupPath(s.Id),
			DeletePath:     rutil.SubscriptionDeletePath(s.Id),
			ShowPath:       rutil.SubscriptionShowPath(s.Id),
			PublishedCount: s.PublishedCount,
			TotalCount:     s.TotalCount,
		}
	}

	type DashboardResult struct {
		Title                  string
		Session                *util.Session
		HasSubscriptions       bool
		SettingUpSubscriptions []DashboardSubscription
		ActiveSubscriptions    []DashboardSubscription
		FinishedSubscriptions  []DashboardSubscription
	}
	result := DashboardResult{
		Title:                  util.DecorateTitle("Dashboard"),
		Session:                rutil.Session(r),
		HasSubscriptions:       len(subscriptions) > 0,
		SettingUpSubscriptions: nil,
		ActiveSubscriptions:    nil,
		FinishedSubscriptions:  nil,
	}
	for _, subscription := range settingUpSubscriptions {
		result.SettingUpSubscriptions =
			append(result.SettingUpSubscriptions, createDashboardSubscription(subscription))
	}
	for _, subscription := range activeSubscriptions {
		result.ActiveSubscriptions =
			append(result.ActiveSubscriptions, createDashboardSubscription(subscription))
	}
	for _, subscription := range finishedSubscriptions {
		result.FinishedSubscriptions =
			append(result.FinishedSubscriptions, createDashboardSubscription(subscription))
	}

	templates.MustWrite(w, "subscriptions/index", result)
}

type subscriptionsScheduleResult struct {
	Name               string
	CurrentCountByDay  map[schedule.DayOfWeek]int
	HasOtherSubs       bool
	OtherSubNamesByDay map[schedule.DayOfWeek][]string
	DaysOfWeek         []schedule.DayOfWeek
}

type subscriptionsScheduleJsResult struct {
	DaysOfWeekJson        template.JS
	ValidateCallback      template.JS
	SetNameChangeCallback template.JS
}

func Subscriptions_Show(w http.ResponseWriter, r *http.Request) {
	conn := rutil.DBConn(r)
	subscriptionIdInt, ok := util.URLParamInt64(r, "id")
	if !ok {
		subscriptions_RedirectNotFound(w, r)
		return
	}

	subscriptionId := models.SubscriptionId(subscriptionIdInt)
	currentUser := rutil.CurrentUser(r)

	var name string
	var isPaused bool
	var status models.SubscriptionStatus
	var scheduleVersion int64
	var isAddedPastMidnight bool
	var url string
	var publishedCount int
	var totalCount int
	row := conn.QueryRow(`
		select name, is_paused, status, schedule_version, is_added_past_midnight,
			(select url from blogs where id = blog_id) as url,
			(
				select count(published_at) from subscription_posts
				where subscription_id = subscriptions_without_discarded.id
			) as published_count,
			(
				select count(1) from subscription_posts
				where subscription_id = subscriptions_without_discarded.id
			) as total_count
		from subscriptions_without_discarded
		where id = $1 and user_id = $2
	`, subscriptionId, currentUser.Id)
	err := row.Scan(
		&name, &isPaused, &status, &scheduleVersion, &isAddedPastMidnight, &url, &publishedCount, &totalCount,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		subscriptions_RedirectNotFound(w, r)
		return
	} else if err != nil {
		panic(err)
	}

	if status != models.SubscriptionStatusLive {
		http.Redirect(w, r, rutil.SubscriptionSetupPath(subscriptionId), http.StatusFound)
		return
	}

	userSettings, err := models.UserSettings_Get(conn, currentUser.Id)
	if err != nil {
		panic(err)
	}
	feedUrl := ""
	if *userSettings.DeliveryChannel != models.DeliveryChannelEmail {
		feedUrl = rutil.SubscriptionFeedUrl(r, subscriptionId)
	}
	countByDay, err := models.Schedule_GetCountsByDay(conn, subscriptionId)
	if err != nil {
		panic(err)
	}
	otherSubNamesByDay, err := models.Subscription_GetOtherNamesByDay(conn, subscriptionId, currentUser.Id)
	if err != nil {
		panic(err)
	}
	preview := subscriptions_MustGetSchedulePreview(
		conn, subscriptionId, status, currentUser.Id, userSettings,
	)

	type SubscriptionResult struct {
		Title           string
		Session         *util.Session
		Name            string
		FeedUrl         string
		IsDone          bool
		IsPaused        bool
		PublishedCount  int
		TotalCount      int
		Url             string
		PausePath       string
		UnpausePath     string
		Schedule        subscriptionsScheduleResult
		ScheduleVersion int64
		SchedulePreview schedulePreview
		ScheduleJS      subscriptionsScheduleJsResult
		DeletePath      string
	}
	templates.MustWrite(w, "subscriptions/show", SubscriptionResult{
		Title:          util.DecorateTitle(name),
		Session:        rutil.Session(r),
		Name:           name,
		FeedUrl:        feedUrl,
		IsDone:         publishedCount >= totalCount,
		IsPaused:       isPaused,
		PublishedCount: publishedCount,
		TotalCount:     totalCount,
		Url:            url,
		PausePath:      rutil.SubscriptionPausePath(subscriptionId),
		UnpausePath:    rutil.SubscriptionUnpausePath(subscriptionId),
		Schedule: subscriptionsScheduleResult{
			Name:               name,
			CurrentCountByDay:  countByDay,
			HasOtherSubs:       len(otherSubNamesByDay) > 0,
			OtherSubNamesByDay: otherSubNamesByDay,
			DaysOfWeek:         schedule.DaysOfWeek,
		},
		ScheduleVersion: scheduleVersion,
		SchedulePreview: preview,
		ScheduleJS: subscriptionsScheduleJsResult{
			DaysOfWeekJson:        schedule.DaysOfWeekJson,
			ValidateCallback:      template.JS("onValidateSchedule"),
			SetNameChangeCallback: template.JS("setNameChangeScheduleCallback"),
		},
		DeletePath: rutil.SubscriptionDeletePath(subscriptionId),
	})
}

func Subscriptions_Create(w http.ResponseWriter, r *http.Request) {
	logger := rutil.Logger(r)
	conn := rutil.DBConn(r)
	currentUser := rutil.CurrentUser(r)
	productUserId := rutil.CurrentProductUserId(r)
	userIsAnonymous := currentUser == nil
	pc := models.NewProductEventContext(conn, r, productUserId)
	startFeedId := models.StartFeedId(util.EnsureParamInt64(r, "start_feed_id"))
	startFeed, err := models.StartFeed_GetUnfetched(conn, startFeedId)
	if err != nil {
		panic(err)
	}

	// Feeds that were fetched were handled in onboarding, this one needs to be fetched
	httpClient := crawler.NewHttpClientImpl(r.Context(), false)
	zlogger := crawler.ZeroLogger{Logger: logger}
	progressLogger := crawler.NewMockProgressLogger(&zlogger)
	crawlCtx := crawler.NewCrawlContext(httpClient, nil, &progressLogger)
	fetchFeedResult := crawler.FetchFeedAtUrl(startFeed.Url, true, &crawlCtx, &zlogger)
	switch fetchResult := fetchFeedResult.(type) {
	case *crawler.FetchedPage:
		finalUrl := fetchResult.Page.FetchUri.String()
		parsedFeed, err := crawler.ParseFeed(fetchResult.Page.Content, fetchResult.Page.FetchUri, &zlogger)
		if err != nil {
			models.ProductEvent_MustEmitDiscoverFeeds(
				pc, startFeed.Url, models.TypedBlogUrlResultBadFeed, userIsAnonymous,
			)
			w.WriteHeader(http.StatusUnsupportedMediaType)
			return
		}
		startFeed, err = models.StartFeed_UpdateFetched(
			conn, startFeed, finalUrl, fetchResult.Page.Content, parsedFeed,
		)
		if err != nil {
			panic(err)
		}
		updatedBlog, err := models.Blog_CreateOrUpdate(conn, startFeed, jobs.GuidedCrawlingJob_PerformNow)
		if err != nil {
			panic(err)
		}
		subscriptionCreateResult, err := models.Subscription_CreateForBlog(
			conn, updatedBlog, currentUser, productUserId,
		)
		if errors.Is(err, models.ErrBlogFailed) {
			models.ProductEvent_MustEmitDiscoverFeeds(
				pc, startFeed.Url, models.TypedBlogUrlResultKnownUnsupported, userIsAnonymous,
			)
			util.MustWrite(w, rutil.BlogUnsupportedPath(updatedBlog.Id))
			return
		} else if err != nil {
			panic(err)
		}

		models.ProductEvent_MustEmitDiscoverFeeds(
			pc, startFeed.Url, models.TypedBlogUrlResultFeed, userIsAnonymous,
		)
		models.ProductEvent_MustEmitCreateSubscription(pc, subscriptionCreateResult, userIsAnonymous)
		util.MustWrite(w, rutil.SubscriptionSetupPath(subscriptionCreateResult.Id))
		return
	case *crawler.FetchFeedErrorBadFeed:
		models.ProductEvent_MustEmitDiscoverFeeds(
			pc, startFeed.Url, models.TypedBlogUrlResultBadFeed, userIsAnonymous,
		)
		w.WriteHeader(http.StatusUnsupportedMediaType)
		return
	case *crawler.FetchFeedErrorCouldNotReach:
		models.ProductEvent_MustEmitDiscoverFeeds(
			pc, startFeed.Url, models.TypedBlogUrlResultCouldNotReach, userIsAnonymous,
		)
		w.WriteHeader(http.StatusUnsupportedMediaType)
		return
	default:
		panic("Unexpected fetch feed result type")
	}
}

func Subscriptions_Setup(w http.ResponseWriter, r *http.Request) {
	conn := rutil.DBConn(r)
	subscriptionIdInt, ok := util.URLParamInt64(r, "id")
	if !ok {
		subscriptions_RedirectNotFound(w, r)
		return
	}

	subscriptionId := models.SubscriptionId(subscriptionIdInt)
	currentUser := rutil.CurrentUser(r)
	var subscriptionStatus models.SubscriptionStatus
	var blogStatus models.BlogStatus
	var subscriptionName string
	var blogId models.BlogId
	var maybeSubscriptionUserId *models.UserId
	row := conn.QueryRow(`
		select status, (select status from blogs where id = blog_id) as blog_status, name, blog_id, user_id
		from subscriptions_without_discarded
		where id = $1
	`, subscriptionId)
	err := row.Scan(&subscriptionStatus, &blogStatus, &subscriptionName, &blogId, &maybeSubscriptionUserId)
	if errors.Is(err, pgx.ErrNoRows) {
		subscriptions_RedirectNotFound(w, r)
		return
	} else if err != nil {
		panic(err)
	}

	if subscriptions_RedirectIfUserMismatch(w, r, maybeSubscriptionUserId) {
		return
	}

	if currentUser == nil && subscriptionStatus != models.SubscriptionStatusWaitingForBlog {
		http.Redirect(w, r, "/signup", http.StatusFound)
		return
	}

	if currentUser == nil {
		if subscriptionStatus == models.SubscriptionStatusWaitingForBlog &&
			blogStatus == models.BlogStatusCrawlFailed {
			util.DeleteCookie(w, rutil.AnonymousSubscription)
		} else {
			util.SetSessionCookie(w, rutil.AnonymousSubscription, fmt.Sprint(subscriptionId))
		}
	}

	switch subscriptionStatus {
	case models.SubscriptionStatusWaitingForBlog:
		switch blogStatus {
		case models.BlogStatusCrawlInProgress:
			clientToken, err := models.BlogCrawlClientToken_GetById(conn, blogId)
			if errors.Is(err, models.ErrBlogCrawlClientTokenNotFound) {
				clientToken, err = models.BlogCrawlClientToken_Create(conn, blogId)
				if err != nil {
					panic(err)
				}
			} else if err != nil {
				panic(err)
			}

			blogCrawlProgress, err := models.BlogCrawlProgress_Get(conn, blogId)
			if err != nil {
				panic(err)
			}

			type CrawlInProgressResult struct {
				Title                         string
				Session                       *util.Session
				SubscriptionName              string
				SubscriptionId                models.SubscriptionId
				BlogId                        models.BlogId
				ClientToken                   models.BlogCrawlClientToken
				BlogCrawlProgress             models.BlogCrawlProgress
				SubscriptionProgressPath      string
				SubscriptionProgressStreamUrl string
				SubscriptionDeletePath        string
			}
			templates.MustWrite(w, "subscriptions/setup_blog_crawl_in_progress", CrawlInProgressResult{
				Title:                         util.DecorateTitle(subscriptionName),
				Session:                       rutil.Session(r),
				SubscriptionName:              subscriptionName,
				SubscriptionId:                subscriptionId,
				BlogId:                        blogId,
				ClientToken:                   clientToken,
				BlogCrawlProgress:             *blogCrawlProgress,
				SubscriptionProgressPath:      rutil.SubscriptionProgressPath(subscriptionId),
				SubscriptionProgressStreamUrl: rutil.SubscriptionProgressStreamUrl(r, subscriptionId),
				SubscriptionDeletePath:        rutil.SubscriptionDeletePath(subscriptionId),
			})
			return
		case models.BlogStatusCrawledVoting,
			models.BlogStatusCrawledConfirmed,
			models.BlogStatusCrawledLooksWrong,
			models.BlogStatusManuallyInserted:

			type TopPost struct {
				Url        string
				Title      string
				IsEarliest bool
				IsNewest   bool
			}

			type CustomPost struct {
				Id        models.BlogPostId
				Url       string
				Title     string
				IsChecked bool
			}

			type Post struct {
				Id         models.BlogPostId
				Url        string
				Title      string
				IsEarliest bool
				IsNewest   bool
				IsChecked  bool
			}

			type Submit struct {
				Suffix                    string
				SubscriptionDeletePath    string
				SubscriptionMarkWrongPath string
				MarkWrongFuncJS           template.JS
			}

			type TopCategoryPosts struct {
				Suffix             string
				ShowAll            bool
				OrderedPostsAll    []TopPost
				OrderedPostsStart  []TopPost
				MiddleCount        int
				OrderedPostsMiddle []TopPost
				OrderedPostsEnd    []TopPost
			}

			type TopCategory struct {
				Id                          models.BlogPostCategoryId
				Name                        string
				PostsCount                  int
				Posts                       TopCategoryPosts
				BlogPostIdsJS               template.JS
				SubscriptionSelectPostsPath string
				Submit                      Submit
			}

			type CustomCategory struct {
				Name            string
				PostsCount      int
				CheckedCount    int
				IsChecked       bool
				IsIndeterminate bool
				Posts           []CustomPost
			}

			type CrawledResult struct {
				Title                       string
				Session                     *util.Session
				SubscriptionName            string
				TopCategories               []TopCategory
				MarkWrongFuncJS             template.JS
				IsCheckedEverything         bool
				CheckedTopCategoryId        models.BlogPostCategoryId
				CheckedTopCategoryName      string
				CheckedBlogPostIdsCount     int
				CustomCategories            []CustomCategory
				AllPostsCount               int
				AllPosts                    []Post
				SubscriptionSelectPostsPath string
				CustomSubmit                Submit
			}

			allBlogPosts, err := models.BlogPost_List(conn, blogId)
			if err != nil {
				panic(err)
			}
			sort.Slice(allBlogPosts, func(i, j int) bool {
				return allBlogPosts[i].Index < allBlogPosts[j].Index
			})

			blogPostsById := make(map[models.BlogPostId]*models.BlogPost)
			for i := range allBlogPosts {
				blogPostsById[allBlogPosts[i].Id] = &allBlogPosts[i]
			}

			allCategories, err := models.BlogPostCategory_ListOrdered(conn, blogId)
			if err != nil {
				panic(err)
			}

			subscriptionDeletePath := rutil.SubscriptionDeletePath(subscriptionId)
			subscriptionSelectPostsPath := rutil.SubscriptionSelectPostsPath(subscriptionId)
			subscrtipionMarkWrongPath := rutil.SubscriptionMarkWrongPath(subscriptionId)
			markWrongFuncJS := template.JS("markWrong")

			checkedBlogPostIds := make(map[models.BlogPostId]bool)
			for _, category := range allCategories {
				if category.IsTop {
					for blogPostId := range category.BlogPostIds {
						checkedBlogPostIds[blogPostId] = true
					}
					break
				}
			}

			var topCategories []TopCategory
			var customCategories []CustomCategory
			for i, category := range allCategories {
				categoryBlogPosts := make([]*models.BlogPost, 0, len(category.BlogPostIds))
				for i := range allBlogPosts {
					if category.BlogPostIds[allBlogPosts[i].Id] {
						categoryBlogPosts = append(categoryBlogPosts, &allBlogPosts[i])
					}
				}
				if category.IsTop {
					posts := make([]TopPost, 0, len(categoryBlogPosts))
					for _, blogPost := range categoryBlogPosts {
						posts = append(posts, TopPost{
							Url:        blogPost.Url,
							Title:      blogPost.Title,
							IsEarliest: false,
							IsNewest:   false,
						})
					}
					posts[0].IsEarliest = true
					posts[len(posts)-1].IsNewest = true

					suffix := fmt.Sprint(i)
					topPosts := TopCategoryPosts{
						Suffix:             suffix,
						ShowAll:            len(posts) <= 12,
						OrderedPostsAll:    posts,
						OrderedPostsStart:  nil,
						MiddleCount:        0,
						OrderedPostsMiddle: nil,
						OrderedPostsEnd:    nil,
					}
					if !topPosts.ShowAll {
						topPosts.OrderedPostsStart = posts[:5]
						topPosts.MiddleCount = len(posts) - 10
						topPosts.OrderedPostsMiddle = posts[5 : len(posts)-5]
						topPosts.OrderedPostsEnd = posts[len(posts)-5:]
					}

					var idsBuilder strings.Builder
					idsBuilder.WriteString("[")
					isFirst := true
					for blogPostId := range category.BlogPostIds {
						if !isFirst {
							idsBuilder.WriteString(", ")
						}
						isFirst = false
						idsBuilder.WriteString(`"`)
						fmt.Fprint(&idsBuilder, blogPostId)
						idsBuilder.WriteString(`"`)
					}
					idsBuilder.WriteString("]")

					topCategories = append(topCategories, TopCategory{
						Id:                          category.Id,
						Name:                        category.Name,
						PostsCount:                  len(category.BlogPostIds),
						Posts:                       topPosts,
						BlogPostIdsJS:               template.JS(idsBuilder.String()),
						SubscriptionSelectPostsPath: subscriptionSelectPostsPath,
						Submit: Submit{
							Suffix:                    suffix,
							SubscriptionDeletePath:    subscriptionDeletePath,
							SubscriptionMarkWrongPath: subscrtipionMarkWrongPath,
							MarkWrongFuncJS:           markWrongFuncJS,
						},
					})
				} else {
					posts := make([]CustomPost, 0, len(categoryBlogPosts))
					checkedCount := 0
					for _, blogPost := range categoryBlogPosts {
						posts = append(posts, CustomPost{
							Id:        blogPost.Id,
							Url:       blogPost.Url,
							Title:     blogPost.Title,
							IsChecked: checkedBlogPostIds[blogPost.Id],
						})
						if checkedBlogPostIds[blogPost.Id] {
							checkedCount++
						}
					}
					postsCount := len(category.BlogPostIds)

					customCategories = append(customCategories, CustomCategory{
						Name:            category.Name,
						PostsCount:      postsCount,
						CheckedCount:    checkedCount,
						IsChecked:       postsCount == checkedCount,
						IsIndeterminate: 0 < checkedCount && checkedCount < postsCount,
						Posts:           posts,
					})
				}
			}

			var allPosts []Post
			for i, blogPost := range allBlogPosts {
				allPosts = append(allPosts, Post{
					Id:         blogPost.Id,
					Url:        blogPost.Url,
					Title:      blogPost.Title,
					IsEarliest: i == 0,
					IsNewest:   i == len(allBlogPosts)-1,
					IsChecked:  checkedBlogPostIds[blogPost.Id],
				})
			}

			templates.MustWrite(w, "subscriptions/setup_blog_select_posts", CrawledResult{
				Title:                       util.DecorateTitle(subscriptionName),
				Session:                     rutil.Session(r),
				SubscriptionName:            subscriptionName,
				TopCategories:               topCategories,
				MarkWrongFuncJS:             markWrongFuncJS,
				IsCheckedEverything:         len(topCategories) == 1,
				CheckedTopCategoryId:        topCategories[0].Id,
				CheckedTopCategoryName:      topCategories[0].Name,
				CheckedBlogPostIdsCount:     len(checkedBlogPostIds),
				CustomCategories:            customCategories,
				AllPostsCount:               len(allBlogPosts),
				AllPosts:                    allPosts,
				SubscriptionSelectPostsPath: subscriptionSelectPostsPath,
				CustomSubmit: Submit{
					Suffix:                    "custom",
					SubscriptionDeletePath:    subscriptionDeletePath,
					SubscriptionMarkWrongPath: subscrtipionMarkWrongPath,
					MarkWrongFuncJS:           markWrongFuncJS,
				},
			})
			return
		case models.BlogStatusCrawlFailed,
			models.BlogStatusUpdateFromFeedFailed:
			type FailedResult struct {
				Title                  string
				Session                *util.Session
				SubscriptionName       string
				SubscriptionDeletePath string
			}
			templates.MustWrite(w, "subscriptions/setup_blog_failed", FailedResult{
				Title:                  util.DecorateTitle("Blog not supported"),
				Session:                rutil.Session(r),
				SubscriptionName:       subscriptionName,
				SubscriptionDeletePath: rutil.SubscriptionDeletePath(subscriptionId),
			})
			return
		default:
			panic(fmt.Errorf("Unknown blog status: %s", blogStatus))
		}
	case models.SubscriptionStatusSetup:
		otherSubNamesByDay, err := models.Subscription_GetOtherNamesByDay(
			conn, subscriptionId, currentUser.Id,
		)
		if err != nil {
			panic(err)
		}
		userSettings, err := models.UserSettings_Get(conn, currentUser.Id)
		if err != nil {
			panic(err)
		}
		preview := subscriptions_MustGetSchedulePreview(
			conn, subscriptionId, subscriptionStatus, currentUser.Id, userSettings,
		)
		type SetScheduleResult struct {
			Title                    string
			Session                  *util.Session
			NameHeaderId             string
			SubscriptionName         string
			Schedule                 subscriptionsScheduleResult
			SchedulePreview          schedulePreview
			ScheduleJS               subscriptionsScheduleJsResult
			IsDeliveryChannelSet     bool
			DeliveryChannel          deliverySettings
			SubscriptionSchedulePath string
		}
		templates.MustWrite(w, "subscriptions/setup_subscription_set_schedule", SetScheduleResult{
			Title:            util.DecorateTitle(subscriptionName),
			Session:          rutil.Session(r),
			NameHeaderId:     "name_header",
			SubscriptionName: subscriptionName,
			Schedule: subscriptionsScheduleResult{
				Name:               subscriptionName,
				CurrentCountByDay:  make(map[schedule.DayOfWeek]int),
				HasOtherSubs:       len(otherSubNamesByDay) > 0,
				OtherSubNamesByDay: otherSubNamesByDay,
				DaysOfWeek:         schedule.DaysOfWeek,
			},
			SchedulePreview: preview,
			ScheduleJS: subscriptionsScheduleJsResult{
				DaysOfWeekJson:        schedule.DaysOfWeekJson,
				ValidateCallback:      template.JS("onValidateSchedule"),
				SetNameChangeCallback: template.JS("setNameChangeScheduleCallback"),
			},
			IsDeliveryChannelSet:     userSettings.DeliveryChannel != nil,
			DeliveryChannel:          newDeliverySettings(userSettings),
			SubscriptionSchedulePath: rutil.SubscriptionSchedulePath(subscriptionId),
		})
	case models.SubscriptionStatusLive:
		subscriptionName, err := models.Subscription_GetName(conn, subscriptionId)
		if err != nil {
			panic(err)
		}
		feedUrl := rutil.SubscriptionFeedUrl(r, subscriptionId)
		userSettings, err := models.UserSettings_Get(conn, currentUser.Id)
		if err != nil {
			panic(err)
		}

		row := conn.QueryRow(`
			select count(1) from subscription_posts where subscription_id = $1 and published_at is not null
		`, subscriptionId)
		var publishedCount int
		err = row.Scan(&publishedCount)
		if err != nil {
			panic(err)
		}

		var willArriveDate string
		var willArriveOne bool
		if publishedCount == 0 {
			countsByDay, err := models.Schedule_GetCountsByDay(conn, subscriptionId)
			if err != nil {
				panic(err)
			}
			utcNow := schedule.UTCNow()
			location := tzdata.LocationByName[userSettings.Timezone]
			localTime := utcNow.In(location)
			localDate := localTime.Date()

			nextJobScheduleDate, err := subscriptions_GetRealisticScheduleDate(
				conn, currentUser.Id, localTime, localDate,
			)
			if err != nil {
				panic(err)
			}
			todaysJobAlreadyRan := nextJobScheduleDate > localDate
			willArriveDateTime := localDate.Time()
			if todaysJobAlreadyRan {
				willArriveDateTime = willArriveDateTime.AddDate(0, 0, 1)
			}
			for countsByDay[willArriveDateTime.DayOfWeek()] <= 0 {
				willArriveDateTime = willArriveDateTime.AddDate(0, 0, 1)
			}

			willArriveDate = willArriveDateTime.Format("Monday, January 2") +
				util.Ordinal(willArriveDateTime.Day())
			willArriveOne = countsByDay[willArriveDateTime.DayOfWeek()] == 1
		}
		type HeresFeedOrEmailResult struct {
			Title            string
			Session          *util.Session
			SubscriptionName string
			FeedUrl          string
			FeedUrlEncoded   string
			Email            string
			ArrivedOne       bool
			ArrivedMany      bool
			WillArriveOne    bool
			WillArriveDate   string
			SubscriptionPath string
		}
		result := HeresFeedOrEmailResult{
			Title:            util.DecorateTitle(subscriptionName),
			Session:          rutil.Session(r),
			SubscriptionName: subscriptionName,
			FeedUrl:          feedUrl,
			FeedUrlEncoded:   url.QueryEscape(feedUrl),
			Email:            currentUser.Email,
			ArrivedOne:       publishedCount == 1,
			ArrivedMany:      publishedCount > 1,
			WillArriveOne:    willArriveOne,
			WillArriveDate:   willArriveDate,
			SubscriptionPath: rutil.SubscriptionPath(subscriptionId),
		}
		switch *userSettings.DeliveryChannel {
		case models.DeliveryChannelSingleFeed, models.DeliveryChannelMultipleFeeds:
			templates.MustWrite(w, "subscriptions/setup_subscription_heres_feed", result)
		case models.DeliveryChannelEmail:
			templates.MustWrite(w, "subscriptions/setup_subscription_heres_email", result)
		default:
			panic(fmt.Errorf("Unknown delivery channel: %s", *userSettings.DeliveryChannel))
		}

	default:
		panic(fmt.Errorf("Unknown subscription status: %s", subscriptionStatus))
	}
}

func Subscriptions_Progress(w http.ResponseWriter, r *http.Request) {
	conn := rutil.DBConn(r)
	logger := rutil.Logger(r)
	subscriptionIdInt, ok := util.URLParamInt64(r, "id")
	if !ok {
		return
	}
	subscriptionId := models.SubscriptionId(subscriptionIdInt)

	var maybeUserId *models.UserId
	var blogId models.BlogId
	var blogStatus models.BlogStatus
	row := conn.QueryRow(`
		select user_id, blog_id, (
			select status from blogs where blogs.id = subscriptions_without_discarded.blog_id
		)
		from subscriptions_without_discarded
		where id = $1
	`, subscriptionId)
	err := row.Scan(&maybeUserId, &blogId, &blogStatus)
	if err != nil {
		panic(err)
	}
	var userId models.UserId
	if maybeUserId != nil {
		userId = *maybeUserId
	}

	currentUserId := rutil.CurrentUserId(r)
	if currentUserId != userId {
		return
	}

	var progressMap map[string]any
	switch blogStatus {
	case models.BlogStatusCrawlInProgress:
		blogCrawlProgress, err := models.BlogCrawlProgress_Get(conn, blogId)
		if err != nil {
			panic(err)
		}

		logger.Info().Msgf("Blog %d crawl in progress (epoch %d)", blogId, blogCrawlProgress.Epoch)
		progressMap = map[string]any{
			"epoch":  blogCrawlProgress.Epoch,
			"status": blogCrawlProgress.Progress,
			"count":  blogCrawlProgress.Count,
		}
	default:
		logger.Info().Msgf("Blog %d crawl done", blogId)
		progressMap = map[string]any{
			"done": true,
		}
	}

	rutil.MustWriteJson(w, http.StatusOK, progressMap)
}

func Subscriptions_SubmitProgressTimes(w http.ResponseWriter, r *http.Request) {
	logger := rutil.Logger(r)
	conn := rutil.DBConn(r)
	subscriptionIdInt, ok := util.URLParamInt64(r, "id")
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	clientToken := models.BlogCrawlClientToken(util.EnsureParamStr(r, "client_token"))
	epochDurations := util.EnsureParamStr(r, "epoch_durations")
	websocketWaitDuration := util.EnsureParamFloat64(r, "websocket_wait_duration")
	totalReconnectAttempts := util.EnsureParamInt(r, "total_reconnect_attempts")

	subscriptionId := models.SubscriptionId(subscriptionIdInt)
	currentUserId := rutil.CurrentUserId(r)
	var maybeSubscriptionUserId *models.UserId
	var blogFeedUrl string
	var blogCrawlClientToken models.BlogCrawlClientToken
	var blogCrawlEpoch int32
	var blogCrawlEpochTimes string
	row := conn.QueryRow(`
		select user_id, blogs.feed_url, blog_crawl_client_tokens.value, blog_crawl_progresses.epoch,
			blog_crawl_progresses.epoch_times
		from subscriptions_without_discarded
		join blogs on subscriptions_without_discarded.blog_id = blogs.id
		join blog_crawl_client_tokens
			on blog_crawl_client_tokens.blog_id = subscriptions_without_discarded.blog_id
		join blog_crawl_progresses
			on blog_crawl_progresses.blog_id = subscriptions_without_discarded.blog_id
		where subscriptions_without_discarded.id = $1
	`, subscriptionId)
	err := row.Scan(
		&maybeSubscriptionUserId, &blogFeedUrl, &blogCrawlClientToken, &blogCrawlEpoch, &blogCrawlEpochTimes,
	)
	if err != nil {
		panic(err)
	}
	var subscriptionUserId models.UserId
	if maybeSubscriptionUserId != nil {
		subscriptionUserId = *maybeSubscriptionUserId
	}
	if subscriptionUserId != currentUserId {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if clientToken != blogCrawlClientToken {
		logger.Info().Msgf(
			"Client token mismatch: incoming %s, expected %s",
			clientToken, blogCrawlClientToken,
		)
		return
	}

	logger.Info().Msgf("Server: %s", blogCrawlEpochTimes)
	logger.Info().Msgf("Client: %s", epochDurations)
	adminTelemetryExtra := map[string]any{
		"feed_url":        blogFeedUrl,
		"subscription_id": subscriptionId,
	}
	if totalReconnectAttempts > 0 {
		logger.Info().Msgf("Total reconnect attempts: %d", totalReconnectAttempts)
		err := models.AdminTelemetry_Create(
			conn, "websocket_reconnects", float64(totalReconnectAttempts), adminTelemetryExtra,
		)
		if err != nil {
			panic(err)
		}
		return
	}

	var serverDurations []float64
	for _, token := range strings.Split(blogCrawlEpochTimes, ";") {
		duration, err := strconv.ParseFloat(token, 64)
		if err != nil {
			panic(err)
		}
		serverDurations = append(serverDurations, duration)
	}
	var clientDurations []float64
	if len(epochDurations) > 0 {
		for _, token := range strings.Split(epochDurations, ";") {
			duration, err := strconv.ParseFloat(token, 64)
			if err != nil {
				panic(err)
			}
			clientDurations = append(clientDurations, duration)
		}
	}
	if len(clientDurations) != len(serverDurations) {
		logger.Info().Msgf(
			"Epoch count mismatch: client %d, server %d", len(clientDurations), len(serverDurations),
		)
		return
	}

	if len(clientDurations) < 3 {
		logger.Info().Msg("Too few client durations to compute anything")
		return
	}

	calcStdDeviation := func(clientDurations, serverDurations []float64) float64 {
		var result float64
		for i, clientDuration := range clientDurations {
			serverDuration := serverDurations[i]
			result += (clientDuration - serverDuration) * (clientDuration - serverDuration)
		}
		result = math.Sqrt(result / float64(blogCrawlEpoch))
		return result
	}

	stdDeviation := calcStdDeviation(clientDurations, serverDurations)
	logger.Info().Msgf("Standard deviation (full): %.03f", stdDeviation)

	var clientDurationsAfterInitialLoad []float64
	for _, clientDuration := range clientDurations[1 : len(clientDurations)-1] {
		if clientDuration != 0 {
			clientDurationsAfterInitialLoad = append(clientDurationsAfterInitialLoad, clientDuration)
		}
	}
	serverDurationsAfterInitialLoad :=
		serverDurations[len(serverDurations)-len(clientDurationsAfterInitialLoad):]
	stdDeviationAfterInitialLoad :=
		calcStdDeviation(clientDurationsAfterInitialLoad, serverDurationsAfterInitialLoad)
	logger.Info().Msgf("Standard deviation after initial load: %.03f", stdDeviationAfterInitialLoad)
	err = models.AdminTelemetry_Create(
		conn, "progress_timing_std_deviation", stdDeviationAfterInitialLoad, adminTelemetryExtra,
	)
	if err != nil {
		panic(err)
	}

	// E2E for crawling job getting picked up and reporting the first rectangle
	initialLoadDuration := clientDurations[0]
	if initialLoadDuration > 10 {
		logger.Warn().Msgf("Initial load duration (exceeds 10 seconds): %.03f", initialLoadDuration)
	} else {
		logger.Info().Msgf("Initial load duration: %.03f", initialLoadDuration)
	}
	err = models.AdminTelemetry_Create(
		conn, "progress_timing_initial_load", initialLoadDuration, adminTelemetryExtra,
	)
	if err != nil {
		panic(err)
	}

	// Just the establishing websocket part, at the granularity of throttled crawl requests
	realWebsocketWaitDuration := websocketWaitDuration - serverDurations[0]
	if realWebsocketWaitDuration < 0 {
		realWebsocketWaitDuration = 0
	}
	logger.Info().Msgf("Websocket wait duration: %.03f", realWebsocketWaitDuration)
	err = models.AdminTelemetry_Create(
		conn, "websocket_wait_duration", realWebsocketWaitDuration, adminTelemetryExtra,
	)
	if err != nil {
		panic(err)
	}
}

func Subscriptions_SelectPosts(w http.ResponseWriter, r *http.Request) {
	logger := rutil.Logger(r)
	conn := rutil.DBConn(r)
	subscriptionIdInt, ok := util.URLParamInt64(r, "id")
	if !ok {
		subscriptions_RedirectNotFound(w, r)
		return
	}

	subscriptionId := models.SubscriptionId(subscriptionIdInt)
	var subscriptionStatus models.SubscriptionStatus
	var blogStatus models.BlogStatus
	var postsCount int
	var blogId models.BlogId
	var blogBestUrl string
	var maybeSubscriptionUserId *models.UserId
	row := conn.QueryRow(`
		select status, (
			select status from blogs where id = blog_id
		) as blog_status, (
			select count(1)
			from blog_posts
			where blog_posts.blog_id = subscriptions_without_discarded.blog_id
		), blog_id,	(
			select coalesce(url, feed_url) from blogs where id = blog_id
		), user_id
		from subscriptions_without_discarded
		where id = $1
	`, subscriptionId)
	err := row.Scan(
		&subscriptionStatus, &blogStatus, &postsCount, &blogId, &blogBestUrl, &maybeSubscriptionUserId,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		subscriptions_RedirectNotFound(w, r)
		return
	} else if err != nil {
		panic(err)
	}

	if subscriptions_RedirectIfUserMismatch(w, r, maybeSubscriptionUserId) {
		return
	}

	if !(subscriptionStatus == models.SubscriptionStatusWaitingForBlog &&
		(blogStatus == models.BlogStatusCrawledVoting ||
			blogStatus == models.BlogStatusCrawledConfirmed ||
			blogStatus == models.BlogStatusCrawledLooksWrong ||
			blogStatus == models.BlogStatusManuallyInserted)) {
		http.Redirect(w, r, rutil.SubscriptionSetupPath(subscriptionId), http.StatusFound)
		return
	}

	var topCategoryId models.BlogPostCategoryId
	blogPostIds := make(map[models.BlogPostId]bool)
	var productSelectedCount int
	var productSelection string
	if topCategoryIdStr, ok := r.Form["top_category_id"]; ok && topCategoryIdStr[0] != "" {
		topCategoryIdInt, err := strconv.ParseInt(topCategoryIdStr[0], 10, 64)
		if err != nil {
			panic(err)
		}
		topCategoryId = models.BlogPostCategoryId(topCategoryIdInt)
		topCategoryName, postsCount, err := models.BlogPostCategory_GetNamePostsCountById(conn, topCategoryId)
		if err != nil {
			panic(err)
		}
		productSelectedCount = postsCount
		if topCategoryName == "Everything" {
			productSelection = "everything"
		} else {
			productSelection = "top_category"
		}
		logger.Info().Msgf("Using top category %s with %d posts", topCategoryName, postsCount)
	} else {
		for key, value := range r.Form {
			if !strings.HasPrefix(key, "post_") {
				continue
			}
			if value[0] != "1" {
				continue
			}
			blogPostIdInt, err := strconv.ParseInt(key[5:], 10, 64)
			if err != nil {
				panic(err)
			}
			blogPostIds[models.BlogPostId(blogPostIdInt)] = true
		}
		productSelectedCount = len(blogPostIds)
		productSelection = "custom"
		logger.Info().Msgf("Using custom selection with %d posts", len(blogPostIds))
	}

	err = models.BlogCrawlVote_Create(
		conn, blogId, rutil.CurrentUserId(r), models.BlogCrawlVoteConfirmed,
	)
	if err != nil {
		panic(err)
	}

	currentUser := rutil.CurrentUser(r)
	pc := models.NewProductEventContext(conn, r, rutil.CurrentProductUserId(r))
	models.ProductEvent_MustEmitFromRequest(pc, "select posts", map[string]any{
		"subscription_id":   subscriptionId,
		"blog_url":          blogBestUrl,
		"selected_count":    productSelectedCount,
		"selected_fraction": float64(productSelectedCount) / float64(postsCount),
		"selection":         productSelection,
		"user_is_anonymous": currentUser == nil,
	}, nil)

	tx, err := rutil.DBConn(r).Begin()
	if err != nil {
		panic(err)
	}
	defer util.CommitOrRollbackOnPanic(tx)

	if topCategoryId != 0 {
		err := models.Subscription_CreatePostsFromCategory(tx, subscriptionId, topCategoryId)
		if err != nil {
			panic(err)
		}
	} else {
		err := models.Subscription_CreatePostsFromIds(tx, subscriptionId, blogId, blogPostIds)
		if err != nil {
			panic(err)
		}
	}
	err = models.Subscription_UpdateStatus(tx, subscriptionId, models.SubscriptionStatusSetup)
	if err != nil {
		panic(err)
	}

	if currentUser != nil {
		http.Redirect(w, r, rutil.SubscriptionSetupPath(subscriptionId), http.StatusFound)
	} else {
		http.Redirect(w, r, "/signup", http.StatusFound)
	}
}

func Subscriptions_MarkWrong(w http.ResponseWriter, r *http.Request) {
	logger := rutil.Logger(r)
	conn := rutil.DBConn(r)
	subscriptionIdInt, ok := util.URLParamInt64(r, "id")
	if !ok {
		subscriptions_RedirectNotFound(w, r)
		return
	}

	subscriptionId := models.SubscriptionId(subscriptionIdInt)
	var subscriptionStatus models.SubscriptionStatus
	var blogStatus models.BlogStatus
	var blogName string
	var blogBestUrl string
	var blogId models.BlogId
	var maybeSubscriptionUserId *models.UserId
	row := conn.QueryRow(`
		select
			status,
			(select status from blogs where id = blog_id) as blog_status,
			(select name from blogs where id = blog_id) as blog_name,
			(select coalesce(url, feed_url) from blogs where id = blog_id),
			blog_id,
			user_id
		from subscriptions_without_discarded
		where id = $1
	`, subscriptionId)
	err := row.Scan(
		&subscriptionStatus, &blogStatus, &blogName, &blogBestUrl, &blogId, &maybeSubscriptionUserId,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		subscriptions_RedirectNotFound(w, r)
		return
	} else if err != nil {
		panic(err)
	}

	if subscriptions_RedirectIfUserMismatch(w, r, maybeSubscriptionUserId) {
		return
	}

	if !(subscriptionStatus == models.SubscriptionStatusWaitingForBlog &&
		(blogStatus == models.BlogStatusCrawledVoting ||
			blogStatus == models.BlogStatusCrawledConfirmed ||
			blogStatus == models.BlogStatusCrawledLooksWrong ||
			blogStatus == models.BlogStatusManuallyInserted)) {
		http.Redirect(w, r, rutil.SubscriptionSetupPath(subscriptionId), http.StatusFound)
		return
	}

	err = models.BlogCrawlVote_Create(
		conn, blogId, rutil.CurrentUserId(r), models.BlogCrawlVoteLooksWrong,
	)
	if err != nil {
		panic(err)
	}

	pc := models.NewProductEventContext(conn, r, rutil.CurrentProductUserId(r))
	models.ProductEvent_MustEmitFromRequest(pc, "mark wrong", map[string]any{
		"subscription_id":   subscriptionId,
		"blog_url":          blogBestUrl,
		"user_is_anonymous": rutil.CurrentUser(r) == nil,
	}, nil)

	logger.Warn().Msgf("Blog %d (%s) marked as wrong", blogId, blogName)
}

func Subscriptions_Schedule(w http.ResponseWriter, r *http.Request) {
	logger := rutil.Logger(r)
	conn := rutil.DBConn(r)
	subscriptionIdInt, ok := util.URLParamInt64(r, "id")
	if !ok {
		subscriptions_RedirectNotFound(w, r)
		return
	}
	subscriptionId := models.SubscriptionId(subscriptionIdInt)
	var subscriptionStatus models.SubscriptionStatus
	var blogStatus models.BlogStatus
	var blogName string
	var blogBestUrl string
	var blogId models.BlogId
	var maybeSubscriptionUserId *models.UserId
	row := conn.QueryRow(`
		select
			status,
			(select status from blogs where id = blog_id) as blog_status,
			(select name from blogs where id = blog_id) as blog_name,
			(select coalesce(url, feed_url) from blogs where id = blog_id),
			blog_id,
			user_id
		from subscriptions_without_discarded
		where id = $1
	`, subscriptionId)
	err := row.Scan(
		&subscriptionStatus, &blogStatus, &blogName, &blogBestUrl, &blogId, &maybeSubscriptionUserId,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		subscriptions_RedirectNotFound(w, r)
		return
	} else if err != nil {
		panic(err)
	}
	if subscriptions_RedirectIfUserMismatch(w, r, maybeSubscriptionUserId) {
		return
	}
	if subscriptionStatus != models.SubscriptionStatusSetup {
		return
	}

	subscriptionName := util.EnsureParamStr(r, "name")

	countsByDay := make(map[schedule.DayOfWeek]int)
	totalCount := 0
	for _, dayOfWeek := range schedule.DaysOfWeek {
		count := util.EnsureParamInt(r, string(dayOfWeek)+"_count")
		if count < 0 {
			panic("Expecting counts to be 0+")
		}
		countsByDay[dayOfWeek] = count
		totalCount += count
	}
	if totalCount <= 0 {
		panic("Expecting some count to not be zero")
	}

	deliveryChannelParam := r.Form.Get("delivery_channel")
	currentUser := rutil.CurrentUser(r)

	// Initializing subscription feed may race with user's update rss job.
	// If the job is already running, wait till it finishes, otherwise lock the row so it doesn't start
	mustSaveSchedule := func() (result bool) {
		tx, err := conn.Begin()
		if err != nil {
			panic(err)
		}
		defer util.CommitOrRollbackMsg(tx, &result, "Unlocked daily jobs")

		logger.Info().Msg("Locking daily jobs")
		lockedJobs, err := jobs.PublishPostsJob_Lock(tx, currentUser.Id)
		if err != nil {
			panic(err)
		}
		logger.Info().Msgf("Locked daily jobs %d", len(lockedJobs))

		for _, job := range lockedJobs {
			if job.LockedBy != "" {
				logger.Info().Msgf("Some jobs are running, unlocking %d", len(lockedJobs))
				return false
			}
		}

		oldUserSettings, err := models.UserSettings_Get(tx, currentUser.Id)
		if err != nil {
			panic(err)
		}

		pc := models.NewProductEventContext(tx, r, rutil.CurrentProductUserId(r))
		var deliveryChannel models.DeliveryChannel
		if deliveryChannelParam != "" {
			switch deliveryChannelParam {
			case "rss":
				deliveryChannel = models.DeliveryChannelMultipleFeeds
			case "email":
				deliveryChannel = models.DeliveryChannelEmail
			default:
				panic(fmt.Errorf("unknown delivery channel: %s", deliveryChannelParam))
			}
			err := models.UserSettings_SaveDeliveryChannel(tx, currentUser.Id, deliveryChannel)
			if err != nil {
				panic(err)
			}
			newUserSettings, err := models.UserSettings_Get(tx, currentUser.Id)
			if err != nil {
				panic(err)
			}
			err = jobs.PublishPostsJob_ScheduleInitial(tx, currentUser.Id, newUserSettings, false)
			if err != nil {
				panic(err)
			}
			models.ProductEvent_MustEmitFromRequest(pc, "pick delivery channel", map[string]any{
				"channel": newUserSettings.DeliveryChannel,
			}, map[string]any{
				"delivery_channel": newUserSettings.DeliveryChannel,
			})
		} else if oldUserSettings.DeliveryChannel == nil {
			panic("Delivery channel is not set for the user and is not passed in the params")
		} else {
			deliveryChannel = *oldUserSettings.DeliveryChannel
		}

		err = models.Schedule_Create(tx, subscriptionId, countsByDay)
		if err != nil {
			panic(err)
		}

		utcNow := schedule.UTCNow()
		location := tzdata.LocationByName[oldUserSettings.Timezone]
		localTime := utcNow.In(location)
		localDate := localTime.Date()

		// If subscription got added early morning, the first post needs to go out the same day, either via
		// the daily job or right away if the update rss job has already ran
		nextJobDate, err := jobs.PublishPostsJob_GetNextScheduledDate(tx, currentUser.Id)
		if err != nil {
			panic(err)
		}
		todaysJobAlreadyRan := nextJobDate > localDate
		isAddedEarlyMorning := localTime.IsEarlyMorning()
		shouldPublishRssPosts := todaysJobAlreadyRan && isAddedEarlyMorning

		err = models.Subscription_FinishSetup(
			tx, subscriptionId, subscriptionName, models.SubscriptionStatusLive, utcNow, 1,
			isAddedEarlyMorning,
		)
		if err != nil {
			panic(err)
		}

		err = publish.InitSubscription(
			tx, currentUser.Id, currentUser.ProductUserId, subscriptionId, subscriptionName, blogBestUrl,
			deliveryChannel, shouldPublishRssPosts, utcNow, localTime, localDate,
		)
		if err != nil {
			panic(err)
		}

		productActiveDays := 0
		for _, count := range countsByDay {
			if count > 0 {
				productActiveDays++
			}
		}
		models.ProductEvent_MustEmitSchedule(
			pc, "schedule", subscriptionId, blogBestUrl, totalCount, productActiveDays,
		)

		slackEmail := jobs.NotifySlackJob_Escape(currentUser.Email)
		slackBlogUrl := jobs.NotifySlackJob_Escape(blogBestUrl)
		slackBlogName := jobs.NotifySlackJob_Escape(blogName)
		err = jobs.NotifySlackJob_PerformNow(
			tx, fmt.Sprintf("*%s* subscribed to *<%s|%s>*", slackEmail, slackBlogUrl, slackBlogName),
		)
		if err != nil {
			logger.Error().Err(err).Msg("Error while submitting a NotifySlackJob")
		}

		util.MustWrite(w, rutil.SubscriptionSetupPath(subscriptionId))
		return true
	}

	failedLockAttempts := 0
	for {
		if failedLockAttempts >= 3 {
			panic("Couldn't lock the job rows")
		} else if failedLockAttempts > 0 {
			time.Sleep(time.Second)
		}

		if mustSaveSchedule() {
			w.WriteHeader(http.StatusOK)
			break
		} else {
			failedLockAttempts++
		}
	}

}

func Subscriptions_Pause(w http.ResponseWriter, r *http.Request) {
	subscriptions_MustPauseUnpause(w, r, true, "pause subscription")
}

func Subscriptions_Unpause(w http.ResponseWriter, r *http.Request) {
	subscriptions_MustPauseUnpause(w, r, false, "unpause subscription")
}

func subscriptions_MustPauseUnpause(
	w http.ResponseWriter, r *http.Request, newIsPaused bool, eventName string,
) {
	subscriptionIdInt, ok := util.URLParamInt64(r, "id")
	if !ok {
		subscriptions_RedirectNotFound(w, r)
		return
	}

	subscriptionId := models.SubscriptionId(subscriptionIdInt)
	conn := rutil.DBConn(r)
	var maybeSubscriptionUserId *models.UserId
	var status models.SubscriptionStatus
	var blogBestUrl string
	row := conn.QueryRow(`
		select user_id, status, (
			select coalesce(url, feed_url) from blogs
			where blogs.id = subscriptions_without_discarded.blog_id
		) from subscriptions_without_discarded where id = $1
	`, subscriptionId)
	err := row.Scan(&maybeSubscriptionUserId, &status, &blogBestUrl)
	if errors.Is(err, pgx.ErrNoRows) {
		subscriptions_RedirectNotFound(w, r)
	} else if err != nil {
		panic(err)
	}

	if subscriptions_RedirectIfUserMismatch(w, r, maybeSubscriptionUserId) {
		return
	}

	if subscriptions_BadRequestIfNotLive(w, status) {
		return
	}

	err = models.Subscription_SetIsPaused(conn, subscriptionId, newIsPaused)
	if err != nil {
		panic(err)
	}

	pc := models.NewProductEventContext(conn, r, rutil.CurrentProductUserId(r))
	models.ProductEvent_MustEmitFromRequest(pc, eventName, map[string]any{
		"subscription_id": subscriptionId,
		"blog_url":        blogBestUrl,
	}, nil)
	w.WriteHeader(http.StatusOK)
}

var dayCountNames []string

func init() {
	for _, day := range schedule.DaysOfWeek {
		dayCountNames = append(dayCountNames, string(day)+"_count")
	}
}

func Subscriptions_Update(w http.ResponseWriter, r *http.Request) {
	tx, err := rutil.DBConn(r).Begin()
	if err != nil {
		panic(err)
	}
	defer util.CommitOrRollbackOnPanic(tx)

	subscriptionIdInt, ok := util.URLParamInt64(r, "id")
	if !ok {
		subscriptions_RedirectNotFound(w, r)
		return
	}

	subscriptionId := models.SubscriptionId(subscriptionIdInt)
	var maybeSubscriptionUserId *models.UserId
	var status models.SubscriptionStatus
	var scheduleVersion int64
	var blogBestUrl string
	row := tx.QueryRow(`
		select user_id, status, schedule_version, (
			select coalesce(url, feed_url) from blogs
			where blogs.id = subscriptions_without_discarded.blog_id
		) from subscriptions_without_discarded where id = $1
	`, subscriptionId)
	err = row.Scan(&maybeSubscriptionUserId, &status, &scheduleVersion, &blogBestUrl)
	if errors.Is(err, pgx.ErrNoRows) {
		subscriptions_RedirectNotFound(w, r)
	} else if err != nil {
		panic(err)
	}

	if subscriptions_RedirectIfUserMismatch(w, r, maybeSubscriptionUserId) {
		return
	}

	if subscriptions_BadRequestIfNotLive(w, status) {
		return
	}

	newVersion := util.EnsureParamInt64(r, "schedule_version")
	if scheduleVersion >= newVersion {
		rutil.MustWriteJson(w, http.StatusConflict, map[string]any{
			"schedule_version": scheduleVersion,
		})
		return
	}

	var totalCount int64
	for _, dayCountName := range dayCountNames {
		dayCount := util.EnsureParamInt64(r, dayCountName)
		totalCount += dayCount
	}
	if totalCount == 0 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	productActiveDays := 0
	countsByDay := make(map[schedule.DayOfWeek]int)
	for _, dayCountName := range dayCountNames {
		dayOfWeek := schedule.DayOfWeek(dayCountName[:3])
		dayCount := util.EnsureParamInt64(r, dayCountName)
		countsByDay[dayOfWeek] = int(dayCount)
		if dayCount > 0 {
			productActiveDays++
		}
	}
	err = models.Schedule_Update(tx, subscriptionId, countsByDay)
	if err != nil {
		panic(err)
	}
	err = models.Subscription_UpdateScheduleVersion(tx, subscriptionId, newVersion)
	if err != nil {
		panic(err)
	}

	pc := models.NewProductEventContext(tx, r, rutil.CurrentProductUserId(r))
	models.ProductEvent_MustEmitSchedule(
		pc, "update schedule", subscriptionId, blogBestUrl, int(totalCount), productActiveDays,
	)
}

func Subscriptions_Delete(w http.ResponseWriter, r *http.Request) {
	conn := rutil.DBConn(r)
	subscriptionIdInt, ok := util.URLParamInt64(r, "id")
	if !ok {
		subscriptions_RedirectNotFound(w, r)
		return
	}

	subscriptionId := models.SubscriptionId(subscriptionIdInt)
	var maybeSubscriptionUserId *models.UserId
	var blogBestUrl string
	row := conn.QueryRow(`
		select user_id, (
			select coalesce(url, feed_url) from blogs
			where blogs.id = subscriptions_without_discarded.blog_id
		) from subscriptions_without_discarded where id = $1
	`, subscriptionId)
	err := row.Scan(&maybeSubscriptionUserId, &blogBestUrl)
	if errors.Is(err, pgx.ErrNoRows) {
		subscriptions_RedirectNotFound(w, r)
	} else if err != nil {
		panic(err)
	}

	if subscriptions_RedirectIfUserMismatch(w, r, maybeSubscriptionUserId) {
		return
	}

	err = models.Subscription_Delete(conn, subscriptionId)
	if err != nil {
		panic(err)
	}

	pc := models.NewProductEventContext(conn, r, rutil.CurrentProductUserId(r))
	models.ProductEvent_MustEmitFromRequest(pc, "delete subscription", map[string]any{
		"subscription_id": subscriptionId,
		"blog_url":        blogBestUrl,
	}, nil)

	if redirect, ok := r.Form["redirect"]; ok && redirect[0] == "add" {
		http.Redirect(w, r, "/subscriptions/add", http.StatusFound)
	} else {
		subscriptions_RedirectNotFound(w, r)
	}
}

type schedulePreview struct {
	PrevPosts                            []models.SchedulePreviewPrevPost
	PrevPostDatesJS                      template.JS
	NextPosts                            []models.SchedulePreviewNextPost
	PrevHasMore                          bool
	NextHasMore                          bool
	TodayDate                            schedule.Date
	NextScheduleDate                     schedule.Date
	Timezone                             string
	Location                             *time.Location
	ShortFriendlySuffixNameByGroupIdJson template.JS
	GroupIdByTimezoneIdJson              template.JS
}

func subscriptions_MustGetSchedulePreview(
	tx pgw.Queryable, subscriptionId models.SubscriptionId, subscriptionStatus models.SubscriptionStatus,
	userId models.UserId, userSettings *models.UserSettings,
) schedulePreview {
	logger := tx.Logger()
	preview, err := models.Subscription_GetSchedulePreview(tx, subscriptionId)
	if err != nil {
		panic(err)
	}
	var datesBuf bytes.Buffer
	datesBuf.WriteString("[")
	for i, prevPost := range preview.PrevPosts {
		if i > 0 {
			datesBuf.WriteString(", ")
		}
		datesBuf.WriteString(`new Date("`)
		datesBuf.WriteString(string(prevPost.PublishDate))
		datesBuf.WriteString(`")`)
	}
	datesBuf.WriteString("]")
	prevPostDatesJS := template.JS(datesBuf.String())

	location, ok := tzdata.LocationByName[userSettings.Timezone]
	if !ok {
		panic(fmt.Errorf("Timezone not found: %s", userSettings.Timezone))
	}
	utcNow := schedule.UTCNow()
	localTime := utcNow.In(location)
	localDate := localTime.Date()

	var nextScheduleDate schedule.Date
	if subscriptionStatus != models.SubscriptionStatusLive && localTime.IsEarlyMorning() {
		nextScheduleDate = localDate
	} else {
		var err error
		nextScheduleDate, err = subscriptions_GetRealisticScheduleDate(tx, userId, localTime, localDate)
		if err != nil {
			panic(err)
		}
	}
	logger.Info().Msgf("Preview next schedule date: %s", nextScheduleDate)

	return schedulePreview{
		PrevPosts:                            preview.PrevPosts,
		PrevPostDatesJS:                      prevPostDatesJS,
		NextPosts:                            preview.NextPosts,
		PrevHasMore:                          preview.PrevHasMore,
		NextHasMore:                          preview.NextHasMore,
		TodayDate:                            localDate,
		NextScheduleDate:                     nextScheduleDate,
		Timezone:                             userSettings.Timezone,
		Location:                             location,
		ShortFriendlySuffixNameByGroupIdJson: util.ShortFriendlySuffixNameByGroupIdJson,
		GroupIdByTimezoneIdJson:              util.GroupIdByTimezoneIdJson,
	}
}

func subscriptions_GetRealisticScheduleDate(
	tx pgw.Queryable, userId models.UserId, localTime schedule.Time, localDate schedule.Date,
) (schedule.Date, error) {
	logger := tx.Logger()
	nextScheduleDate, err := jobs.PublishPostsJob_GetNextScheduledDate(tx, userId)
	if err != nil {
		return "", err
	}
	if nextScheduleDate == "" {
		if localTime.IsEarlyMorning() {
			return localDate, nil
		} else {
			nextDay := localTime.AddDate(0, 0, 1)
			return nextDay.Date(), nil
		}
	} else if nextScheduleDate < localDate {
		logger.Warn().Msgf("Job is scheduled in the past for user %d: %s (today is %s)", userId, nextScheduleDate, localDate)
		return localDate, nil
	} else {
		return nextScheduleDate, nil
	}
}

func Subscriptions_ProgressStream(w http.ResponseWriter, r *http.Request) {
	logger := rutil.Logger(r)
	conn := rutil.DBConn(r)
	subscriptionIdInt, ok := util.URLParamInt64(r, "id")
	if !ok {
		subscriptions_RedirectNotFound(w, r)
		return
	}

	subscriptionId := models.SubscriptionId(subscriptionIdInt)
	var maybeSubscriptionUserId *models.UserId
	var blogId models.BlogId
	var blogStatus models.BlogStatus
	row := conn.QueryRow(`
		select user_id, blog_id, (select status from blogs where id = blog_id) as blog_status 
		from subscriptions_without_discarded
		where id = $1
	`, subscriptionId)
	err := row.Scan(&maybeSubscriptionUserId, &blogId, &blogStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		subscriptions_RedirectNotFound(w, r)
		return
	} else if err != nil {
		panic(err)
	}

	if subscriptions_RedirectIfUserMismatch(w, r, maybeSubscriptionUserId) {
		return
	}

	var upgrader = websocket.Upgrader{} // nolint:exhaustruct
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		panic(err)
	}
	defer ws.Close()

	if blogStatus != models.BlogStatusCrawlInProgress {
		err := ws.WriteJSON(map[string]any{"done": true})
		if err != nil {
			panic(err)
		}
		return
	}

	listenConn, err := db.Pool.Acquire(r.Context(), logger)
	if err != nil {
		panic(err)
	}
	defer listenConn.Release()

	channelName := jobs.DiscoveryChannelName(blogId)
	_, err = listenConn.Exec("listen " + channelName)
	if err != nil {
		panic(err)
	}
	logger.Info().Msgf("Started listen on %s", channelName)

	// Guard against a race condition where the last NOTIFY happened before we
	// started listening
	var blogStatusRefresh models.BlogStatus
	row = conn.QueryRow(`
		select (select status from blogs where id = blog_id) as blog_status 
		from subscriptions_without_discarded
		where id = $1
	`, subscriptionId)
	err = row.Scan(&blogStatusRefresh)
	if err != nil {
		panic(err)
	}
	if blogStatusRefresh != models.BlogStatusCrawlInProgress {
		logger.Info().Msgf("Blog %d finished crawling before a notification was received", blogId)
		err := ws.WriteJSON(map[string]any{"done": true})
		if err != nil {
			panic(err)
		}
		return
	}

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	notificationChan := make(chan *pgconn.Notification)
	errorChan := make(chan error)
	go func() {
		for {
			notification, err := listenConn.WaitForNotification()
			if err != nil {
				errorChan <- err
				break
			}
			notificationChan <- notification
		}
	}()

	for {
		select {
		case err := <-errorChan:
			panic(err)
		case notification := <-notificationChan:
			var payload map[string]any
			err := json.Unmarshal([]byte(notification.Payload), &payload)
			if err != nil {
				panic(err)
			}
			logger.Info().Msgf("%s: %s", channelName, notification.Payload)
			err = ws.WriteMessage(websocket.TextMessage, []byte(notification.Payload))
			if err != nil {
				panic(err)
			}
			if payload["done"] == true {
				return
			}
		case <-ticker.C:
			payload, err := json.Marshal(map[string]any{
				"type":    "ping",
				"message": time.Now().Unix(),
			})
			if err != nil {
				panic(err)
			}
			logger.Info().Msgf("%s: %s", channelName, payload)
			err = ws.WriteMessage(websocket.TextMessage, []byte(payload))
			if err != nil {
				panic(err)
			}
		}
	}
}

func subscriptions_RedirectNotFound(w http.ResponseWriter, r *http.Request) {
	path := "/"
	if rutil.CurrentUser(r) != nil {
		path = "/subscriptions"
	}
	http.Redirect(w, r, path, http.StatusFound)
}

func subscriptions_RedirectIfUserMismatch(
	w http.ResponseWriter, r *http.Request, subscriptionUserId *models.UserId,
) bool {
	if subscriptionUserId != nil {
		currentUser := rutil.CurrentUser(r)
		if currentUser == nil {
			http.Redirect(w, r, util.LoginPathWithRedirect(r), http.StatusFound)
			return true
		} else if *subscriptionUserId != currentUser.Id {
			http.Redirect(w, r, "/subscriptions", http.StatusFound)
			return true
		}
	}

	return false
}

func subscriptions_BadRequestIfNotLive(w http.ResponseWriter, status models.SubscriptionStatus) bool {
	if status != models.SubscriptionStatusLive {
		w.WriteHeader(http.StatusBadRequest)
		return true
	}

	return false
}
