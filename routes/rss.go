package routes

import (
	"feedrewind/models"
	"feedrewind/routes/rutil"
	"feedrewind/util"
	"net/http"
	"strings"
)

func Rss_SubscriptionFeed(w http.ResponseWriter, r *http.Request) {
	conn := rutil.DBConn(r)
	subscriptionIdInt, ok := util.URLParamInt64(r, "id")
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	subscriptionId := models.SubscriptionId(subscriptionIdInt)
	subscription, err := models.Subscription_GetWithRss(conn, subscriptionId)
	if err != nil {
		panic(err)
	}

	if !subscription.IsPausedOrFinished {
		productRssClient := resolveRssClient(r)
		models.ProductEvent_MustEmit(conn, subscription.ProductUserId, "poll feed", map[string]any{
			"subscription_id": subscriptionId,
			"blog_url":        subscription.BlogBestUrl,
			"feed_type":       "subscription",
			"client":          productRssClient,
		}, nil)
	}

	w.Header().Set("Content-Type", "application/xml")
	util.MustWrite(w, subscription.Rss)
}

func Rss_UserFeed(w http.ResponseWriter, r *http.Request) {
	conn := rutil.DBConn(r)
	userIdInt, ok := util.URLParamInt64(r, "id")
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	userId := models.UserId(userIdInt)
	user, err := models.User_GetWithRss(conn, userId)
	if err != nil {
		panic(err)
	}

	if user.AnySubcriptionNotPausedOrFinished {
		productRssClient := resolveRssClient(r)
		models.ProductEvent_MustEmit(conn, user.ProductUserId, "poll feed", map[string]any{
			"feed_type": "user",
			"client":    productRssClient,
		}, nil)
	}

	w.Header().Set("Content-Type", "application/xml")
	util.MustWrite(w, user.Rss)
}

func resolveRssClient(r *http.Request) string {
	userAgent := r.UserAgent()
	if strings.HasPrefix(userAgent, "Feedly/") {
		return "Feedly"
	} else if strings.Contains(userAgent, "inoreader.com;") {
		return "Inoreader"
	} else {
		return userAgent
	}
}
