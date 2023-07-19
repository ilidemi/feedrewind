package routes

import (
	"bytes"
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
)

func Subscriptions_Index(w http.ResponseWriter, r *http.Request) {
	conn := rutil.DBConn(r)
	currentUser := rutil.CurrentUser(r)
	subscriptions := models.Subscription_MustListWithPostCounts(conn, currentUser.Id)

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
	subscription, ok := models.Subscription_MustGetWithPostCounts(conn, subscriptionId, currentUser.Id)
	if !ok {
		subscriptions_RedirectNotFound(w, r)
		return
	}

	if subscription.Status != models.SubscriptionStatusLive {
		http.Redirect(w, r, rutil.SubscriptionSetupPath(subscriptionId), http.StatusFound)
		return
	}

	userSettings := models.UserSettings_MustGetById(conn, currentUser.Id)
	feedUrl := ""
	if *userSettings.DeliveryChannel != models.DeliveryChannelEmail {
		feedUrl = rutil.SubscriptionFeedUrl(r, subscriptionId)
	}
	countByDay := models.Schedule_MustGetCounts(conn, subscriptionId)
	otherSubNamesByDay := models.Subscription_MustGetOtherNamesByDay(conn, subscriptionId, currentUser.Id)
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
	subscriptions_PauseUnpause(w, r, true, "pause subscription")
}

func Subscriptions_Unpause(w http.ResponseWriter, r *http.Request) {
	subscriptions_PauseUnpause(w, r, false, "unpause subscription")
}

var dayCountNames []string

func init() {
	for _, day := range util.DaysOfWeek {
		dayCountNames = append(dayCountNames, string(day)+"_count")
	}
}

func Subscriptions_Update(w http.ResponseWriter, r *http.Request) {
	tx := rutil.DBConn(r).MustBegin()
	defer util.CommitOrRollback(tx, true, "")

	subscriptionIdInt, ok := util.URLParamInt64(r, "id")
	if !ok {
		subscriptions_RedirectNotFound(w, r)
		return
	}

	subscriptionId := models.SubscriptionId(subscriptionIdInt)
	subscription, ok := models.Subscription_MustGetUserIdStatusScheduleVersion(tx, subscriptionId)
	if !ok {
		subscriptions_RedirectNotFound(w, r)
		return
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
	models.Schedule_MustUpdate(tx, subscriptionId, countsByDay)
	models.Subscription_MustUpdateScheduleVersion(tx, subscriptionId, newVersion)

	models.ProductEvent_MustEmitSchedule(models.ProductEventScheduleArgs{
		Tx:             tx,
		Request:        r,
		ProductUserId:  rutil.CurrentProductUserId(r),
		EventType:      "update schedule",
		SubscriptionId: subscriptionId,
		BlogBestUrl:    subscription.BlogBestUrl,
		WeeklyCount:    int(totalCount),
		ActiveDays:     productActiveDays,
	})
}

func Subscriptions_Delete(w http.ResponseWriter, r *http.Request) {
	conn := rutil.DBConn(r)
	subscriptionIdInt, ok := util.URLParamInt64(r, "id")
	if !ok {
		subscriptions_RedirectNotFound(w, r)
		return
	}

	subscriptionId := models.SubscriptionId(subscriptionIdInt)
	subscription, ok := models.Subscription_MustGetUserIdStatusBlogBestUrl(conn, subscriptionId)
	if !ok {
		subscriptions_RedirectNotFound(w, r)
		return
	}

	if subscriptions_RedirectIfUserMismatch(w, r, subscription.UserId) {
		return
	}

	models.Subscription_MustDelete(conn, subscriptionId)

	models.ProductEvent_MustEmitFromRequest(models.ProductEventRequestArgs{
		Tx:            conn,
		Request:       r,
		ProductUserId: rutil.CurrentProductUserId(r),
		EventType:     "delete subscription",
		EventProperties: map[string]any{
			"subscription_id": subscriptionId,
			"blog_url":        subscription.BlogBestUrl,
		},
		UserProperties: nil,
	})

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
	userId models.UserId, userSettings models.UserSettings,
) schedulePreview {
	preview := models.Subscription_MustGetSchedulePreview(tx, subscriptionId)
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
		nextScheduleDate = subscriptions_MustGetRealisticScheduleDate(tx, userId, localTime, localDate)
	}
	log.Info().
		Str("date", string(nextScheduleDate)).
		Msg("Preview next schedule date")

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

func subscriptions_MustGetRealisticScheduleDate(
	tx pgw.Queryable, userId models.UserId, localTime time.Time, localDate util.Date,
) util.Date {
	nextScheduleDate := jobs.PublishPostsJob_MustGetNextScheduledDate(tx, userId)
	if nextScheduleDate == "" {
		if util.Schedule_IsEarlyMorning(localTime) {
			return localDate
		} else {
			nextDay := localTime.AddDate(0, 0, 1)
			return util.Schedule_Date(nextDay)
		}
	} else if nextScheduleDate < localDate {
		log.Warn().
			Int64("user_id", int64(userId)).
			Str("next_schedule_date", string(nextScheduleDate)).
			Str("today", string(localDate)).
			Msg("Job is scheduled in the past")
		return localDate
	} else {
		return nextScheduleDate
	}
}

func subscriptions_PauseUnpause(w http.ResponseWriter, r *http.Request, newIsPaused bool, eventName string) {
	subscriptionIdInt, ok := util.URLParamInt64(r, "id")
	if !ok {
		subscriptions_RedirectNotFound(w, r)
		return
	}

	subscriptionId := models.SubscriptionId(subscriptionIdInt)
	conn := rutil.DBConn(r)
	subscription, ok := models.Subscription_MustGetUserIdStatusBlogBestUrl(conn, subscriptionId)
	if !ok {
		subscriptions_RedirectNotFound(w, r)
		return
	}

	if subscriptions_RedirectIfUserMismatch(w, r, subscription.UserId) {
		return
	}

	if subscriptions_BadRequestIfNotLive(w, subscription.Status) {
		return
	}

	models.Subscription_MustSetIsPaused(conn, subscriptionId, newIsPaused)

	models.ProductEvent_MustEmitFromRequest(models.ProductEventRequestArgs{
		Tx:            conn,
		Request:       r,
		ProductUserId: rutil.CurrentProductUserId(r),
		EventType:     eventName,
		EventProperties: map[string]any{
			"subscription_id": subscriptionId,
			"blog_url":        subscription.BlogBestUrl,
		},
		UserProperties: nil,
	})
	w.WriteHeader(http.StatusOK)
}
