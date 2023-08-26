package routes

import (
	"feedrewind/crawler"
	hardcodedblogs "feedrewind/crawler/hardcoded_blogs"
	"feedrewind/db/pgw"
	"feedrewind/jobs"
	"feedrewind/log"
	"feedrewind/models"
	"feedrewind/routes/rutil"
	"feedrewind/templates"
	"feedrewind/util"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/pkg/errors"
)

type feedsData struct {
	StartUrl        string
	StartUrlEncoded string
	Feeds           []*models.StartFeed
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
	pc := models.NewProductEventContext(conn, r, productUserId)
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
		models.ProductEvent_MustEmitVisitAddPage(pc, path, userIsAnonymous, map[string]any{
			"blog_url": startUrl,
		})
		discoverFeedsResult, typedResult := onboarding_MustDiscoverFeeds(
			conn, startUrl, currentUser, productUserId,
		)
		models.ProductEvent_MustEmitDiscoverFeeds(pc, startUrl, typedResult, userIsAnonymous)
		var maybeUserId *models.UserId
		if currentUser != nil {
			maybeUserId = &currentUser.Id
		}
		err := models.TypedBlogUrl_Create(conn, typedUrl, startUrl, path, typedResult, maybeUserId)
		if err != nil {
			panic(err)
		}
		switch discoverResult := discoverFeedsResult.(type) {
		case *discoveredSubscription:
			models.ProductEvent_MustEmitCreateSubscription(pc, discoverResult.subscription, userIsAnonymous)
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
		models.ProductEvent_MustEmitVisitAddPage(pc, "/subscriptions/add", userIsAnonymous, nil)
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
	pc := models.NewProductEventContext(conn, r, productUserId)
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
	models.ProductEvent_MustEmitDiscoverFeeds(pc, startUrl, typedResult, userIsAnonymous)
	var maybeUserId *models.UserId
	if currentUser != nil {
		maybeUserId = &currentUser.Id
	}
	err := models.TypedBlogUrl_Create(conn, typedUrl, startUrl, "/", typedResult, maybeUserId)
	if err != nil {
		panic(err)
	}
	switch discoverResult := discoverFeedsResult.(type) {
	case *discoveredSubscription:
		models.ProductEvent_MustEmitCreateSubscription(pc, discoverResult.subscription, userIsAnonymous)
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
	pc := models.NewProductEventContext(conn, r, productUserId)
	userIsAnonymous := currentUser == nil

	var result feedsData

	typedUrl := util.EnsureParamStr(r, "start_url")
	startUrl := strings.TrimSpace(typedUrl)
	discoverFeedsResult, typedResult := onboarding_MustDiscoverFeeds(
		conn, startUrl, currentUser, productUserId,
	)
	models.ProductEvent_MustEmitDiscoverFeeds(pc, startUrl, typedResult, userIsAnonymous)
	var maybeUserId *models.UserId
	if currentUser != nil {
		maybeUserId = &currentUser.Id
	}
	err := models.TypedBlogUrl_Create(
		conn, typedUrl, startUrl, "/subscriptions/add", typedResult, maybeUserId,
	)
	if err != nil {
		panic(err)
	}
	switch discoverResult := discoverFeedsResult.(type) {
	case *discoveredSubscription:
		models.ProductEvent_MustEmitCreateSubscription(pc, discoverResult.subscription, userIsAnonymous)
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
	subscription *models.SubscriptionCreateResult
}

type discoveredUnsupportedBlog struct {
	blog *models.Blog
}

type discoveredFeeds struct {
	feeds []*models.StartFeed
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
		blog, err := models.Blog_GetLatestByFeedUrl(tx, hardcodedblogs.OurMachinery)
		if errors.Is(err, models.ErrBlogNotFound) {
			panic("Our Machinery not found")
		} else if err != nil {
			panic(err)
		}
		subscription, err := models.Subscription_CreateForBlog(tx, blog, currentUser, productUserId)
		if errors.Is(err, models.ErrBlogFailed) {
			log.Info().Msg("Discover feeds for Our Machinery - unsupported blog")
			return &discoveredUnsupportedBlog{blog: blog}, models.TypedBlogUrlResultHardcoded
		} else if err != nil {
			panic(err)
		} else {
			log.Info().Msg("Discover feeds for Our Machinery - created subscription")
			return &discoveredSubscription{subscription: subscription}, models.TypedBlogUrlResultHardcoded
		}
	}

	crawlCtx := crawler.CrawlContext{}
	httpClient := crawler.HttpClient{
		EnableThrottling: false,
	}
	logger := crawler.ZeroLogger{}
	discoverFeedsResult := crawler.DiscoverFeedsAtUrl(startUrl, true, &crawlCtx, &httpClient, &logger)
	switch r := discoverFeedsResult.(type) {
	case *crawler.DiscoveredSingleFeed:
		log.Info().Msgf("Discover feeds at %s - found single feed", startUrl)
		var maybeStartPageId *models.StartPageId
		if r.MaybeStartPage != nil {
			startPageId, err := models.StartPage_Create(tx, *r.MaybeStartPage)
			if err != nil {
				panic(err)
			}
			maybeStartPageId = &startPageId
		}
		startFeed, err := models.StartFeed_CreateFetched(tx, maybeStartPageId, r.Feed)
		if err != nil {
			panic(err)
		}
		updatedBlog, err := models.Blog_CreateOrUpdate(tx, startFeed, jobs.GuidedCrawlingJob_PerformNow)
		if err != nil {
			panic(err)
		}
		subscription, err := models.Subscription_CreateForBlog(tx, updatedBlog, currentUser, productUserId)
		if errors.Is(err, models.ErrBlogFailed) {
			log.Info().Msgf("Discover feeds at %s - unsupported blog", startUrl)
			return &discoveredUnsupportedBlog{blog: updatedBlog}, models.TypedBlogUrlResultKnownUnsupported
		} else if err != nil {
			panic(err)
		} else {
			log.Info().Msgf("Discover feeds at %s - created subscription", startUrl)
			return &discoveredSubscription{subscription: subscription}, models.TypedBlogUrlResultFeed
		}
	case *crawler.DiscoveredMultipleFeeds:
		log.Info().Msgf("Discover feeds at %s - found %d feeds", startUrl, len(r.Feeds))
		startPageId, err := models.StartPage_Create(tx, r.StartPage)
		if err != nil {
			panic(err)
		}
		var startFeeds []*models.StartFeed
		for _, discoveredFeed := range r.Feeds {
			startFeed, err := models.StartFeed_Create(tx, startPageId, discoveredFeed)
			if err != nil {
				panic(err)
			}
			startFeeds = append(startFeeds, startFeed)
		}
		return &discoveredFeeds{feeds: startFeeds}, models.TypedBlogUrlResultPageWithMultipleFeeds
	case *crawler.DiscoverFeedsErrorNotAUrl:
		log.Info().Msgf("Discover feeds at %s - not a url", startUrl)
		return &discoverError{}, models.TypedBlogUrlResultNotAUrl
	case *crawler.DiscoverFeedsErrorCouldNotReach:
		log.Info().Msgf("Discover feeds at %s - could not reach", startUrl)
		return &discoverError{}, models.TypedBlogUrlResultCouldNotReach
	case *crawler.DiscoverFeedsErrorNoFeeds:
		log.Info().Msgf("Discover feeds at %s - no feeds", startUrl)
		return &discoverError{}, models.TypedBlogUrlResultNoFeeds
	case *crawler.DiscoverFeedsErrorBadFeed:
		log.Info().Msgf("Discover feeds at %s - bad feed", startUrl)
		return &discoverError{}, models.TypedBlogUrlResultBadFeed
	default:
		panic("unknown discover feeds result type")
	}
}
