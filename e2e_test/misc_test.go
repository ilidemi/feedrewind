//go:build e2etesting

package e2etest

import (
	"encoding/json"
	"feedrewind/models"
	"feedrewind/oops"
	"feedrewind/util/schedule"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/stretchr/testify/require"
)

func TestDoubleSchedule(t *testing.T) {
	l := launcher.New().Headless(false)
	defer l.Cleanup()
	browserUrl := l.MustLaunch()

	browser := rod.New().ControlURL(browserUrl).MustConnect()
	defer browser.MustClose()

	email := "test_pst@feedrewind.com"

	page := visitDev(browser, "login")
	page.MustElement("#email").MustInput(email)
	page.MustElement("#current-password").MustInput("tz123456")
	page.MustElementR("input", "Sign in").MustClick()
	page.MustWaitLoad()

	page = visitAdmin(browser, "destroy_user_subscriptions")
	require.Equal(t, "OK", pageText(page))

	todayUtc := schedule.NewTime(2022, 6, 1, 0, 0, 0, 0, time.UTC)
	todayLocal := todayUtc.Add(7 * time.Hour)
	todayLocal1AM := todayLocal.Add(1 * time.Hour)
	todayLocal1AMStr := todayLocal1AM.Format(time.RFC3339)
	page = visitAdminf(browser, "travel_to?timestamp=%s", todayLocal1AMStr)
	require.Equal(t, todayLocal1AMStr, pageText(page))
	page = visitAdmin(browser, "reschedule_user_job")
	require.Equal(t, "OK", pageText(page))

	userRows := visitAdminSql(browser, fmt.Sprintf("select id from users where email = '%s'", email))
	userId := userRows[0]["id"]

	initialJobRows := visitAdminSql(browser, fmt.Sprintf(`
		select id from delayed_jobs
		where handler like '%%PublishPostsJob%%'
			and handler like E'%%\n  - %s\n%%'
	`, userId))
	require.Len(t, initialJobRows, 1)
	initialJobId := initialJobRows[0]["id"]

	page = visitDev(browser, "subscriptions/add")
	page.MustElement("#start_url").MustInput("https://ilidemi.github.io/dummy-blogs/1man/rss.xml")
	page.MustElementR("button", "Go").MustClick()
	page.MustWaitLoad()

	for page.MustHas("#progress_count") {
		time.Sleep(100 * time.Millisecond)
	}

	page.MustElementR("input", "Continue").MustClick()

	page.MustElement("#wed_add").MustClick()
	page.MustElement("#save_button").MustClick()
	page.MustElement("#arrival_msg")
	subscriptionId := parseSubscriptionId(page)

	// Duplicate the job
	visitAdminSql(browser, fmt.Sprintf(`
		insert into delayed_jobs (priority, attempts, handler, run_at, queue, created_at, updated_at)
		select priority, attempts, handler, run_at, queue, created_at, updated_at
		from delayed_jobs
		where id = %s
		returning *
	`, initialJobId))
	duplicateJobRows := visitAdminSql(browser, fmt.Sprintf(`
		select id from delayed_jobs
		where handler like '%%PublishPostsJob%%'
			and handler like E'%%\n  - %s\n%%'
	`, userId))
	require.Len(t, duplicateJobRows, 2)

	todayLocal3AMStr := todayLocal.Add(3 * time.Hour).Format(time.RFC3339)
	page = visitAdminf(browser, "travel_to?timestamp=%s", todayLocal3AMStr)
	require.Equal(t, todayLocal3AMStr, pageText(page))
	page = visitAdmin(browser, "wait_for_publish_posts_job")
	require.Equal(t, "OK", pageText(page))

	// Assert 1 job for tomorrow and 1 published post
	// Postgres might need time to reflect the new publish posts job update on another connection
	pollCount := 0
	for {
		rescheduledJobRows := visitAdminSql(browser, fmt.Sprintf(`
			select id from delayed_jobs
			where handler like '%%PublishPostsJob%%'
				and handler like E'%%\n  - %s\n%%'
		`, userId))
		if len(rescheduledJobRows) == 1 {
			break
		}
		pollCount++
		require.Less(t, pollCount, 30)
		time.Sleep(100 * time.Millisecond)
	}

	publishedPostRows := visitAdminSql(browser, fmt.Sprintf(`
		select id from subscription_posts
		where subscription_id = %s
			and published_at_local_date = '2022-06-01'
	`, subscriptionId))
	require.Len(t, publishedPostRows, 1)

	// Cleanup
	page = visitAdmin(browser, "travel_back")
	serverTimeStr := pageText(page)
	serverTime, err := time.Parse(time.RFC3339, serverTimeStr)
	oops.RequireNoError(t, err)
	require.InDelta(t, time.Now().Unix(), serverTime.Unix(), 60)
	page = visitAdmin(browser, "reschedule_user_job")
	require.Equal(t, "OK", pageText(page))

	visitDev(browser, "logout")
}

func TestUpdateFromFeed(t *testing.T) {
	l := launcher.New().Headless(false)
	defer l.Cleanup()
	browserUrl := l.MustLaunch()

	browser := rod.New().ControlURL(browserUrl).MustConnect()
	defer browser.MustClose()

	page := visitDev(browser, "login")
	page.MustElement("#email").MustInput("test_pst@feedrewind.com")
	page.MustElement("#current-password").MustInput("tz123456")
	page.MustElementR("input", "Sign in").MustClick()
	page.MustWaitLoad()

	page = visitAdmin(browser, "destroy_user_subscriptions")
	require.Equal(t, "OK", pageText(page))

	page = visitDev(browser, "admin/add_blog")
	page.MustElement("#name").MustInput("1man")
	page.MustElement("#feed_url").MustInput("https://ilidemi.github.io/dummy-blogs/1man/rss.xml")
	page.MustElement("#url").MustInput("https://ilidemi.github.io/dummy-blogs/1man/")
	page.MustElement("#posts").MustInput(
		`https://ilidemi.github.io/dummy-blogs/1man/post2 post2
https://ilidemi.github.io/dummy-blogs/1man/post3 post3
https://ilidemi.github.io/dummy-blogs/1man/post4 post4
https://ilidemi.github.io/dummy-blogs/1man/post5 post5`,
	)
	page.MustElement("#update_action").MustSelect("update_from_feed_or_fail")
	page.MustElementR("input", "Save").MustClick()
	page.MustWaitLoad()
	addBlogText := page.MustElement("main").MustText()
	require.Contains(t, addBlogText, "Created \"1man\"")

	statusRows := visitAdminSql(browser, fmt.Sprintf(`
		select id, status from blogs
		where feed_url = 'https://ilidemi.github.io/dummy-blogs/1man/rss.xml' and
			version = %d
	`, models.BlogLatestVersion))
	require.Equal(t, "manually_inserted", statusRows[0]["status"])
	blogId := statusRows[0]["id"].(json.Number).String()

	deletedRows := visitAdminSql(browser, fmt.Sprintf(`
		delete from blog_discarded_feed_entries where blog_id = %s returning *
	`, blogId))
	require.Len(t, deletedRows, 1)

	page = visitDev(browser, "subscriptions/add")
	page.MustElement("#start_url").MustInput("https://ilidemi.github.io/dummy-blogs/1man/rss.xml")
	page.MustElementR("button", "Go").MustClick()

	page.MustElementR("input", "Continue").MustClick()

	page.MustElement("#wed_add").MustClick()
	page.MustElement("#save_button").MustClick()
	page.MustElement("#arrival_msg")

	page.MustElementR("a", "Manage").MustClick()

	publishedCountStr := page.MustElement("#published_count").MustText()
	numberSuffixRegex := regexp.MustCompile("[0-9]+$")
	totalCountStr := numberSuffixRegex.FindString(publishedCountStr)
	require.Equal(t, "5", totalCountStr)
}
