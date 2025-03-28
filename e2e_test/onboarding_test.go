//go:build e2etesting

package e2etest

import (
	"bytes"
	"encoding/xml"
	"io"
	"net/http"
	"testing"
	"time"

	"feedrewind.com/oops"
	"feedrewind.com/util/schedule"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/stretchr/testify/require"
)

func TestOnboardingSuggestion(t *testing.T) {
	email := "test_onboarding@feedrewind.com"
	timezone := "America/Los_Angeles"

	l := launcher.New().Headless(false)
	browserUrl := l.MustLaunch()
	browser := rod.New().ControlURL(browserUrl).MustConnect()

	page := visitAdminf(browser, "destroy_user?email=%s", email)
	require.Contains(t, []string{"OK", "NotFound"}, mustPageText(page))

	todayUtc := schedule.NewTime(2022, 6, 1, 0, 0, 0, 0, time.UTC)
	todayLocal := todayUtc.Add(7 * time.Hour)
	signupTimestamp := todayLocal.Add(1 * time.Hour)
	rssPublishTimestamp := todayLocal.Add(2 * time.Hour)

	signupTimestampStr := signupTimestamp.Format(time.RFC3339)
	page = visitAdminf(browser, "travel_to?timestamp=%s", signupTimestampStr)
	require.Equal(t, signupTimestampStr, mustPageText(page))

	// Landing
	page = visitDev(browser, "")
	page.MustElement(
		`form[action="/subscriptions/add/https:%2F%2Fwww.brendangregg.com%2Fblog%2Frss.xml"] > button`,
	).MustClick()

	page.MustElementR("input", "Continue").MustClick()

	// Pricing
	page.MustElement("#signup_free").MustClick()

	// Create user
	err := proto.EmulationSetTimezoneOverride{TimezoneID: timezone}.Call(page)
	oops.RequireNoError(t, err)
	page.MustElement("#email").MustInput(email)
	page.MustElement("#new-password").MustInput("tz123456")
	page.MustElementR("input", "Sign up").MustClick()
	page.MustWaitLoad()

	// Add a subscription
	page.MustElement("#wed_add").MustClick()
	page.MustElement("#delivery_rss").MustClick()

	page.MustElement("#save_button").MustClick()
	page.MustElement("#arrival_msg")
	subscriptionId := mustParseSubscriptionId(page)

	// Assert published count
	rssPublishTimestampStr := rssPublishTimestamp.Format(time.RFC3339)
	page = visitAdminf(browser, "travel_to?timestamp=%s", rssPublishTimestampStr)
	require.Equal(t, rssPublishTimestampStr, mustPageText(page))
	page = visitAdmin(browser, "wait_for_publish_posts_job")
	require.Equal(t, "OK", mustPageText(page))
	page = visitDevf(browser, "subscriptions/%s", subscriptionId)
	publishedCount := parsePublishedCount(page)
	require.Equal(t, "1", publishedCount)

	// Get feed body
	feedUrl := page.MustElement("#feed_url").MustText()
	resp, err := http.Get(feedUrl)
	oops.RequireNoError(t, err)
	feedBody, err := io.ReadAll(resp.Body)
	oops.RequireNoError(t, err)
	oops.RequireNoError(t, resp.Body.Close())
	type Item struct {
		Link string `xml:"link"`
	}
	type Channel struct {
		Items []Item `xml:"item"`
	}
	type RSS struct {
		Channel Channel `xml:"channel"`
	}
	var rss RSS
	err = xml.NewDecoder(bytes.NewReader(feedBody)).Decode(&rss)
	oops.RequireNoError(t, err)
	require.NotEmpty(t, rss.Channel.Items)

	// Assert redirect
	resp, err = http.Get(rss.Channel.Items[0].Link)
	oops.RequireNoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	oops.RequireNoError(t, resp.Body.Close())

	// Cleanup
	page = visitAdminf(browser, "destroy_user?email=%s", email)
	require.Equal(t, "OK", mustPageText(page))
	page = visitAdmin(browser, "travel_back")
	serverTimeStr := mustPageText(page)
	serverTime, err := time.Parse(time.RFC3339, serverTimeStr)
	oops.RequireNoError(t, err)
	require.InDelta(t, time.Now().Unix(), serverTime.Unix(), 60)

	browser.MustClose()
	l.Cleanup()
}

func TestOnboardingCustomLink(t *testing.T) {
	email := "test_onboarding@feedrewind.com"
	timezone := "America/Los_Angeles"

	l := launcher.New().Headless(false)
	browserUrl := l.MustLaunch()
	browser := rod.New().ControlURL(browserUrl).MustConnect()

	page := visitAdminf(browser, "destroy_user?email=%s", email)
	require.Contains(t, []string{"OK", "NotFound"}, mustPageText(page))

	todayUtc := schedule.NewTime(2022, 6, 1, 0, 0, 0, 0, time.UTC)
	todayLocal := todayUtc.Add(7 * time.Hour)
	signupTimestamp := todayLocal.Add(1 * time.Hour)
	rssPublishTimestamp := todayLocal.Add(2 * time.Hour)

	signupTimestampStr := signupTimestamp.Format(time.RFC3339)
	page = visitAdminf(browser, "travel_to?timestamp=%s", signupTimestampStr)
	require.Equal(t, signupTimestampStr, mustPageText(page))

	// Landing
	page = visitDev(browser, "")
	page.MustElement("#start_url").MustInput("https://ilidemi.github.io/dummy-blogs/1a/rss.xml")
	page.MustElement("#discover_go").MustClick()

	page.MustElementR("input", "Continue").MustClick()

	// Pricing
	page.MustElement("#signup_free").MustClick()

	// Create user
	err := proto.EmulationSetTimezoneOverride{TimezoneID: timezone}.Call(page)
	oops.RequireNoError(t, err)
	page.MustElement("#email").MustInput(email)
	page.MustElement("#new-password").MustInput("tz123456")
	page.MustElementR("input", "Sign up").MustClick()
	page.MustWaitLoad()

	// Add a subscription
	page.MustElement("#wed_add").MustClick()
	page.MustElement("#delivery_rss").MustClick()

	page.MustElement("#save_button").MustClick()
	page.MustElement("#arrival_msg")
	subscriptionId := mustParseSubscriptionId(page)

	// Assert published count
	rssPublishTimestampStr := rssPublishTimestamp.Format(time.RFC3339)
	page = visitAdminf(browser, "travel_to?timestamp=%s", rssPublishTimestampStr)
	require.Equal(t, rssPublishTimestampStr, mustPageText(page))
	page = visitAdmin(browser, "wait_for_publish_posts_job")
	require.Equal(t, "OK", mustPageText(page))
	page = visitDevf(browser, "subscriptions/%s", subscriptionId)
	publishedCount := parsePublishedCount(page)
	require.Equal(t, "1", publishedCount)

	// Cleanup
	page = visitAdminf(browser, "destroy_user?email=%s", email)
	require.Equal(t, "OK", mustPageText(page))
	page = visitAdmin(browser, "travel_back")
	serverTimeStr := mustPageText(page)
	serverTime, err := time.Parse(time.RFC3339, serverTimeStr)
	oops.RequireNoError(t, err)
	require.InDelta(t, time.Now().Unix(), serverTime.Unix(), 60)

	browser.MustClose()
	l.Cleanup()
}

func TestOnboardingMultipleFeeds(t *testing.T) {
	l := launcher.New().Headless(false)
	browserUrl := l.MustLaunch()
	browser := rod.New().ControlURL(browserUrl).MustConnect()

	feedButtonCount := 6 // 3 feeds, a title and a button each
	for i := range feedButtonCount {
		page := visitDev(browser, "")
		page.MustElement("#start_url").
			MustInput("https://ilidemi.github.io/dummy-blogs/multiple-feeds/multiple/")
		page.MustElement("#discover_go").MustClick()

		feedElements := page.MustElements(".feeds-choose")
		require.Equal(t, feedButtonCount, len(feedElements))
		feedElements[i].MustClick()
		page.MustWaitLoad()

		page.MustElement("#select_posts")
	}

	browser.MustClose()
	l.Cleanup()
}
