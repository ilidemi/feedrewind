//go:build e2etesting

package e2etest

import (
	"encoding/json"
	"feedrewind/models"
	"fmt"
	"regexp"
	"testing"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/stretchr/testify/require"
)

func TestUpdateFromFeed(t *testing.T) {
	l := launcher.New().Headless(false)
	defer l.Cleanup()
	browserUrl := l.MustLaunch()

	browser := rod.New().ControlURL(browserUrl).MustConnect()
	defer browser.MustClose()

	page := browser.MustPage(devUrl("login"))
	page.MustElement("#email").MustInput("test_pst@test.com")
	page.MustElement("#current-password").MustInput("tz123456")
	page.MustElementR("input", "Sign in").MustClick()
	page.MustWaitDOMStable()

	page = browser.MustPage(adminUrl("destroy_user_subscriptions"))
	require.Equal(t, "OK", page.MustElement("body").MustText())

	page = browser.MustPage(devUrl("admin/add_blog"))
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
	page.MustWaitDOMStable()
	addBlogText := page.MustElement("main").MustText()
	require.Contains(t, addBlogText, "Created \"1man\"")

	statusRows := adminSql(browser, fmt.Sprintf(`
		select id, status from blogs
		where feed_url = 'https://ilidemi.github.io/dummy-blogs/1man/rss.xml' and
			version = %d
	`, models.BlogLatestVersion))
	require.Equal(t, "manually_inserted", statusRows[0]["status"])
	blogId := statusRows[0]["id"].(json.Number).String()

	deletedRows := adminSql(browser, fmt.Sprintf(`
		delete from blog_discarded_feed_entries where blog_id = %s returning *
	`, blogId))
	require.Len(t, deletedRows, 1)

	page = browser.MustPage(devUrl("subscriptions/add"))
	page.MustElement("#start_url").MustInput("https://ilidemi.github.io/dummy-blogs/1man/rss.xml")
	page.MustElementR("button", "Go").MustClick()

	page.MustElementR("input", "Continue").MustClick()

	page.MustWaitDOMStable() // Javascript
	page.MustElement("#wed_add").MustClick()
	page.MustWaitDOMStable() // Javascript
	page.MustElementR("button", "Continue").MustClick()
	page.MustElement("#arrival_msg")

	page.MustElementR("a", "Manage").MustClick()

	publishedCountStr := page.MustElement("#published_count").MustText()
	numberSuffixRegex := regexp.MustCompile("[0-9]+$")
	totalCountStr := numberSuffixRegex.FindString(publishedCountStr)
	require.Equal(t, "5", totalCountStr)
}
