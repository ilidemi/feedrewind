package routes

import (
	"feedrewind/models"
	"feedrewind/routes/rutil"
	"feedrewind/util"
	"net/http"
)

func Posts_Post(w http.ResponseWriter, r *http.Request) {
	randomId := models.SubscriptionPostRandomId(util.URLParamStr(r, "random_id"))
	conn := rutil.DBConn(r)
	post, err := models.SubscriptionPost_GetByRandomId(conn, randomId)
	if err != nil {
		panic(err)
	}

	models.ProductEvent_MustEmit(conn, post.ProductUserId, "open post", map[string]any{
		"subscription_id": post.SubscriptionId,
		"blog_url":        post.BlogBestUrl,
	}, nil)

	http.Redirect(w, r, post.Url, http.StatusFound)
}
