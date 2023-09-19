//go:build e2etesting

package e2etest

import (
	"feedrewind/oops"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/stretchr/testify/require"
)

func TestSignupRss(t *testing.T) {
	type TestCase struct {
		Email                 string
		Timezone              string
		SetDeliveryInSettings bool
	}

	tests := []TestCase{
		{
			Email:                 "test_nz@test.com",
			Timezone:              "Pacific/Auckland",
			SetDeliveryInSettings: true,
		},
		{
			Email:                 "test_pst@test.com",
			Timezone:              "America/Los_Angeles",
			SetDeliveryInSettings: true,
		},
		{
			Email:                 "test_nz@test.com",
			Timezone:              "Pacific/Auckland",
			SetDeliveryInSettings: false,
		},
		{
			Email:                 "test_pst@test.com",
			Timezone:              "America/Los_Angeles",
			SetDeliveryInSettings: false,
		},
	}

	for _, tc := range tests {
		setDeliveryPage := "schedule"
		if tc.SetDeliveryInSettings {
			setDeliveryPage = "settings"
		}
		description := fmt.Sprintf(
			"Timezone %s, sign up and choose rss delivery in %s", tc.Timezone, setDeliveryPage,
		)

		l := launcher.New().Headless(false)
		browserUrl := l.MustLaunch()
		browser := rod.New().ControlURL(browserUrl).MustConnect()

		page := visitAdminf(browser, "destroy_user?email=%s", tc.Email)
		require.Contains(t, []string{"OK", "NotFound"}, page.MustElement("body").MustText(), description)

		todayUtc := time.Date(2022, 6, 1, 0, 0, 0, 0, time.UTC)
		var todayLocal time.Time
		switch tc.Timezone {
		case "America/Los_Angeles":
			todayLocal = todayUtc.Add(7 * time.Hour)
		case "Pacific/Auckland":
			todayLocal = todayUtc.Add(-12 * time.Hour)
		default:
			require.Fail(t, "Unknown timezone", description)
		}
		signupTimestamp := todayLocal.Add(1 * time.Hour)
		rssPublishTimestamp := todayLocal.Add(2 * time.Hour)

		signupTimestampStr := signupTimestamp.Format(time.RFC3339)
		page = visitAdminf(browser, "travel_to?timestamp=%s", signupTimestampStr)
		require.Equal(t, signupTimestampStr, page.MustElement("body").MustText(), description)

		// Create user
		page = visitDev(browser, "signup")
		err := proto.EmulationSetTimezoneOverride{TimezoneID: tc.Timezone}.Call(page)
		oops.RequireNoError(t, err, description)
		page.MustElement("#email").MustInput(tc.Email)
		page.MustElement("#new-password").MustInput("tz123456")
		page.MustElementR("input", "Sign up").MustClick()
		page.MustWaitLoad()

		// Assert timezone
		page = visitDev(browser, "settings")
		page.MustElement(fmt.Sprintf("option[value='%s'][selected='selected']", tc.Timezone))

		if tc.SetDeliveryInSettings {
			// Set delivery channel
			page.MustElement("#delivery_rss").MustClick()
			page.MustElement("#delivery_channel_save_spinner.hidden")
		}

		// Add a subscription
		page = visitDev(browser, "subscriptions/add")
		page.MustElement("#start_url").MustInput("https://ilidemi.github.io/dummy-blogs/1a/rss.xml")
		page.MustElementR("button", "Go").MustClick()

		page.MustElementR("input", "Continue").MustClick()

		page.MustElement("#wed_add").MustClick()

		if tc.SetDeliveryInSettings {
			_, err := page.Sleeper(rod.NotFoundSleeper).Element("#delivery_channel_picker")
			require.ErrorIs(t, err, &rod.ErrElementNotFound{}, description)
		} else {
			page.MustElement("#delivery_rss").MustClick()
		}

		page.MustElement("#save_button").MustClick()
		page.MustElement("#arrival_msg")
		subscriptionId := parseSubscriptionId(page)

		// Assert published count
		rssPublishTimestampStr := rssPublishTimestamp.Format(time.RFC3339)
		page = visitAdminf(browser, "travel_to?timestamp=%s", rssPublishTimestampStr)
		require.Equal(t, rssPublishTimestampStr, page.MustElement("body").MustText(), description)
		page = visitAdmin(browser, "wait_for_publish_posts_job")
		require.Equal(t, "OK", page.MustElement("body").MustText(), description)
		page = visitDevf(browser, "subscriptions/%s", subscriptionId)
		publishedCountText := page.MustElement("#published_count").MustText()
		publishedCountRegex := regexp.MustCompile("^[0-9]+")
		publishedCount := publishedCountRegex.FindStringSubmatch(publishedCountText)[0]
		require.Equal(t, "1", publishedCount)

		// Cleanup
		page = visitAdmin(browser, "travel_back")
		serverTimeStr := page.MustElement("body").MustText()
		serverTime, err := time.Parse(time.RFC3339, serverTimeStr)
		oops.RequireNoError(t, err)
		require.InDelta(t, time.Now().Unix(), serverTime.Unix(), 60)
		page = visitAdmin(browser, "reschedule_user_job")
		require.Equal(t, "OK", page.MustElement("body").MustText())

		browser.MustClose()
		l.Cleanup()
	}
}
