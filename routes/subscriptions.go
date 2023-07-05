package routes

import (
	"feedrewind/models"
	"feedrewind/routes/rutil"
	"feedrewind/templates"
	"feedrewind/util"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

func SubscriptionsIndex(w http.ResponseWriter, r *http.Request) {
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

func SubscriptionsDelete(w http.ResponseWriter, r *http.Request) {
	conn := rutil.DBConn(r)
	subscriptionIdStr := chi.URLParam(r, "id")
	subscriptionIdInt, err := strconv.ParseInt(subscriptionIdStr, 10, 64)
	if err != nil {
		panic(err)
	}
	subscriptionId := models.SubscriptionId(subscriptionIdInt)
	subscription, ok := models.Subscription_MustGetUserIdBlogBestUrl(conn, subscriptionId)
	if !ok {
		subscriptionsRedirectNotFound(w, r)
		return
	}

	if subscriptionsRedirectIfUserMismatch(w, r, subscription.UserId) {
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
		subscriptionsRedirectNotFound(w, r)
	}
}

func subscriptionsRedirectNotFound(w http.ResponseWriter, r *http.Request) {
	path := "/"
	if rutil.CurrentUser(r) != nil {
		path = "/subscriptions"
	}
	http.Redirect(w, r, path, http.StatusFound)
}

func subscriptionsRedirectIfUserMismatch(
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
