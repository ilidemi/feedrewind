package util

import (
	"fmt"
	"net/url"
)

func addFeedPath(feedUrl string) string {
	escapedUrl := url.QueryEscape(feedUrl)
	return fmt.Sprintf("/subscriptions/add/%s", escapedUrl)
}
