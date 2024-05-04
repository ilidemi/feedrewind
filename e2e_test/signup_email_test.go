//go:build emailtesting

package e2etest

import (
	"feedrewind/oops"
	"feedrewind/util"
	"feedrewind/util/schedule"
	"fmt"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/stretchr/testify/require"
)

func TestSignupEmail(t *testing.T) {
	type TestCase struct {
		Email                 string
		Timezone              string
		SetDeliveryInSettings bool
	}

	tests := []TestCase{
		{
			Email:                 "test_email_signup_nz@feedrewind.com",
			Timezone:              "Pacific/Auckland",
			SetDeliveryInSettings: true,
		},
		{
			Email:                 "test_email_signup_pst@feedrewind.com",
			Timezone:              "America/Los_Angeles",
			SetDeliveryInSettings: true,
		},
		{
			Email:                 "test_email_signup_nz@feedrewind.com",
			Timezone:              "Pacific/Auckland",
			SetDeliveryInSettings: false,
		},
		{
			Email:                 "test_email_signup_pst@feedrewind.com",
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
			"Timezone %s, sign up and choose email delivery in %s", tc.Timezone, setDeliveryPage,
		)

		l := launcher.New().Headless(false)
		browserUrl := l.MustLaunch()
		browser := rod.New().ControlURL(browserUrl).MustConnect()

		page := visitAdminf(browser, "destroy_user?email=%s", tc.Email)
		require.Contains(t, []string{"OK", "NotFound"}, pageText(page), description)

		todayUtc := schedule.NewTime(2022, 6, 1, 0, 0, 0, 0, time.UTC)
		var todayLocal schedule.Time
		switch tc.Timezone {
		case "America/Los_Angeles":
			todayLocal = todayUtc.Add(7 * time.Hour)
		case "Pacific/Auckland":
			todayLocal = todayUtc.Add(-12 * time.Hour)
		default:
			require.Fail(t, "Unknown timezone", description)
		}
		signupTimestamp := todayLocal.Add(1 * time.Hour)
		emailTimestamp := todayLocal.Add(5 * time.Hour)

		signupTimestampStr := signupTimestamp.Format(time.RFC3339)
		page = visitAdminf(browser, "travel_to?timestamp=%s", signupTimestampStr)
		require.Equal(t, signupTimestampStr, pageText(page), description)

		emailMetadata, err := util.RandomInt63()
		oops.RequireNoError(t, err, description)
		page = visitAdminf(browser, "set_test_singleton?key=email_metadata&value=%d", emailMetadata)
		require.Equal(t, "OK", pageText(page), description)

		// Create user
		page = visitDev(browser, "signup")
		err = proto.EmulationSetTimezoneOverride{TimezoneID: tc.Timezone}.Call(page)
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
			page.MustElement("#delivery_email").MustClick()
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
			page.MustElement("#delivery_email").MustClick()
		}

		page.MustElement("#save_button").MustClick()
		page.MustElement("#arrival_msg")

		// Assert published count
		emailTimestampStr := emailTimestamp.Format(time.RFC3339)
		page = visitAdminf(browser, "travel_to?timestamp=%s", emailTimestampStr)
		require.Equal(t, emailTimestampStr, pageText(page), description)
		page = visitAdmin(browser, "wait_for_publish_posts_job")
		require.Equal(t, "OK", pageText(page), description)
		emailTimestampUTCStr := emailTimestamp.MustUTCString()
		page = visitAdminf(
			browser,
			"assert_email_count_with_metadata?value=%d&count=2&last_timestamp=%s&last_tag=subscription_post",
			emailMetadata, emailTimestampUTCStr,
		)
		require.Equal(t, "OK", pageText(page), description)

		// Cleanup
		page = visitAdmin(browser, "travel_back")
		serverTimeStr := pageText(page)
		serverTime, err := time.Parse(time.RFC3339, serverTimeStr)
		oops.RequireNoError(t, err)
		require.InDelta(t, time.Now().Unix(), serverTime.Unix(), 60, description)
		page = visitAdmin(browser, "reschedule_user_job")
		require.Equal(t, "OK", pageText(page), description)
		page = visitAdmin(browser, "delete_test_singleton?key=email_metadata")
		require.Equal(t, "OK", pageText(page), description)
		page = visitAdminf(browser, "destroy_user?email=%s", tc.Email)
		require.Equal(t, "OK", pageText(page), description)

		browser.MustClose()
		l.Cleanup()
	}
}
