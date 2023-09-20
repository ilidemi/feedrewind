//go:build e2etesting

package e2etest

import (
	"feedrewind/oops"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/stretchr/testify/require"
)

func TestRssSchedule(t *testing.T) {
	scheduleOutputs := map[string]string{
		//                              before pr  arrivalMsg       creation cnt/pr        late cnt/pr            midnight pr        tomorrow cnt/pr
		"job_today_job_tomorrow":     " - td tm da ; one will td  ; 0 ;       - td tm da ; 1 ; td    - tm da    ; ys    - td tm    ; 2 ; ys td - tm   ",
		"job_today_no_tomorrow":      " - td da    ; one will td  ; 0 ;       - td da    ; 1 ; td    - da       ; ys    - tm       ; 1 ; ys    - tm   ",
		"no_today_job_tomorrow":      " - tm da    ; one will tm  ; 0 ;       - tm da    ; 0 ;       - tm da    ;       - td tm    ; 1 ; td    - tm   ",
		"no_today_no_tomorrow":       " - da       ; one will da  ; 0 ;       - da       ; 0 ;       - da       ;       - tm       ; 0 ;       - tm   ",
		"init_today_job_tomorrow":    " - td tm da ; one has      ; 1 ; td    - tm da    ; 1 ; td    - tm da    ; ys    - td tm    ; 2 ; ys td - tm   ",
		"init_today_no_tomorrow":     " - td da    ; one has      ; 1 ; td    - da       ; 1 ; td    - da       ; ys    - tm       ; 1 ; ys    - tm   ",
		"x2_job_today_job_tomorrow":  " - td td tm ; many will td ; 0 ;       - td td tm ; 2 ; td td - tm tm da ; ys ys - td td tm ; 4 ; el td - tm tm",
		"x2_job_today_no_tomorrow":   " - td td da ; many will td ; 0 ;       - td td da ; 2 ; td td - da da    ; ys ys - tm tm    ; 2 ; ys ys - tm tm",
		"x2_no_today_job_tomorrow":   " - tm tm da ; many will tm ; 0 ;       - tm tm da ; 0 ;       - tm tm da ;       - td td tm ; 2 ; td td - tm tm",
		"x2_no_today_no_tomorrow":    " - da da    ; many will da ; 0 ;       - da da    ; 0 ;       - da da    ;       - tm tm    ; 0 ;       - tm tm",
		"x2_init_today_job_tomorrow": " - td td tm ; many have    ; 2 ; td td - tm tm da ; 2 ; td td - tm tm da ; ys ys - td td tm ; 4 ; el td - tm tm",
		"x2_init_today_no_tomorrow":  " - td td da ; many have    ; 2 ; td td - da da    ; 2 ; td td - da da    ; ys ys - tm tm    ; 2 ; ys ys - tm tm",
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
		{"test_pst@test.com", 1, VeryEarly, true, true, "job_today_job_tomorrow"},
		{"test_pst@test.com", 1, VeryEarly, true, false, "job_today_no_tomorrow"},
		{"test_pst@test.com", 1, VeryEarly, false, true, "no_today_job_tomorrow"},
		{"test_pst@test.com", 1, Early, true, true, "init_today_job_tomorrow"},
		{"test_pst@test.com", 1, Early, true, false, "init_today_no_tomorrow"},
		{"test_pst@test.com", 1, Early, false, true, "no_today_job_tomorrow"},
		{"test_pst@test.com", 1, Day, true, true, "no_today_job_tomorrow"},
		{"test_pst@test.com", 1, Day, true, false, "no_today_no_tomorrow"},
		{"test_pst@test.com", 1, Day, false, true, "no_today_job_tomorrow"},
		{"test_nz@test.com", 2, VeryEarly, true, true, "x2_job_today_job_tomorrow"},
		{"test_nz@test.com", 2, VeryEarly, true, false, "x2_job_today_no_tomorrow"},
		{"test_nz@test.com", 2, VeryEarly, false, true, "x2_no_today_job_tomorrow"},
		{"test_nz@test.com", 2, Early, true, true, "x2_init_today_job_tomorrow"},
		{"test_nz@test.com", 2, Early, true, false, "x2_init_today_no_tomorrow"},
		{"test_nz@test.com", 2, Early, false, true, "x2_no_today_job_tomorrow"},
		{"test_nz@test.com", 2, Day, true, true, "x2_no_today_job_tomorrow"},
		{"test_nz@test.com", 2, Day, true, false, "x2_no_today_no_tomorrow"},
		{"test_nz@test.com", 2, Day, false, true, "x2_no_today_job_tomorrow"},
	}

	timezoneByEmail := map[string]string{
		"test_pst@test.com": "America/Los_Angeles",
		"test_nz@test.com":  "Pacific/Auckland",
	}

	for _, tc := range tests {
		description := fmt.Sprintf("%#v", tc)
		timezone := timezoneByEmail[tc.Email]

		todayUtc := time.Date(2022, 6, 1, 0, 0, 0, 0, time.UTC)
		var todayLocal time.Time
		switch timezone {
		case "America/Los_Angeles":
			todayLocal = todayUtc.Add(7 * time.Hour)
		case "Pacific/Auckland":
			todayLocal = todayUtc.Add(-12 * time.Hour)
		default:
			require.FailNowf(t, description, "Unknown timezone: %s", timezone)
		}

		var creationTimestamp time.Time
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

		lateTimestamp := todayLocal.Add(23 * time.Hour)
		midnightTimestamp := todayLocal.AddDate(0, 0, 1).Add(1 * time.Minute)
		tomorrowTimestamp := todayLocal.AddDate(0, 0, 1).Add(3*time.Hour + 1*time.Minute)

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
		outputPreviewLate := outputTokens[5]
		outputAtMidnight := outputTokens[6]
		outputCountTomorrow := outputTokens[7]
		outputPreviewTomorrow := outputTokens[8]

		// Assert preview before
		assertSchedulePreview(t, page, outputPreviewBefore, description)

		page.MustElement("#save_button").MustClick()

		// Assert arrival msg
		var expectedArrivalMsg strings.Builder
		expectedArrivalMsg.WriteString("First")
		outputArrivalMsgTokens := strings.Split(outputArrivalMsg, " ")
		switch outputArrivalMsgTokens[0] {
		case "one":
			expectedArrivalMsg.WriteString(" entry")
		case "many":
			expectedArrivalMsg.WriteString(" entries")
		default:
			require.FailNowf(
				t, description, "Unexpected arrival entries count: %s", outputArrivalMsgTokens[0],
			)
		}

		switch outputArrivalMsgTokens[1] {
		case "will":
			expectedArrivalMsg.WriteString(" will arrive")
			require.Len(t, outputArrivalMsgTokens, 3, description)
			switch outputArrivalMsgTokens[2] {
			case "td":
				expectedArrivalMsg.WriteString(" Wednesday, June 1st")
			case "tm":
				expectedArrivalMsg.WriteString(" Thursday, June 2nd")
			case "da":
				expectedArrivalMsg.WriteString(" Friday, June 3rd")
			default:
				require.FailNowf(t, description, "Unexpected arrival date: %s", outputArrivalMsgTokens[2])
			}
			expectedArrivalMsg.WriteString(" in the morning.")
		case "has":
			require.Len(t, outputArrivalMsgTokens, 2, description)
			expectedArrivalMsg.WriteString(" has already arrived.")
		case "have":
			require.Len(t, outputArrivalMsgTokens, 2, description)
			expectedArrivalMsg.WriteString(" have already arrived.")
		default:
			require.FailNowf(t, description, "Unexpected arrival verb: %s", outputArrivalMsgTokens[1])
		}

		arrivalMsg := page.MustElement("#arrival_msg").MustText()
		require.Equal(t, expectedArrivalMsg.String(), arrivalMsg, description)
		subscriptionId := parseSubscriptionId(page)
		subscriptionPath := fmt.Sprintf("subscriptions/%s", subscriptionId)

		// Assert published at creation
		page = visitDev(browser, subscriptionPath)
		publishedCountAtCreation := parsePublishedCount(page)
		require.Equal(t, outputCountAtCreation, publishedCountAtCreation, description)

		// Assert preview at creation
		assertSchedulePreview(t, page, outputPreviewAtCreation, description)

		// Assert published late
		lateTimestampStr := lateTimestamp.Format(time.RFC3339)
		page = visitAdminf(browser, "travel_to?timestamp=%s", lateTimestampStr)
		require.Equal(t, lateTimestampStr, pageText(page), description)
		page = visitAdmin(browser, "wait_for_publish_posts_job")
		require.Equal(t, "OK", pageText(page), description)
		page = visitDev(browser, subscriptionPath)
		publishedCountLate := parsePublishedCount(page)
		require.Equal(t, outputCountLate, publishedCountLate, description)

		// Assert preview late
		assertSchedulePreview(t, page, outputPreviewLate, description)

		// Assert preview at midnight
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
		page = visitDev(browser, subscriptionPath)
		publishedCountTomorrow := parsePublishedCount(page)
		require.Equal(t, outputCountTomorrow, publishedCountTomorrow, description)

		// Assert preview tomorrow
		assertSchedulePreview(t, page, outputPreviewTomorrow, description)

		// Cleanup
		page = visitAdmin(browser, "travel_back")
		serverTimeStr := pageText(page)
		serverTime, err := time.Parse(time.RFC3339, serverTimeStr)
		oops.RequireNoError(t, err)
		require.InDelta(t, time.Now().Unix(), serverTime.Unix(), 60, description)
		page = visitAdmin(browser, "reschedule_user_job")
		require.Equal(t, "OK", pageText(page), description)
		visitDev(browser, "logout")

		browser.MustClose()
		l.Cleanup()
	}
}
