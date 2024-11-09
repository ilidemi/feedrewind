//go:build e2etesting

package e2etest

import (
	"encoding/json"
	"fmt"
	"regexp"
	"testing"

	"feedrewind.com/models"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/stretchr/testify/require"
)

func TestUpdateFromFeedAndDelete(t *testing.T) {
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
	require.Equal(t, "OK", mustPageText(page))

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

	page.MustElement("#delete_button").MustClick()
	page.MustElement("#delete_popup_keep_button").MustClick()
	page.MustWaitDOMStable()
	page.MustElement("#delete_button").MustClick()
	page.MustElement("#delete_popup_delete_button").MustClick()
	page.MustElement("#no_subscriptions")
}

func TestSubscriptionDeleteSettingUp(t *testing.T) {
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
	require.Equal(t, "OK", mustPageText(page))

	page = visitDev(browser, "subscriptions/add")
	page.MustElement("#start_url").MustInput("https://ilidemi.github.io/dummy-blogs/1man/rss.xml")
	page.MustElementR("button", "Go").MustClick()

	page.MustElementR("input", "Continue").MustClick()

	subscriptionId := mustParseSubscriptionId(page)
	page = visitDev(browser, "subscriptions")
	page.MustElement("#delete_button_" + subscriptionId).MustClick()
	page.MustElement("#delete_popup_keep_button").MustClick()
	page.MustWaitDOMStable()
	page.MustElement("#delete_button_" + subscriptionId).MustClick()
	page.MustElement("#delete_popup_delete_button").MustClick()
	page.MustWaitDOMStable()
	page.MustElement("#no_subscriptions")
}
