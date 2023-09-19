//go:build e2etesting

package e2etest

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/go-rod/rod"
	"github.com/goccy/go-json"
)

func visitDev(browser *rod.Browser, path string) *rod.Page {
	return browser.MustPage(fmt.Sprintf("http://localhost:3000/%s", path))
}

func visitDevf(browser *rod.Browser, format string, args ...any) *rod.Page {
	return visitDev(browser, fmt.Sprintf(format, args...))
}

func visitAdmin(browser *rod.Browser, path string) *rod.Page {
	return browser.MustPage(fmt.Sprintf("http://localhost:3000/test/%s", path))
}

func visitAdminf(browser *rod.Browser, format string, args ...any) *rod.Page {
	return visitAdmin(browser, fmt.Sprintf(format, args...))
}

func visitAdminSql(browser *rod.Browser, query string) []map[string]any {
	escapedQuery := url.QueryEscape(query)
	url := fmt.Sprintf("http://localhost:3000/test/execute_sql?query=%s", escapedQuery)
	page := browser.MustPage(url)
	text := pageText(page)
	if text == "" {
		return nil
	}

	var result []map[string]any
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.UseNumber()
	err := decoder.Decode(&result)
	if err != nil {
		panic(err)
	}
	return result
}

var subscriptionIdRegex *regexp.Regexp
var publishedCountRegex *regexp.Regexp

func init() {
	subscriptionIdRegex = regexp.MustCompile("/([0-9]+)")
	publishedCountRegex = regexp.MustCompile("^[0-9]+")
}

func parseSubscriptionId(page *rod.Page) string {
	return subscriptionIdRegex.FindStringSubmatch(page.MustInfo().URL)[1]
}

func pageText(page *rod.Page) string {
	return page.MustElement("body").MustText()
}

func parsePublishedCount(page *rod.Page) string {
	publishedCountText := page.MustElement("#published_count").MustText()
	return publishedCountRegex.FindStringSubmatch(publishedCountText)[0]
}
