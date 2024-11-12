package routes

import (
	"net/http"
	"strings"

	"feedrewind.com/models"
	"feedrewind.com/routes/rutil"
	"feedrewind.com/util"
	"github.com/jackc/pgx/v5"
	"github.com/pkg/errors"
)

func Rss_SubscriptionFeed(w http.ResponseWriter, r *http.Request) {
	pool := rutil.DBPool(r)
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

	row := pool.QueryRow(`
		select
			(is_paused or (final_item_published_at is not null)),
			(select body from subscription_rsses where subscription_id = $1),
			(select coalesce(url, feed_url) from blogs where blogs.id = blog_id),
			(select product_user_id from users_with_discarded where users_with_discarded.id = user_id)
		from subscriptions_without_discarded where id = $1
	`, subscriptionId)
	err := row.Scan(&isPausedOrFinished, &rss, &blogBestUrl, &productUserId)
	if errors.Is(err, pgx.ErrNoRows) {
		w.WriteHeader(http.StatusNotFound)
		return
	} else if err != nil {
		panic(err)
	}

	if !isPausedOrFinished {
		productRssClient := resolveRssClient(r)
		models.ProductEvent_MustEmit(pool, productUserId, "poll feed", map[string]any{
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
	pool := rutil.DBPool(r)
	userIdInt, ok := util.URLParamInt64(r, "id")
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	userId := models.UserId(userIdInt)
	var hasActiveSubscriptions bool
	var rss string
	var productUserId models.ProductUserId
	row := pool.QueryRow(`
		select (
			select count(1) from subscriptions_without_discarded
			where subscriptions_without_discarded.user_id = $1 and
				not is_paused and
				final_item_published_at is null
		) > 0, (
			select body from user_rsses where user_id = $1
		),
		product_user_id
		from users_with_discarded
		where id = $1
	`, userId)
	err := row.Scan(&hasActiveSubscriptions, &rss, &productUserId)
	if errors.Is(err, pgx.ErrNoRows) {
		w.WriteHeader(http.StatusNotFound)
		return
	} else if err != nil {
		panic(err)
	}

	if hasActiveSubscriptions {
		productRssClient := resolveRssClient(r)
		models.ProductEvent_MustEmit(pool, productUserId, "poll feed", map[string]any{
			"feed_type": "user",
			"client":    productRssClient,
		}, nil)
	}

	w.Header().Set("Content-Type", "application/xml")
	util.MustWrite(w, rss)
}

func resolveRssClient(r *http.Request) string {
	userAgent := r.UserAgent()
	switch {
	case strings.HasPrefix(userAgent, "Feedly/"):
		return "Feedly"
	case strings.Contains(userAgent, "inoreader.com;"):
		return "Inoreader"
	default:
		return userAgent
	}
}
