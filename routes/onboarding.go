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

type feedsData struct {
	StartUrl        string
	StartUrlEncoded string
	Feeds           []models.StartFeed
	IsNotAUrl       bool
	AreNoFeeds      bool
	CouldNotReach   bool
	IsBadFeed       bool
}

func feedsDataFromTypedResult(startUrl string, typedResult models.TypedBlogUrlResult) feedsData {
	switch typedResult {
	case models.TypedBlogUrlResultNotAUrl:
		startUrlEncoded := url.QueryEscape(startUrl)
		return feedsData{ //nolint:exhaustruct
			StartUrl:        startUrl,
			StartUrlEncoded: startUrlEncoded,
			IsNotAUrl:       true,
		}
	case models.TypedBlogUrlResultNoFeeds:
		return feedsData{ //nolint:exhaustruct
			StartUrl:   startUrl,
			AreNoFeeds: true,
		}
	case models.TypedBlogUrlResultCouldNotReach:
		return feedsData{ //nolint:exhaustruct
			StartUrl:      startUrl,
			CouldNotReach: true,
		}
	case models.TypedBlogUrlResultBadFeed:
		return feedsData{ //nolint:exhaustruct
			StartUrl:  startUrl,
			IsBadFeed: true,
		}
	default:
		panic(fmt.Errorf("unknown typed result: %s", typedResult))
	}
}

func Onboarding_Add(w http.ResponseWriter, r *http.Request) {
	conn := rutil.DBConn(r)
	currentUser := rutil.CurrentUser(r)
	productUserId := rutil.CurrentProductUserId(r)
	userIsAnonymous := currentUser == nil

	type onboardingResult struct {
		Session     *util.Session
		FeedsData   *feedsData
		Suggestions *rutil.Suggestions
	}
	var result onboardingResult

	escapedUrl := util.URLParamStr(r, "start_url")
	typedUrl, err := url.PathUnescape(escapedUrl)
	if err != nil {
		typedUrl = escapedUrl
	}
	if typedUrl != "" {
		startUrl := strings.TrimSpace(typedUrl)
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
		var maybeUserId *models.UserId
		if currentUser != nil {
			maybeUserId = &currentUser.Id
		}
		models.TypedBlogUrl_MustCreate(conn, typedUrl, startUrl, path, typedResult, maybeUserId)
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
					StartUrl: startUrl,
					Feeds:    discoverResult.feeds,
				},
				Suggestions: nil,
			}
		case *discoverError:
			feeds := feedsDataFromTypedResult(startUrl, typedResult)
			result = onboardingResult{
				Session:     rutil.Session(r),
				FeedsData:   &feeds,
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

func Onboarding_AddLanding(w http.ResponseWriter, r *http.Request) {
	conn := rutil.DBConn(r)
	currentUser := rutil.CurrentUser(r)
	productUserId := rutil.CurrentProductUserId(r)
	userIsAnonymous := currentUser == nil

	type onboardingResult struct {
		Session     *util.Session
		FeedsData   *feedsData
		Suggestions *rutil.Suggestions
	}
	var result onboardingResult

	typedUrl := util.EnsureParamStr(r, "start_url")
	startUrl := strings.TrimSpace(typedUrl)
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
	var maybeUserId *models.UserId
	if currentUser != nil {
		maybeUserId = &currentUser.Id
	}
	models.TypedBlogUrl_MustCreate(conn, typedUrl, startUrl, "/", typedResult, maybeUserId)
	switch discoverResult := discoverFeedsResult.(type) {
	case *discoveredSubscription:
		models.ProductEvent_MustEmitCreateSubscription(models.ProductEventCreateSubscriptionArgs{
			Tx:              conn,
			Request:         r,
			ProductUserId:   productUserId,
			Subscription:    discoverResult.subscription,
			UserIsAnonymous: userIsAnonymous,
		})
		redirectPath := rutil.SubscriptionSetupPath(discoverResult.subscription.Id)
		http.Redirect(w, r, redirectPath, http.StatusFound)
		return
	case *discoveredUnsupportedBlog:
		redirectPath := rutil.BlogUnsupportedPath(discoverResult.blog.Id)
		http.Redirect(w, r, redirectPath, http.StatusFound)
		return
	case *discoveredFeeds:
		result = onboardingResult{
			Session: rutil.Session(r),
			FeedsData: &feedsData{ //nolint:exhaustruct
				StartUrl: startUrl,
				Feeds:    discoverResult.feeds,
			},
			Suggestions: nil,
		}
	case *discoverError:
		feeds := feedsDataFromTypedResult(startUrl, typedResult)
		result = onboardingResult{
			Session:     rutil.Session(r),
			FeedsData:   &feeds,
			Suggestions: nil,
		}
	default:
		panic("Unknown discover feeds result type")
	}

	templates.MustWrite(w, "onboarding/add", result)
}

func Onboarding_DiscoverFeeds(w http.ResponseWriter, r *http.Request) {
	conn := rutil.DBConn(r)
	currentUser := rutil.CurrentUser(r)
	productUserId := rutil.CurrentProductUserId(r)
	userIsAnonymous := currentUser == nil

	var result feedsData

	typedUrl := util.EnsureParamStr(r, "start_url")
	startUrl := strings.TrimSpace(typedUrl)
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
	var maybeUserId *models.UserId
	if currentUser != nil {
		maybeUserId = &currentUser.Id
	}
	models.TypedBlogUrl_MustCreate(conn, typedUrl, startUrl, "/subscriptions/add", typedResult, maybeUserId)
	switch discoverResult := discoverFeedsResult.(type) {
	case *discoveredSubscription:
		models.ProductEvent_MustEmitCreateSubscription(models.ProductEventCreateSubscriptionArgs{
			Tx:              conn,
			Request:         r,
			ProductUserId:   productUserId,
			Subscription:    discoverResult.subscription,
			UserIsAnonymous: userIsAnonymous,
		})
		_, err := w.Write([]byte(rutil.SubscriptionSetupPath(discoverResult.subscription.Id)))
		if err != nil {
			panic(err)
		}
		return
	case *discoveredUnsupportedBlog:
		_, err := w.Write([]byte(rutil.BlogUnsupportedPath(discoverResult.blog.Id)))
		if err != nil {
			panic(err)
		}
		return
	case *discoveredFeeds:
		result = feedsData{ //nolint:exhaustruct
			StartUrl: startUrl,
			Feeds:    discoverResult.feeds,
		}
	case *discoverError:
		result = feedsDataFromTypedResult(startUrl, typedResult)
	default:
		panic("Unknown discover feeds result type")
	}

	templates.MustWrite(w, "onboarding/partial_feeds", result)
}

func Onboarding_Preview(w http.ResponseWriter, r *http.Request) {
	slug := util.URLParamStr(r, "slug")
	link, ok := rutil.ScreenshotLinksBySlug[slug]
	if !ok {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	type previewResult struct {
		Session     *util.Session
		Title       string
		Url         string
		AddFeedPath string
	}
	result := previewResult{
		Session:     rutil.Session(r),
		Title:       link.TitleStr,
		Url:         link.Url,
		AddFeedPath: rutil.SubscriptionAddFeedPath(link.FeedUrl),
	}
	templates.MustWrite(w, "onboarding/preview", result)
}

type discoveredSubscription struct {
	subscription models.SubscriptionCreateResult
}

type discoveredUnsupportedBlog struct {
	blog models.Blog
}

type discoveredFeeds struct {
	feeds []models.StartFeed
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
		startFeed := models.StartFeed_MustCreateFetched(tx, startPageId, r.Feed)
		updatedBlog := models.Blog_MustCreateOrUpdate(tx, startFeed, jobs.GuidedCrawlingJob_MustPerformNow)
		subscription, ok := models.Subscription_MustCreateForBlog(tx, updatedBlog, currentUser, productUserId)
		if ok {
			return &discoveredSubscription{subscription: subscription}, models.TypedBlogUrlResultFeed
		} else {
			return &discoveredUnsupportedBlog{blog: updatedBlog}, models.TypedBlogUrlResultKnownUnsupported
		}
	case *crawler.DiscoveredMultipleFeeds:
		startPageId := models.StartPage_MustCreate(tx, r.StartPage)
		var startFeeds []models.StartFeed
		for _, discoveredFeed := range r.Feeds {
			startFeed := models.StartFeed_MustCreate(tx, startPageId, discoveredFeed)
			startFeeds = append(startFeeds, startFeed)
		}
		return &discoveredFeeds{feeds: startFeeds}, models.TypedBlogUrlResultPageWithMultipleFeeds
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
