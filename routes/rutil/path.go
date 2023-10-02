package rutil

import (
	"feedrewind/models"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

func BlogUnsupportedPath(blogId models.BlogId) string {
	return fmt.Sprintf("/blogs/%d/unsupported", blogId)
}

func SubscriptionSetupPath(subscriptionId models.SubscriptionId) string {
	return fmt.Sprintf("/subscriptions/%d/setup", subscriptionId)
}

func SubscriptionProgressPath(subscriptionId models.SubscriptionId) string {
	return fmt.Sprintf("/subscriptions/%d/progress", subscriptionId)
}

func SubscriptionSelectPostsPath(subscriptionId models.SubscriptionId) string {
	return fmt.Sprintf("/subscriptions/%d/select_posts", subscriptionId)
}

func SubscriptionMarkWrongPath(subscriptionId models.SubscriptionId) string {
	return fmt.Sprintf("/subscriptions/%d/mark_wrong", subscriptionId)
}

func SubscriptionSchedulePath(subscriptionId models.SubscriptionId) string {
	return fmt.Sprintf("/subscriptions/%d/schedule", subscriptionId)
}

func SubscriptionDeletePath(subscriptionId models.SubscriptionId) string {
	return fmt.Sprintf("/subscriptions/%d/delete", subscriptionId)
}

func SubscriptionAddFeedPath(feedUrl string) string {
	return fmt.Sprintf("/subscriptions/add/%s", url.PathEscape(feedUrl))
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

func SubscriptionUrl(subscriptionId models.SubscriptionId) string {
	return fmt.Sprintf("https://feedrewind.com/subscriptions/%d", subscriptionId)
}

func SubscriptionPath(subscriptionId models.SubscriptionId) string {
	return fmt.Sprintf("/subscriptions/%d", subscriptionId)
}

func SubscriptionProgressStreamUrl(r *http.Request, subscriptionId models.SubscriptionId) string {
	proto := "ws"
	if r.TLS != nil {
		proto = "wss"
	}
	host, port := parseHostPort(r)
	return fmt.Sprintf("%s://%s%s/subscriptions/%d/progress_stream", proto, host, port, subscriptionId)
}

func SubscriptionFeedUrl(r *http.Request, subscriptionId models.SubscriptionId) string {
	proto := "http"
	if r.TLS != nil {
		proto = "https"
	}
	host, port := parseHostPort(r)
	return fmt.Sprintf("%s://%s%s/feeds/%d", proto, host, port, subscriptionId)
}

func parseHostPort(r *http.Request) (host, port string) {
	lastColonIndex := strings.LastIndex(r.Host, ":")
	if lastColonIndex >= 0 {
		host = r.Host[:lastColonIndex]
		port = r.Host[lastColonIndex:]
	} else {
		host = r.Host
		port = ""
	}
	if port == ":80" && r.TLS == nil {
		port = ""
	}
	if port == ":443" && r.TLS != nil {
		port = ""
	}
	return host, port
}

func SubscriptionPostUrl(title, randomId string) string {
	slug := postSlug(title)
	return fmt.Sprintf("https://feedrewind.com/posts/%s/%s/", slug, randomId)
}

var postSlugRegex regexp.Regexp

func init() {
	postSlugRegex = *regexp.MustCompile(`\w+`)
}

func postSlug(title string) string {
	tokens := postSlugRegex.FindAllString(strings.ToLower(title), 10)
	totalLength := 0
	for _, token := range tokens {
		totalLength += len(token)
	}
	totalLength += len(tokens) - 1
	for totalLength > 100 && len(tokens) > 1 {
		totalLength -= len(tokens[len(tokens)-1]) + 1
		tokens = tokens[:len(tokens)-1]
	}
	if totalLength > 100 {
		return tokens[0][:100]
	} else {
		return strings.Join(tokens, "-")
	}
}
