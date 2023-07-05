package rutil

import (
	"feedrewind/models"
	"fmt"
	"net/http"
	"net/url"
)

func SubscriptionSetupPath(subscriptionId models.SubscriptionId) string {
	return fmt.Sprintf("/subscriptions/%d/setup", subscriptionId)
}

func SubscriptionDeletePath(subscriptionId models.SubscriptionId) string {
	return fmt.Sprintf("/subscriptions/%d/delete", subscriptionId)
}

func SubscriptionShowPath(subscriptionId models.SubscriptionId) string {
	return fmt.Sprintf("/subscriptions/%d", subscriptionId)
}

func SubscriptionPausePath(subscriptionId models.SubscriptionId) string {
	return fmt.Sprintf("/subscriptions/%d/pause", subscriptionId)
}

func SubscriptionUnpausePath(subscriptionId models.SubscriptionId) string {
	return fmt.Sprintf("/subscriptions/%d/unpause", subscriptionId)
}

func SubscriptionFeedUrl(r *http.Request, subscriptionId models.SubscriptionId) string {
	proto := "http"
	if r.TLS != nil {
		proto = "https"
	}

	port := ":" + r.URL.Port()
	if port == ":80" && proto == "http" {
		port = ""
	}
	if port == ":443" && proto == "https" {
		port = ""
	}

	return fmt.Sprintf("%s://%s%s/subscriptions/%d/feed", proto, r.Host, port, subscriptionId)
}

func addFeedPath(feedUrl string) string {
	escapedUrl := url.QueryEscape(feedUrl)
	return fmt.Sprintf("/subscriptions/add/%s", escapedUrl)
}
