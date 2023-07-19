package routes

import (
	"feedrewind/crawler"
	hardcodedblogs "feedrewind/crawler/hardcoded_blogs"
	"feedrewind/db/pgw"
	"feedrewind/jobs"
	"feedrewind/models"
	"feedrewind/routes/rutil"
	"feedrewind/templates"
	"feedrewind/util"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func Onboarding_Add(w http.ResponseWriter, r *http.Request) {
	conn := rutil.DBConn(r)
	currentUser := rutil.CurrentUser(r)
	productUserId := rutil.CurrentProductUserId(r)
	userIsAnonymous := currentUser == nil

	type feedsData struct {
		StartUrl        string
		StartUrlEncoded string
		Feeds           []crawler.DiscoveredFeed
		IsNotAUrl       bool
		AreNoFeeds      bool
		CouldNotReach   bool
		IsBadFeed       bool
	}
	type onboardingResult struct {
		Session     *util.Session
		FeedsData   *feedsData
		Suggestions *rutil.Suggestions
	}
	var result onboardingResult

	startUrls, ok := r.Form["start_url"]
	if ok {
		typedUrl := startUrls[0]
		startUrl := strings.TrimSpace(typedUrl)
		startUrlEncoded := url.QueryEscape(startUrl)
		path := "/subscriptions/add?start_url="
		models.ProductEvent_MustEmitVisitAddPage(models.ProductEventVisitAddPageArgs{
			Tx:              conn,
			Request:         r,
			ProductUserId:   productUserId,
			Path:            path,
			UserIsAnonymous: userIsAnonymous,
			Extra: map[string]any{
				"blog_url": startUrl,
			},
		})
		discoverFeedsResult, typedResult := onboarding_MustDiscoverFeeds(
			conn, startUrl, currentUser, productUserId,
		)
		models.ProductEvent_MustEmitDiscoverFeeds(models.ProductEventDiscoverFeedArgs{
			Tx:              conn,
			Request:         r,
			ProductUserId:   productUserId,
			BlogUrl:         startUrl,
			Result:          typedResult,
			UserIsAnonymous: userIsAnonymous,
		})
		var userId *models.UserId
		if currentUser != nil {
			userId = &currentUser.Id
		}
		models.TypedBlogUrl_MustCreate(conn, typedUrl, startUrl, path, typedResult, userId)
		switch discoverResult := discoverFeedsResult.(type) {
		case *discoveredSubscription:
			models.ProductEvent_MustEmitCreateSubscription(models.ProductEventCreateSubscriptionArgs{
				Tx:              conn,
				Request:         r,
				ProductUserId:   productUserId,
				Subscription:    discoverResult.subscription,
				UserIsAnonymous: userIsAnonymous,
			})
			http.Redirect(w, r, rutil.SubscriptionSetupPath(discoverResult.subscription.Id), http.StatusFound)
			return
		case *discoveredUnsupportedBlog:
			http.Redirect(w, r, rutil.BlogUnsupportedPath(discoverResult.blog.Id), http.StatusFound)
			return
		case *discoveredFeeds:
			result = onboardingResult{
				Session: rutil.Session(r),
				FeedsData: &feedsData{ //nolint:exhaustruct
					StartUrl:        startUrl,
					StartUrlEncoded: startUrlEncoded,
					Feeds:           discoverResult.feeds,
				},
				Suggestions: nil,
			}
		case *discoverError:
			var feeds *feedsData
			switch typedResult {
			case models.TypedBlogUrlResultNotAUrl:
				feeds = &feedsData{ //nolint:exhaustruct
					StartUrl:        startUrl,
					StartUrlEncoded: startUrlEncoded,
					IsNotAUrl:       true,
				}
			case models.TypedBlogUrlResultNoFeeds:
				feeds = &feedsData{ //nolint:exhaustruct
					StartUrl:        startUrl,
					StartUrlEncoded: startUrlEncoded,
					AreNoFeeds:      true,
				}
			case models.TypedBlogUrlResultCouldNotReach:
				feeds = &feedsData{ //nolint:exhaustruct
					StartUrl:        startUrl,
					StartUrlEncoded: startUrlEncoded,
					CouldNotReach:   true,
				}
			case models.TypedBlogUrlResultBadFeed:
				feeds = &feedsData{ //nolint:exhaustruct
					StartUrl:        startUrl,
					StartUrlEncoded: startUrlEncoded,
					IsBadFeed:       true,
				}
			default:
				panic(fmt.Errorf("unknown typed result: %s", typedResult))
			}
			result = onboardingResult{
				Session:     rutil.Session(r),
				FeedsData:   feeds,
				Suggestions: nil,
			}
		default:
			panic("Unknown discover feeds result type")
		}
	} else {
		models.ProductEvent_MustEmitVisitAddPage(models.ProductEventVisitAddPageArgs{
			Tx:              conn,
			Request:         r,
			ProductUserId:   rutil.CurrentProductUserId(r),
			Path:            "/subscriptions/add",
			UserIsAnonymous: userIsAnonymous,
			Extra:           nil,
		})

		result = onboardingResult{
			Session:   rutil.Session(r),
			FeedsData: nil,
			Suggestions: &rutil.Suggestions{
				SuggestedCategories: rutil.SuggestedCategories,
				MiscellaneousBlogs:  rutil.MiscellaneousBlogs,
				WidthClass:          "max-w-full",
			},
		}
	}

	templates.MustWrite(w, "onboarding/add", result)
}

type discoveredSubscription struct {
	subscription models.SubscriptionCreateResult
}

type discoveredUnsupportedBlog struct {
	blog models.Blog
}

type discoveredFeeds struct {
	feeds []crawler.DiscoveredFeed
}

type discoverError struct{}

type discoverResult interface {
	discoverResultTag()
}

func (*discoveredSubscription) discoverResultTag()    {}
func (*discoveredUnsupportedBlog) discoverResultTag() {}
func (*discoveredFeeds) discoverResultTag()           {}
func (*discoverError) discoverResultTag()             {}

func onboarding_MustDiscoverFeeds(
	tx pgw.Queryable, startUrl string, currentUser *models.User, productUserId models.ProductUserId,
) (discoverResult, models.TypedBlogUrlResult) {
	if startUrl == hardcodedblogs.OurMachinery {
		blog, ok := models.Blog_MustGetLatestByFeedUrl(tx, hardcodedblogs.OurMachinery)
		if !ok {
			panic("Our Machinery not found")
		}
		subscription, ok := models.Subscription_MustCreateForBlog(tx, blog, currentUser, productUserId)
		if ok {
			return &discoveredSubscription{subscription: subscription}, models.TypedBlogUrlResultHardcoded
		} else {
			return &discoveredUnsupportedBlog{blog: blog}, models.TypedBlogUrlResultHardcoded
		}
	}

	crawlCtx := crawler.CrawlContext{}
	httpClient := crawler.HttpClient{
		EnableThrottling: false,
	}
	logger := crawler.ZeroLogger{}
	discoverFeedsResult := crawler.MustDiscoverFeedsAtUrl(startUrl, true, &crawlCtx, &httpClient, &logger)
	switch r := discoverFeedsResult.(type) {
	case *crawler.DiscoveredSingleFeed:
		startPageId := models.StartPage_MustCreate(tx, r.StartPage)
		startFeedId := models.StartFeed_MustCreateFetched(tx, startPageId, r.Feed)
		updatedBlog := models.Blog_MustCreateOrUpdate(
			tx, startFeedId, r.Feed, jobs.GuidedCrawlingJob_MustPerformNow,
		)
		subscription, ok := models.Subscription_MustCreateForBlog(tx, updatedBlog, currentUser, productUserId)
		if ok {
			return &discoveredSubscription{subscription: subscription}, models.TypedBlogUrlResultFeed
		} else {
			return &discoveredUnsupportedBlog{blog: updatedBlog}, models.TypedBlogUrlResultKnownUnsupported
		}
	case *crawler.DiscoveredMultipleFeeds:
		startPageId := models.StartPage_MustCreate(tx, r.StartPage)
		for _, discoveredFeed := range r.Feeds {
			models.StartFeed_MustCreate(tx, startPageId, discoveredFeed)
		}
		return &discoveredFeeds{feeds: r.Feeds}, models.TypedBlogUrlResultPageWithMultipleFeeds
	case *crawler.DiscoverFeedsErrorNotAUrl:
		return &discoverError{}, models.TypedBlogUrlResultNotAUrl
	case *crawler.DiscoverFeedsErrorCouldNotReach:
		return &discoverError{}, models.TypedBlogUrlResultCouldNotReach
	case *crawler.DiscoverFeedsErrorNoFeeds:
		return &discoverError{}, models.TypedBlogUrlResultNoFeeds
	case *crawler.DiscoverFeedsErrorBadFeed:
		return &discoverError{}, models.TypedBlogUrlResultBadFeed
	default:
		panic("unknown discover feeds result type")
	}
}
