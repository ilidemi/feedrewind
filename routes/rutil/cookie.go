package rutil

import (
	"feedrewind/models"
	"feedrewind/util"
	"net/http"
	"strconv"
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
