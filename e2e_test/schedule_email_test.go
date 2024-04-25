//go:build e2etesting && emailtesting

package e2etest

import (
	"feedrewind/oops"
	"feedrewind/util"
	"feedrewind/util/schedule"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/stretchr/testify/require"
)

func TestEmailSchedule(t *testing.T) {
	scheduleOutputs := map[string]string{
		//                             before pr  arrivalMsg  creation cnt/pr  late cnt/last_ts/pr          midnight pr        tomorrow cnt/last_ts/pr
		"job_today_job_tomorrow":    " - td tm da ; one td  ; 1 ; - td tm da ; 2 ; td  ; td    - tm da    ; ys    - td tm    ; 3 ; td  ; ys td - tm",
		"job_today_no_tomorrow":     " - td da    ; one td  ; 1 ; - td da    ; 2 ; td  ; td    - da       ; ys    - tm       ; 2 ; ys  ; ys    - tm",
		"no_today_job_tomorrow":     " - tm da    ; one tm  ; 1 ; - tm da    ; 1 ; crt ;       - tm da    ;       - td tm    ; 2 ; td  ; td    - tm",
		"no_today_no_tomorrow":      " - da       ; one da  ; 1 ; - da       ; 1 ; crt ;       - da       ;       - tm       ; 1 ; crt ;       - tm",
		"x2_job_today_job_tomorrow": " - td td tm ; many td ; 1 ; - td td tm ; 3 ; td  ; td td - tm tm da ; ys ys - td td tm ; 5 ; td  ; el td - tm tm",
		"x2_job_today_no_tomorrow":  " - td td da ; many td ; 1 ; - td td da ; 3 ; td  ; td td - da da    ; ys ys - tm tm    ; 3 ; ys  ; ys ys - tm tm",
		"x2_no_today_job_tomorrow":  " - tm tm da ; many tm ; 1 ; - tm tm da ; 1 ; crt ;       - tm tm da ;       - td td tm ; 3 ; td  ; td td - tm tm",
		"x2_no_today_no_tomorrow":   " - da da    ; many da ; 1 ; - da da    ; 1 ; crt ;       - da da    ;       - tm tm    ; 1 ; crt ;       - tm tm",
	}

	type CreationTime int
	const (
		VeryEarly CreationTime = iota
		Early
		Day
	)

	type Test struct {
		Email           string
		PublishCount    int
		CreationTime    CreationTime
		PublishToday    bool
		PublishTomorrow bool
		OutputName      string
	}

	tests := []Test{
		{"test_email_pst@feedrewind.com", 1, VeryEarly, true, true, "job_today_job_tomorrow"},
		{"test_email_pst@feedrewind.com", 1, VeryEarly, true, false, "job_today_no_tomorrow"},
		{"test_email_pst@feedrewind.com", 1, VeryEarly, false, true, "no_today_job_tomorrow"},
		{"test_email_pst@feedrewind.com", 1, Early, true, true, "job_today_job_tomorrow"},
		{"test_email_pst@feedrewind.com", 1, Early, true, false, "job_today_no_tomorrow"},
		{"test_email_pst@feedrewind.com", 1, Early, false, true, "no_today_job_tomorrow"},
		{"test_email_pst@feedrewind.com", 1, Day, true, true, "no_today_job_tomorrow"},
		{"test_email_pst@feedrewind.com", 1, Day, true, false, "no_today_no_tomorrow"},
		{"test_email_pst@feedrewind.com", 1, Day, false, true, "no_today_job_tomorrow"},
		{"test_email_nz@feedrewind.com", 2, VeryEarly, true, true, "x2_job_today_job_tomorrow"},
		{"test_email_nz@feedrewind.com", 2, VeryEarly, true, false, "x2_job_today_no_tomorrow"},
		{"test_email_nz@feedrewind.com", 2, VeryEarly, false, true, "x2_no_today_job_tomorrow"},
		{"test_email_nz@feedrewind.com", 2, Early, true, true, "x2_job_today_job_tomorrow"},
		{"test_email_nz@feedrewind.com", 2, Early, true, false, "x2_job_today_no_tomorrow"},
		{"test_email_nz@feedrewind.com", 2, Early, false, true, "x2_no_today_job_tomorrow"},
		{"test_email_nz@feedrewind.com", 2, Day, true, true, "x2_no_today_job_tomorrow"},
		{"test_email_nz@feedrewind.com", 2, Day, true, false, "x2_no_today_no_tomorrow"},
		{"test_email_nz@feedrewind.com", 2, Day, false, true, "x2_no_today_job_tomorrow"},
	}

	timezoneByEmail := map[string]string{
		"test_email_pst@feedrewind.com": "America/Los_Angeles",
		"test_email_nz@feedrewind.com":  "Pacific/Auckland",
	}

	for _, tc := range tests {
		description := fmt.Sprintf("%#v", tc)
		timezone := timezoneByEmail[tc.Email]

		todayUtc := schedule.NewTime(2022, 6, 1, 0, 0, 0, 0, time.UTC)
		var todayLocal schedule.Time
		switch timezone {
		case "America/Los_Angeles":
			todayLocal = todayUtc.Add(7 * time.Hour)
		case "Pacific/Auckland":
			todayLocal = todayUtc.Add(-12 * time.Hour)
		default:
			require.FailNowf(t, description, "Unknown timezone: %s", timezone)
		}

		var creationTimestamp schedule.Time
		switch tc.CreationTime {
		case VeryEarly:
			creationTimestamp = todayLocal.Add(1 * time.Hour)
		case Early:
			creationTimestamp = todayLocal.Add(4 * time.Hour)
		case Day:
			creationTimestamp = todayLocal.Add(13 * time.Hour)
		default:
			panic("Unknown creation time")
		}

		todayEmailTimestamp := todayLocal.Add(5 * time.Hour)
		lateTimestamp := todayLocal.Add(23 * time.Hour)
		midnightTimestamp := todayLocal.AddDate(0, 0, 1).Add(1 * time.Minute)
		tomorrowEmailTimestamp := todayLocal.AddDate(0, 0, 1).Add(5 * time.Hour)
		tomorrowTimestamp := todayLocal.AddDate(0, 0, 1).Add(6*time.Hour + 1*time.Minute)

		l := launcher.New().Headless(false)
		browserUrl := l.MustLaunch()
		browser := rod.New().ControlURL(browserUrl).MustConnect()

		page := visitDev(browser, "login")
		page.MustElement("#email").MustInput(tc.Email)
		page.MustElement("#current-password").MustInput("tz123456")
		page.MustElementR("input", "Sign in").MustClick()
		page.MustWaitLoad()

		page = visitAdmin(browser, "destroy_user_subscriptions")
		require.Equal(t, "OK", pageText(page), description)

		creationTimestampStr := creationTimestamp.Format(time.RFC3339)
		page = visitAdminf(browser, "travel_to?timestamp=%s", creationTimestampStr)
		require.Equal(t, creationTimestampStr, pageText(page), description)
		page = visitAdmin(browser, "reschedule_user_job")
		require.Equal(t, "OK", pageText(page), description)

		emailMetadata, err := util.RandomInt63()
		oops.RequireNoError(t, err, description)
		page = visitAdminf(browser, "set_email_metadata?value=%d", emailMetadata)
		require.Equal(t, "OK", pageText(page), description)

		page = visitDev(browser, "subscriptions/add")
		page.MustElement("#start_url").MustInput("https://ilidemi.github.io/dummy-blogs/1a/rss.xml")
		page.MustElementR("button", "Go").MustClick()

		page.MustElementR("input", "Continue").MustClick()

		if tc.PublishToday {
			for i := 0; i < tc.PublishCount; i++ {
				page.MustElement("#wed_add").MustClick()
			}
		}

		if tc.PublishTomorrow {
			for i := 0; i < tc.PublishCount; i++ {
				page.MustElement("#thu_add").MustClick()
			}
		}

		for i := 0; i < tc.PublishCount; i++ {
			page.MustElement("#fri_add").MustClick()
		}
		page.MustElement("tr.next_post")

		outputStr := scheduleOutputs[tc.OutputName]
		outputTokens := splitAndTrimSpace(outputStr, ";")
		outputPreviewBefore := outputTokens[0]
		outputArrivalMsg := outputTokens[1]
		outputCountAtCreation := outputTokens[2]
		outputPreviewAtCreation := outputTokens[3]
		outputCountLate := outputTokens[4]
		outputLastTimestampLate := outputTokens[5]
		outputPreviewLate := outputTokens[6]
		outputAtMidnight := outputTokens[7]
		outputCountTomorrow := outputTokens[8]
		outputLastTimestampTomorrow := outputTokens[9]
		outputPreviewTomorrow := outputTokens[10]

		// Assert preview before
		assertSchedulePreview(t, page, outputPreviewBefore, description)

		page.MustElement("#save_button").MustClick()

		// Assert arrival msg
		var expectedArrivalMsg strings.Builder
		outputArrivalMsgTokens := strings.Split(outputArrivalMsg, " ")
		require.Len(t, outputArrivalMsgTokens, 2, description)
		switch outputArrivalMsgTokens[0] {
		case "one":
			expectedArrivalMsg.WriteString("First entry")
		case "many":
			expectedArrivalMsg.WriteString("First entries")
		default:
			require.FailNowf(
				t, description, "Unexpected arrival entries count: %s", outputArrivalMsgTokens[0],
			)
		}

		expectedArrivalMsg.WriteString(" will be sent on")
		switch outputArrivalMsgTokens[1] {
		case "td":
			expectedArrivalMsg.WriteString(" Wednesday, June 1st")
		case "tm":
			expectedArrivalMsg.WriteString(" Thursday, June 2nd")
		case "da":
			expectedArrivalMsg.WriteString(" Friday, June 3rd")
		default:
			require.FailNowf(t, description, "Unexpected arrival date: %s", outputArrivalMsgTokens[2])
		}

		expectedArrivalMsg.WriteString(" to ")
		expectedArrivalMsg.WriteString(tc.Email)
		expectedArrivalMsg.WriteString(".")

		arrivalMsg := page.MustElement("#arrival_msg").MustText()
		require.Equal(t, expectedArrivalMsg.String(), arrivalMsg, description)
		subscriptionId := parseSubscriptionId(page)
		subscriptionPath := fmt.Sprintf("subscriptions/%s", subscriptionId)

		// Assert published at creation
		creationTimestampUTCStr := creationTimestamp.MustUTCString()
		page = visitAdminf(
			browser,
			"assert_email_count_with_metadata?value=%d&count=%s&last_timestamp=%s&last_tag=subscription_initial",
			emailMetadata, outputCountAtCreation, creationTimestampUTCStr,
		)
		require.Equal(t, "OK", pageText(page), description)

		// Assert preview at creation
		page = visitDev(browser, subscriptionPath)
		assertSchedulePreview(t, page, outputPreviewAtCreation, description)

		// Assert published late
		lateTimestampStr := lateTimestamp.Format(time.RFC3339)
		page = visitAdminf(browser, "travel_to?timestamp=%s", lateTimestampStr)
		require.Equal(t, lateTimestampStr, pageText(page), description)
		page = visitAdmin(browser, "wait_for_publish_posts_job")
		require.Equal(t, "OK", pageText(page), description)
		var lastTimestampLate schedule.Time
		var lastTagLate string
		switch outputLastTimestampLate {
		case "crt":
			lastTimestampLate = creationTimestamp
			lastTagLate = "subscription_initial"
		case "td":
			lastTimestampLate = todayEmailTimestamp
			lastTagLate = "subscription_post"
		default:
			require.FailNowf(t, description, "Unexpected last timestamp late: %s", outputLastTimestampLate)
		}
		lastTimestampLateStr := lastTimestampLate.MustUTCString()
		page = visitAdminf(
			browser,
			"assert_email_count_with_metadata?value=%d&count=%s&last_timestamp=%s&last_tag=%s",
			emailMetadata, outputCountLate, lastTimestampLateStr, lastTagLate,
		)
		require.Equal(t, "OK", pageText(page), description)

		// Assert preview late
		page = visitDev(browser, subscriptionPath)
		assertSchedulePreview(t, page, outputPreviewLate, description)

		// Assert preview at midhight
		midnightTimestampStr := midnightTimestamp.Format(time.RFC3339)
		page = visitAdminf(browser, "travel_to?timestamp=%s", midnightTimestampStr)
		require.Equal(t, midnightTimestampStr, pageText(page), description)
		page = visitDev(browser, subscriptionPath)
		assertSchedulePreview(t, page, outputAtMidnight, description)

		// Assert published tomorrow
		tomorrowTimestampStr := tomorrowTimestamp.Format(time.RFC3339)
		page = visitAdminf(browser, "travel_to?timestamp=%s", tomorrowTimestampStr)
		require.Equal(t, tomorrowTimestampStr, pageText(page), description)
		page = visitAdmin(browser, "wait_for_publish_posts_job")
		require.Equal(t, "OK", pageText(page), description)
		var lastTimestampTomorrow schedule.Time
		var lastTagTomorrow string
		switch outputLastTimestampTomorrow {
		case "crt":
			lastTimestampTomorrow = creationTimestamp
			lastTagTomorrow = "subscription_initial"
		case "ys":
			lastTimestampTomorrow = todayEmailTimestamp
			lastTagTomorrow = "subscription_post"
		case "td":
			lastTimestampTomorrow = tomorrowEmailTimestamp
			lastTagTomorrow = "subscription_post"
		default:
			require.FailNowf(
				t, description, "Unexpected last timestamp tomorrow: %s", outputLastTimestampTomorrow,
			)
		}
		lastTimestampTomorrowStr := lastTimestampTomorrow.MustUTCString()
		page = visitAdminf(
			browser,
			"assert_email_count_with_metadata?value=%d&count=%s&last_timestamp=%s&last_tag=%s",
			emailMetadata, outputCountTomorrow, lastTimestampTomorrowStr, lastTagTomorrow,
		)
		require.Equal(t, "OK", pageText(page), description)

		// Assert preview tomorrow
		page = visitDev(browser, subscriptionPath)
		assertSchedulePreview(t, page, outputPreviewTomorrow, description)

		// Cleanup
		page = visitAdmin(browser, "travel_back")
		serverTimeStr := pageText(page)
		serverTime, err := time.Parse(time.RFC3339, serverTimeStr)
		oops.RequireNoError(t, err)
		require.InDelta(t, time.Now().Unix(), serverTime.Unix(), 60, description)
		page = visitAdmin(browser, "reschedule_user_job")
		require.Equal(t, "OK", pageText(page), description)
		page = visitAdmin(browser, "delete_email_metadata")
		require.Equal(t, "OK", pageText(page), description)
		page = visitAdmin(browser, "destroy_user_subscriptions")
		require.Equal(t, "OK", pageText(page), description)
		visitDev(browser, "logout")

		browser.MustClose()
		l.Cleanup()
	}
}
