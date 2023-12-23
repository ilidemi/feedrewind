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
	var isPausedOrFinished bool
	var rss string
	var blogBestUrl string
	var productUserId models.ProductUserId

	row := conn.QueryRow(`
		select
			(is_paused or final_item_published_at is not null),
			(select body from subscription_rsses where subscription_id = $1),
			(select coalesce(url, feed_url) from blogs where blogs.id = blog_id),
			(select product_user_id from users where users.id = user_id)
		from subscriptions_without_discarded where id = $1
	`, subscriptionId)
	err := row.Scan(&isPausedOrFinished, &rss, &blogBestUrl, &productUserId)
	if err != nil {
		panic(err)
	}

	if !isPausedOrFinished {
		productRssClient := resolveRssClient(r)
		models.ProductEvent_MustEmit(conn, productUserId, "poll feed", map[string]any{
			"subscription_id": subscriptionId,
			"blog_url":        blogBestUrl,
			"feed_type":       "subscription",
			"client":          productRssClient,
		}, nil)
	}

	w.Header().Set("Content-Type", "application/xml")
	util.MustWrite(w, rss)
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
