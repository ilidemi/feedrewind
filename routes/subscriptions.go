package routes

import (
	"bytes"
	"feedrewind/crawler"
	"feedrewind/db/pgw"
	"feedrewind/jobs"
	"feedrewind/log"
	"feedrewind/models"
	"feedrewind/routes/rutil"
	"feedrewind/templates"
	"feedrewind/third_party/tzdata"
	"feedrewind/util"
	"fmt"
	"html/template"
	"net/http"
	"time"

	"github.com/pkg/errors"
)

func Subscriptions_Index(w http.ResponseWriter, r *http.Request) {
	conn := rutil.DBConn(r)
	currentUser := rutil.CurrentUser(r)
	subscriptions, err := models.Subscription_ListWithPostCounts(conn, currentUser.Id)
	if err != nil {
		panic(err)
	}

	var settingUpSubscriptions []models.SubscriptionWithPostCounts
	var activeSubscriptions []models.SubscriptionWithPostCounts
	var finishedSubscriptions []models.SubscriptionWithPostCounts
	for _, subscription := range subscriptions {
		if subscription.Status != models.SubscriptionStatusLive {
			settingUpSubscriptions = append(settingUpSubscriptions, subscription)
		} else if subscription.PublishedCount < subscription.TotalCount {
			activeSubscriptions = append(activeSubscriptions, subscription)
		} else {
			finishedSubscriptions = append(finishedSubscriptions, subscription)
		}
	}

	type dashboardSubscription struct {
		Id             models.SubscriptionId
		Name           string
		IsPaused       bool
		SetupPath      string
		DeletePath     string
		ShowPath       string
		PublishedCount int
		TotalCount     int
	}

	createDashboardSubscription := func(s models.SubscriptionWithPostCounts) dashboardSubscription {
		return dashboardSubscription{
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

	type dashboardResult struct {
		Session                *util.Session
		HasSubscriptions       bool
		SettingUpSubscriptions []dashboardSubscription
		ActiveSubscriptions    []dashboardSubscription
		FinishedSubscriptions  []dashboardSubscription
	}

	result := dashboardResult{
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

func Subscriptions_Create(w http.ResponseWriter, r *http.Request) {
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
	crawlCtx := &crawler.CrawlContext{}
	httpClient := &crawler.HttpClient{EnableThrottling: false}
	logger := &crawler.ZeroLogger{}
	fetchFeedResult := crawler.FetchFeedAtUrl(startFeed.Url, true, crawlCtx, httpClient, logger)
	switch fetchResult := fetchFeedResult.(type) {
	case *crawler.FetchedPage:
		finalUrl := fetchResult.Page.FetchUri.String()
		parsedFeed, err := crawler.ParseFeed(fetchResult.Page.Content, fetchResult.Page.FetchUri, logger)
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
			_, err := w.Write([]byte(rutil.BlogUnsupportedPath(updatedBlog.Id)))
			if err != nil {
				panic(err)
			}
			return
		} else if err != nil {
			panic(err)
		}

		models.ProductEvent_MustEmitDiscoverFeeds(
			pc, startFeed.Url, models.TypedBlogUrlResultFeed, userIsAnonymous,
		)
		models.ProductEvent_MustEmitCreateSubscription(pc, subscriptionCreateResult, userIsAnonymous)
		_, err = w.Write([]byte(rutil.SubscriptionSetupPath(subscriptionCreateResult.Id)))
		if err != nil {
			panic(err)
		}
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

type subscriptionsScheduleResult struct {
	Name               string
	CurrentCountByDay  map[util.DayOfWeek]int
	HasOtherSubs       bool
	OtherSubNamesByDay map[util.DayOfWeek][]string
	DaysOfWeek         []util.DayOfWeek
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
	subscription, err := models.Subscription_GetWithPostCounts(conn, subscriptionId, currentUser.Id)
	if errors.Is(err, models.ErrSubscriptionNotFound) {
		subscriptions_RedirectNotFound(w, r)
		return
	} else if err != nil {
		panic(err)
	}

	if subscription.Status != models.SubscriptionStatusLive {
		http.Redirect(w, r, rutil.SubscriptionSetupPath(subscriptionId), http.StatusFound)
		return
	}

	userSettings, err := models.UserSettings_GetById(conn, currentUser.Id)
	if err != nil {
		panic(err)
	}
	feedUrl := ""
	if *userSettings.DeliveryChannel != models.DeliveryChannelEmail {
		feedUrl = rutil.SubscriptionFeedUrl(r, subscriptionId)
	}
	countByDay, err := models.Schedule_GetCounts(conn, subscriptionId)
	if err != nil {
		panic(err)
	}
	otherSubNamesByDay, err := models.Subscription_GetOtherNamesByDay(conn, subscriptionId, currentUser.Id)
	if err != nil {
		panic(err)
	}
	preview := subscriptions_MustGetSchedulePreview(
		conn, subscriptionId, subscription.Status, currentUser.Id, userSettings,
	)

	type subscriptionsShowResult struct {
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
	result := subscriptionsShowResult{
		Session:        rutil.Session(r),
		Name:           subscription.Name,
		FeedUrl:        feedUrl,
		IsDone:         subscription.PublishedCount >= subscription.TotalCount,
		IsPaused:       subscription.IsPaused,
		PublishedCount: subscription.PublishedCount,
		TotalCount:     subscription.TotalCount,
		Url:            subscription.Url,
		PausePath:      rutil.SubscriptionPausePath(subscriptionId),
		UnpausePath:    rutil.SubscriptionUnpausePath(subscriptionId),
		Schedule: subscriptionsScheduleResult{
			Name:               subscription.Name,
			CurrentCountByDay:  countByDay,
			HasOtherSubs:       len(otherSubNamesByDay) > 0,
			OtherSubNamesByDay: otherSubNamesByDay,
			DaysOfWeek:         util.DaysOfWeek,
		},
		ScheduleVersion: subscription.ScheduleVersion,
		SchedulePreview: preview,
		ScheduleJS: subscriptionsScheduleJsResult{
			DaysOfWeekJson:        util.DaysOfWeekJson,
			ValidateCallback:      template.JS("onValidateSchedule"),
			SetNameChangeCallback: template.JS("setNameChangeScheduleCallback"),
		},
		DeletePath: rutil.SubscriptionDeletePath(subscriptionId),
	}

	templates.MustWrite(w, "subscriptions/show", result)
}

func Subscriptions_Setup(w http.ResponseWriter, r *http.Request) {
	_, err := w.Write([]byte("setup"))
	if err != nil {
		panic(err)
	}
}

func Subscriptions_Pause(w http.ResponseWriter, r *http.Request) {
	subscriptions_MustPauseUnpause(w, r, true, "pause subscription")
}

func Subscriptions_Unpause(w http.ResponseWriter, r *http.Request) {
	subscriptions_MustPauseUnpause(w, r, false, "unpause subscription")
}

var dayCountNames []string

func init() {
	for _, day := range util.DaysOfWeek {
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
	subscription, err := models.Subscription_GetUserIdStatusScheduleVersion(tx, subscriptionId)
	if errors.Is(err, models.ErrSubscriptionNotFound) {
		subscriptions_RedirectNotFound(w, r)
		return
	} else if err != nil {
		panic(err)
	}

	if subscriptions_RedirectIfUserMismatch(w, r, subscription.UserId) {
		return
	}

	if subscriptions_BadRequestIfNotLive(w, subscription.Status) {
		return
	}

	newVersion := util.EnsureParamInt64(r, "schedule_version")
	if subscription.ScheduleVersion >= newVersion {
		rutil.MustWriteJson(w, http.StatusConflict, map[string]any{
			"schedule_version": subscription.ScheduleVersion,
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
	countsByDay := make(map[util.DayOfWeek]int)
	for _, dayCountName := range dayCountNames {
		dayOfWeek := util.DayOfWeek(dayCountName[:3])
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
		pc, "update schedule", subscriptionId, subscription.BlogBestUrl, int(totalCount), productActiveDays,
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
	subscription, err := models.Subscription_GetUserIdStatusBlogBestUrl(conn, subscriptionId)
	if errors.Is(err, models.ErrSubscriptionNotFound) {
		subscriptions_RedirectNotFound(w, r)
		return
	} else if err != nil {
		panic(err)
	}

	if subscriptions_RedirectIfUserMismatch(w, r, subscription.UserId) {
		return
	}

	err = models.Subscription_Delete(conn, subscriptionId)
	if err != nil {
		panic(err)
	}

	pc := models.NewProductEventContext(conn, r, rutil.CurrentProductUserId(r))
	models.ProductEvent_MustEmitFromRequest(pc, "delete subscription", map[string]any{
		"subscription_id": subscriptionId,
		"blog_url":        subscription.BlogBestUrl,
	}, nil)

	if redirect, ok := r.Form["redirect"]; ok && redirect[0] == "add" {
		http.Redirect(w, r, "/subscriptions/add", http.StatusFound)
	} else {
		subscriptions_RedirectNotFound(w, r)
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

type schedulePreview struct {
	PrevPosts                            []models.SchedulePreviewPrevPost
	PrevPostDatesJS                      template.JS
	NextPosts                            []models.SchedulePreviewNextPost
	PrevHasMore                          bool
	NextHasMore                          bool
	TodayDate                            util.Date
	NextScheduleDate                     util.Date
	Timezone                             string
	Location                             *time.Location
	ShortFriendlySuffixNameByGroupIdJson template.JS
	GroupIdByTimezoneIdJson              template.JS
}

func subscriptions_MustGetSchedulePreview(
	tx pgw.Queryable, subscriptionId models.SubscriptionId, subscriptionStatus models.SubscriptionStatus,
	userId models.UserId, userSettings *models.UserSettings,
) schedulePreview {
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
	utcNow := time.Now().UTC()
	localTime := utcNow.In(location)
	localDate := util.Schedule_Date(localTime)

	var nextScheduleDate util.Date
	if subscriptionStatus != models.SubscriptionStatusLive && util.Schedule_IsEarlyMorning(localTime) {
		nextScheduleDate = localDate
	} else {
		var err error
		nextScheduleDate, err = subscriptions_GetRealisticScheduleDate(tx, userId, localTime, localDate)
		if err != nil {
			panic(err)
		}
	}
	log.Info().Msgf("Preview next schedule date: %s", nextScheduleDate)

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
	tx pgw.Queryable, userId models.UserId, localTime time.Time, localDate util.Date,
) (util.Date, error) {
	nextScheduleDate, err := jobs.PublishPostsJob_GetNextScheduledDate(tx, userId)
	if err != nil {
		return "", err
	}
	if nextScheduleDate == "" {
		if util.Schedule_IsEarlyMorning(localTime) {
			return localDate, nil
		} else {
			nextDay := localTime.AddDate(0, 0, 1)
			return util.Schedule_Date(nextDay), nil
		}
	} else if nextScheduleDate < localDate {
		log.Warn().Msgf("Job is scheduled in the past for user %d: %s (today is %s)", userId, nextScheduleDate, localDate)
		return localDate, nil
	} else {
		return nextScheduleDate, nil
	}
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
	subscription, err := models.Subscription_GetUserIdStatusBlogBestUrl(conn, subscriptionId)
	if errors.Is(err, models.ErrSubscriptionNotFound) {
		subscriptions_RedirectNotFound(w, r)
		return
	} else if err != nil {
		panic(err)
	}

	if subscriptions_RedirectIfUserMismatch(w, r, subscription.UserId) {
		return
	}

	if subscriptions_BadRequestIfNotLive(w, subscription.Status) {
		return
	}

	err = models.Subscription_SetIsPaused(conn, subscriptionId, newIsPaused)
	if err != nil {
		panic(err)
	}

	pc := models.NewProductEventContext(conn, r, rutil.CurrentProductUserId(r))
	models.ProductEvent_MustEmitFromRequest(pc, eventName, map[string]any{
		"subscription_id": subscriptionId,
		"blog_url":        subscription.BlogBestUrl,
	}, nil)
	w.WriteHeader(http.StatusOK)
}
