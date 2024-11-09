package routes

import (
	"net/http"

	"feedrewind.com/models"
	"feedrewind.com/routes/rutil"
	"feedrewind.com/util"
)

func Posts_Post(w http.ResponseWriter, r *http.Request) {
	randomId := models.SubscriptionPostRandomId(util.URLParamStr(r, "random_id"))
	pool := rutil.DBPool(r)
	row := pool.QueryRow(`
		select
			(select url from blog_posts where blog_posts.id = subscription_posts.blog_post_id),
			subscription_id,
			(select coalesce(url, feed_url) from blogs where blogs.id = (
				select blog_id from blog_posts where blog_posts.id = subscription_posts.blog_post_id
			)),
			(select product_user_id from users_with_discarded where users_with_discarded.id = (
				select user_id from subscriptions_with_discarded
				where subscriptions_with_discarded.id = subscription_id
			))
		from subscription_posts
		where random_id = $1
	`, randomId)
	var url string
	var subscriptionId models.SubscriptionId
	var blogBestUrl string
	var productUserId models.ProductUserId
	err := row.Scan(&url, &subscriptionId, &blogBestUrl, &productUserId)
	if err != nil {
		panic(err)
	}

	models.ProductEvent_MustEmit(pool, productUserId, "open post", map[string]any{
		"subscription_id": subscriptionId,
		"blog_url":        blogBestUrl,
	}, nil)

	http.Redirect(w, r, url, http.StatusSeeOther)
}
