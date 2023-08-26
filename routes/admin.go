package routes

import (
	"feedrewind/crawler"
	"feedrewind/models"
	"feedrewind/oops"
	"feedrewind/routes/rutil"
	"feedrewind/templates"
	"feedrewind/util"
	"fmt"
	"net/http"
	"strings"

	"github.com/pkg/errors"
)

func Admin_AddBlog(w http.ResponseWriter, r *http.Request) {
	type addBlogResult struct {
		Session *util.Session
	}
	templates.MustWrite(w, "admin/add_blog", addBlogResult{
		Session: rutil.Session(r),
	})
}

func Admin_PostBlog(w http.ResponseWriter, r *http.Request) {
	postBlogImpl := func() (blog *models.BlogWithCounts, err error) {
		name := util.EnsureParamStr(r, "name")

		feedUrl := util.EnsureParamStr(r, "feed_url")
		if !strings.HasPrefix(feedUrl, "http://") && !strings.HasPrefix(feedUrl, "https://") {
			return nil, oops.Newf("Feed url is supposed to be full: %s", feedUrl)
		}

		blogUrl := util.EnsureParamStr(r, "url")
		direction := util.EnsureParamStr(r, "direction")

		postUrlsTitles, err := admin_ParseUrlsLabels(util.EnsureParamStr(r, "posts"))
		if err != nil {
			return nil, err
		}
		if direction == "newest_first" {
			for i, j := 0, len(postUrlsTitles)-1; i < j; i, j = i+1, j-1 {
				postUrlsTitles[i], postUrlsTitles[j] = postUrlsTitles[j], postUrlsTitles[i]
			}
		}
		postUrlsSet := make(map[string]bool)
		for _, post := range postUrlsTitles {
			postUrlsSet[post.Url] = true
		}

		postUrlsCategories, err := admin_ParseUrlsLabels(util.EnsureParamStr(r, "post_categories"))
		if err != nil {
			return nil, err
		}
		for _, urlCategory := range postUrlsCategories {
			if !postUrlsSet[urlCategory.Url] {
				return nil, oops.Newf("Unknown categorized url: %s", urlCategory.Url)
			}
		}

		topCategoriesStr := util.EnsureParamStr(r, "top_categories")
		var topCategories []string
		if topCategoriesStr != "" {
			topCategories = strings.Split(topCategoriesStr, ";")
		}
		postCategories := make([]string, len(topCategories))
		copy(postCategories, topCategories)
		topCategoriesSet := make(map[string]bool)
		postCategoriesSet := make(map[string]bool)
		for _, topCategory := range topCategories {
			topCategoriesSet[topCategory] = true
			postCategoriesSet[topCategory] = true
		}
		for _, urlCategory := range postUrlsCategories {
			if postCategoriesSet[urlCategory.Label] {
				continue
			}
			postCategories = append(postCategories, urlCategory.Label)
			postCategoriesSet[urlCategory.Label] = true
		}

		topCategoriesSet["Everything"] = true
		postCategories = append(postCategories, "Everything")
		postCategoriesSet["Everything"] = true
		for postUrl := range postUrlsSet {
			postUrlsCategories = append(postUrlsCategories, urlLabel{
				Url:   postUrl,
				Label: "Everything",
			})
		}

		sameHosts := strings.Split(util.EnsureParamStr(r, "same_hosts"), "\n")
		sameHostsSet := make(map[string]bool)
		for i := range sameHosts {
			sameHostsSet[strings.TrimSpace(sameHosts[i])] = true
		}
		expectTumblrPaths := util.EnsureParamStr(r, "expect_tumblr_paths") == "1"
		curiEqCfg := crawler.CanonicalEqualityConfig{
			SameHosts:         sameHostsSet,
			ExpectTumblrPaths: expectTumblrPaths,
		}

		updateAction := models.BlogUpdateAction(util.EnsureParamStr(r, "update_action"))

		var postLinks []*crawler.Link
		var postCuris []crawler.CanonicalUri
		logger := crawler.ZeroLogger{}
		for _, urlTitle := range postUrlsTitles {
			postLink, ok := crawler.ToCanonicalLink(urlTitle.Url, &logger, nil)
			if !ok {
				return nil, oops.Newf("Bad post link: %s", urlTitle.Url)
			}
			postLinks = append(postLinks, postLink)
			postCuris = append(postCuris, postLink.Curi)
		}
		postCurisSet := crawler.NewCanonicalUriSet(postCuris, &curiEqCfg)

		var discardedFromFeedEntryUrls []string
		var missingFromFeedEntryUrls []string
		if util.EnsureParamStr(r, "skip_feed_validation") != "1" {
			crawlCtx := crawler.CrawlContext{}
			httpClient := crawler.HttpClient{EnableThrottling: false}
			feedResult := crawler.FetchFeedAtUrl(feedUrl, false, &crawlCtx, &httpClient, &logger)
			feedPage, ok := feedResult.(*crawler.FetchedPage)
			if !ok {
				return nil, oops.Newf("Couldn't fetch feed: %s", feedUrl)
			}

			feedLink, ok := crawler.ToCanonicalLink(feedUrl, &logger, nil)
			if !ok {
				return nil, oops.Newf("Bad feed url: %s", feedUrl)
			}

			parsedFeed, err := crawler.ParseFeed(feedPage.Page.Content, feedLink.Uri, &logger)
			if err != nil {
				return nil, err
			}

			feedEntryLinks := parsedFeed.EntryLinks.ToSlice()
			for _, entryLink := range feedEntryLinks {
				if !postCurisSet.Contains(entryLink.Curi) {
					discardedFromFeedEntryUrls = append(discardedFromFeedEntryUrls, entryLink.Url)
				}
			}

			var feedCuris []crawler.CanonicalUri
			for _, entryLink := range feedEntryLinks {
				feedCuris = append(feedCuris, entryLink.Curi)
			}
			feedCurisSet := crawler.NewCanonicalUriSet(feedCuris, &curiEqCfg)

			oldestFeedPostIndex := 0
			oldestFeedEntryCuri := feedEntryLinks[len(feedEntryLinks)-1].Curi
			for i, postLink := range postLinks {
				if crawler.CanonicalUriEqual(postLink.Curi, oldestFeedEntryCuri, &curiEqCfg) {
					oldestFeedPostIndex = i
				}
			}
			for _, postLink := range postLinks[oldestFeedPostIndex:] {
				if !feedCurisSet.Contains(postLink.Curi) {
					missingFromFeedEntryUrls = append(missingFromFeedEntryUrls, postLink.Url)
				}
			}
		}

		conn := rutil.DBConn(r)
		tx, err := conn.Begin()
		if err != nil {
			return nil, err
		}
		defer util.CommitOrRollbackErr(tx, &err)

		oldBlog, err := models.Blog_GetLatestByFeedUrl(tx, feedUrl)
		if !errors.Is(err, models.ErrBlogNotFound) && err != nil {
			return nil, err
		} else if err == nil {
			err = models.Blog_Downgrade(tx, oldBlog.Id)
			if err != nil {
				return nil, err
			}
		}

		blogId, err := models.Blog_Create(
			tx, name, feedUrl, blogUrl, models.BlogStatusManuallyInserted, models.BlogLatestVersion, updateAction,
		)
		if err != nil {
			return nil, err
		}

		var newPosts []models.NewBlogPost
		for i, urlTitle := range postUrlsTitles {
			newPosts = append(newPosts, models.NewBlogPost{
				Index: int32(i),
				Url:   urlTitle.Url,
				Title: urlTitle.Label,
			})
		}
		createdPosts, err := models.BlogPost_CreateMany(tx, blogId, newPosts)
		if err != nil {
			return nil, err
		}
		blogPostIdsByUrl := make(map[string][]models.BlogPostId)
		for _, post := range createdPosts {
			blogPostIdsByUrl[post.Url] = append(blogPostIdsByUrl[post.Url], post.Id)
		}

		var newCategories []models.NewBlogPostCategory
		for i, name := range postCategories {
			newCategories = append(newCategories, models.NewBlogPostCategory{
				Name:  name,
				Index: int32(i),
				IsTop: topCategoriesSet[name],
			})
		}
		createdCategories, err := models.BlogPostCategory_CreateMany(tx, blogId, newCategories)
		if err != nil {
			return nil, err
		}
		categoryIdsByName := make(map[string]models.BlogPostCategoryId)
		for _, category := range createdCategories {
			categoryIdsByName[category.Name] = category.Id
		}

		var newAssignments []models.NewBlogPostCategoryAssignment
		for _, urlCategory := range postUrlsCategories {
			for _, blogPostId := range blogPostIdsByUrl[urlCategory.Url] {
				newAssignments = append(newAssignments, models.NewBlogPostCategoryAssignment{
					BlogPostId: blogPostId,
					CategoryId: categoryIdsByName[urlCategory.Label],
				})
			}
		}
		err = models.BlogPostCategoryAssignment_CreateMany(tx, newAssignments)
		if err != nil {
			return nil, err
		}

		err = models.BlogPostLock_Create(tx, blogId)
		if err != nil {
			return nil, err
		}

		err = models.BlogCanonicalEqualityConfig_Create(tx, blogId, sameHosts, expectTumblrPaths)
		if err != nil {
			return nil, err
		}

		err = models.BlogDiscardedFeedEntry_CreateMany(tx, blogId, discardedFromFeedEntryUrls)
		if err != nil {
			return nil, err
		}

		err = models.BlogMissingFromFeedEntry_CreateMany(tx, blogId, missingFromFeedEntryUrls)
		if err != nil {
			return nil, err
		}

		blog, err = models.Blog_GetWithCounts(tx, blogId)
		return blog, err
	}

	var message string
	blog, err := postBlogImpl()
	if err != nil {
		message = err.Error()
	} else {
		message = fmt.Sprintf(
			"Created %q (%s) with %d posts and %d categories",
			blog.Name, blog.FeedUrl, blog.PostsCount, blog.CategoriesCount,
		)
	}

	type postBlogResult struct {
		Session *util.Session
		Message string
	}
	result := postBlogResult{
		Session: rutil.Session(r),
		Message: message,
	}
	templates.MustWrite(w, "admin/post_blog", result)
}

type urlLabel struct {
	Url   string
	Label string
}

func admin_ParseUrlsLabels(text string) ([]urlLabel, error) {
	if text == "" {
		return nil, nil
	}

	lines := strings.Split(text, "\n")
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	var urlsLabels []urlLabel
	for i, line := range lines {
		if !strings.HasPrefix(line, "http://") && !strings.HasPrefix(line, "https://") {
			return nil, oops.Newf("Line %d doesn't start with a full url: %s", i+1, line)
		}
		if !strings.Contains(line, " ") {
			return nil, oops.Newf("Line %d doesn't have a space between url and title: %s", i+1, line)
		}
		tokens := strings.SplitN(line, " ", 2)
		urlsLabels = append(urlsLabels, urlLabel{
			Url:   tokens[0],
			Label: tokens[1],
		})
	}
	return urlsLabels, nil
}
