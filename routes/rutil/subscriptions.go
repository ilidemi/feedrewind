package rutil

import (
	"feedrewind/models"
	"fmt"
	"net/url"
)

func SubscriptionSetupPath(subscriptionId models.SubscriptionId) string {
	return fmt.Sprintf("/subscriptions/%d/setup", subscriptionId)
}

func addFeedPath(feedUrl string) string {
	escapedUrl := url.QueryEscape(feedUrl)
	return fmt.Sprintf("/subscriptions/add/%s", escapedUrl)
}
