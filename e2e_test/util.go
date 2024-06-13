package e2etest

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/go-rod/rod"
	"github.com/goccy/go-json"
	"github.com/stretchr/testify/require"
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
	text := mustPageText(page)
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

func mustParseSubscriptionId(page *rod.Page) string {
	return subscriptionIdRegex.FindStringSubmatch(page.MustInfo().URL)[1]
}

func mustPageText(page *rod.Page) string {
	return page.MustElement("body").MustText()
}

func mustRequireNoElement(t *testing.T, page *rod.Page, selector string) {
	elements := page.MustElements(selector)
	require.True(t, elements.Empty())
}

func parsePublishedCount(page *rod.Page) string {
	publishedCountText := page.MustElement("#published_count").MustText()
	return publishedCountRegex.FindStringSubmatch(publishedCountText)[0]
}

func requireSchedulePreview(t *testing.T, page *rod.Page, expectedPreview string, description string) {
	expectedTokens := strings.Split(expectedPreview, "-")
	expectedPrevStr := strings.TrimSpace(expectedTokens[0])
	expectedNextStr := strings.TrimSpace(expectedTokens[1])
	var expectedPrev []string
	var expectedNext []string
	if expectedPrevStr != "" {
		expectedPrev = strings.Split(expectedPrevStr, " ")
	}
	if expectedNextStr != "" {
		expectedNext = strings.Split(expectedNextStr, " ")
	}

	tbody := page.MustElement("#schedule_preview").MustElement("tbody")
	prevRows := tbody.MustElements("tr.prev_post")
	if len(expectedPrev) != len(prevRows) {
		_ = 0
	}
	require.Equal(t, len(expectedPrev), len(prevRows), description)
	for i, expected := range expectedPrev {
		row := prevRows[i]
		rowDate := row.MustElementX(".//td[2]").MustText()
		var expectedDate string
		switch expected {
		case "el":
			expectedDate = "…"
		case "ys":
			expectedDate = "Yesterday"
		case "td":
			expectedDate = "Today"
		default:
			require.FailNowf(t, description, "Unknown date: %s", expected)
		}
		require.Equal(t, expectedDate, rowDate, description)
	}

	nextRows := tbody.MustElements("tr.next_post")
	for i, expected := range expectedNext {
		row := nextRows[i]
		rowDate := row.MustElementX(".//td[2]").MustText()
		var expectedDate string
		switch expected {
		case "td":
			expectedDate = "Today"
		case "tm":
			expectedDate = "Tomorrow"
		case "da":
			expectedDate = "Fri, June 3"
		default:
			require.FailNowf(t, description, "Unknown date: %s", expected)
		}
		require.Equal(t, expectedDate, rowDate, description)
	}
}

func splitAndTrimSpace(s string, sep string) []string {
	tokens := strings.Split(s, sep)
	trimmedTokens := make([]string, len(tokens))
	for i, token := range tokens {
		trimmedTokens[i] = strings.TrimSpace(token)
	}
	return trimmedTokens
}
