package rutil

import (
	"feedrewind/config"
	"feedrewind/models"
	"feedrewind/util"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

func BlogUnsupportedPath(blogId models.BlogId) string {
	return fmt.Sprintf("/blogs/%d/unsupported", blogId)
}

func SubscriptionAddUrl() string {
	return fmt.Sprintf("%s/subscriptions/add", config.Cfg.RootUrl)
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

func SubscriptionNotifyWhenSupportedPath(subscriptionId models.SubscriptionId) string {
	return fmt.Sprintf("/subscriptions/%d/notify_when_supported", subscriptionId)
}

func SubscriptionRequestCustomBlogPath(subscriptionId models.SubscriptionId) string {
	return fmt.Sprintf("/subscriptions/%d/request", subscriptionId)
}

func SubscriptionRequestCustomBlogSubmitPath(subscriptionId models.SubscriptionId) string {
	return fmt.Sprintf("/subscriptions/%d/submit_request", subscriptionId)
}

func SubscriptionRequestCustomBlogCheckoutPath(subscriptionId models.SubscriptionId) string {
	return fmt.Sprintf("/subscriptions/%d/checkout", subscriptionId)
}

func SubscriptionAddFeedPath(feedUrl string) string {
	return util.SubscriptionAddFeedPath(feedUrl)
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
	return fmt.Sprintf("%s/subscriptions/%d", config.Cfg.RootUrl, subscriptionId)
}

func SubscriptionPath(subscriptionId models.SubscriptionId) string {
	return fmt.Sprintf("/subscriptions/%d", subscriptionId)
}

func SubscriptionProgressStreamUrl(r *http.Request, subscriptionId models.SubscriptionId) string {
	proto := "ws"
	if !config.Cfg.Env.IsDevOrTest() {
		proto = "wss"
	}
	host, port := parseHostPort(r)
	return fmt.Sprintf("%s://%s%s/subscriptions/%d/progress_stream", proto, host, port, subscriptionId)
}

func SubscriptionFeedUrl(r *http.Request, subscriptionId models.SubscriptionId) string {
	proto := "http"
	if !config.Cfg.Env.IsDevOrTest() {
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
	if port == ":80" && config.Cfg.Env.IsDevOrTest() {
		port = ""
	}
	if port == ":443" && !config.Cfg.Env.IsDevOrTest() {
		port = ""
	}
	return host, port
}

func SubscriptionPostUrl(title string, randomId models.SubscriptionPostRandomId) string {
	slug := postSlug(title)
	return fmt.Sprintf("%s/posts/%s/%s/", config.Cfg.RootUrl, slug, randomId)
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
