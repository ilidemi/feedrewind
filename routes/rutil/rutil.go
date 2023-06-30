package rutil

import (
	"feedrewind/models"
	"feedrewind/util"
	"net/http"
	"strconv"

	"github.com/goccy/go-json"
)

// Returns 0 if not found
func MustExtractAnonymousSubscriptionId(w http.ResponseWriter, r *http.Request) models.SubscriptionId {
	var subscriptionId models.SubscriptionId
	const anonSubscription = "anonymous_subscription"
	if subscriptionIdStr, ok := util.FindCookie(r, anonSubscription); ok {
		subscriptionIdInt, _ := strconv.ParseInt(subscriptionIdStr, 10, 64)
		subscriptionId = models.SubscriptionId(subscriptionIdInt)
		util.DeleteCookie(w, anonSubscription)
	}
	return subscriptionId
}

func MustWriteJson(w http.ResponseWriter, statusCode int, data map[string]any) {
	bytes, err := json.Marshal(data)
	if err != nil {
		panic(err)
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	_, err = w.Write(bytes)
	if err != nil {
		panic(err)
	}
}
