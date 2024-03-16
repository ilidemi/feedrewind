package routes

import (
	"feedrewind/crawler"
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

	type OnboardingResult struct {
		Title            string
		Session          *util.Session
		MaybeFeedsData   *feedsData
		MaybeSuggestions *util.Suggestions
	}
	var result OnboardingResult
	title := util.DecorateTitle("Add blog")

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
			result = OnboardingResult{
				Title:   title,
				Session: rutil.Session(r),
				MaybeFeedsData: &feedsData{ //nolint:exhaustruct
					StartUrl: startUrl,
					Feeds:    discoverResult.feeds,
				},
				MaybeSuggestions: nil,
			}
		case *discoverError:
			feeds := feedsDataFromTypedResult(startUrl, typedResult)
			result = OnboardingResult{
				Title:            title,
				Session:          rutil.Session(r),
				MaybeFeedsData:   &feeds,
				MaybeSuggestions: nil,
			}
		default:
			panic("Unknown discover feeds result type")
		}
	} else {
		models.ProductEvent_MustEmitVisitAddPage(pc, "/subscriptions/add", userIsAnonymous, nil)
		result = OnboardingResult{
			Title:          title,
			Session:        rutil.Session(r),
			MaybeFeedsData: nil,
			MaybeSuggestions: &util.Suggestions{
				Session:             rutil.Session(r),
				SuggestedCategories: util.SuggestedCategories,
				MiscellaneousBlogs:  util.MiscellaneousBlogs,
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

	type OnboardingResult struct {
		Title            string
		Session          *util.Session
		MaybeFeedsData   *feedsData
		MaybeSuggestions *util.Suggestions
	}
	var result OnboardingResult
	title := util.DecorateTitle("Add blog")

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
		result = OnboardingResult{
			Title:   title,
			Session: rutil.Session(r),
			MaybeFeedsData: &feedsData{ //nolint:exhaustruct
				StartUrl: startUrl,
				Feeds:    discoverResult.feeds,
			},
			MaybeSuggestions: nil,
		}
	case *discoverError:
		feeds := feedsDataFromTypedResult(startUrl, typedResult)
		result = OnboardingResult{
			Title:            title,
			Session:          rutil.Session(r),
			MaybeFeedsData:   &feeds,
			MaybeSuggestions: nil,
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
		util.MustWrite(w, rutil.SubscriptionSetupPath(discoverResult.subscription.Id))
		return
	case *discoveredUnsupportedBlog:
		util.MustWrite(w, rutil.BlogUnsupportedPath(discoverResult.blog.Id))
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
	link, ok := util.ScreenshotLinksBySlug[slug]
	if !ok {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	type Result struct {
		Title       string
		Session     *util.Session
		Url         string
		AddFeedPath string
	}
	templates.MustWrite(w, "onboarding/preview", Result{
		Title:       link.TitleStr,
		Session:     rutil.Session(r),
		Url:         link.Url,
		AddFeedPath: rutil.SubscriptionAddFeedPath(link.FeedUrl),
	})
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
	logger := tx.Logger()
	if startUrl == crawler.HardcodedOurMachinery || startUrl == crawler.HardcodedSequences {
		blog, err := models.Blog_GetLatestByFeedUrl(tx, startUrl)
		if errors.Is(err, models.ErrBlogNotFound) {
			panic(fmt.Errorf("Blog not found for %s", startUrl))
		} else if err != nil {
			panic(err)
		}
		subscription, err := models.Subscription_CreateForBlog(tx, blog, currentUser, productUserId)
		if errors.Is(err, models.ErrBlogFailed) {
			logger.Info().Msgf("Discover feeds for %s - unsupported blog", startUrl)
			return &discoveredUnsupportedBlog{blog: blog}, models.TypedBlogUrlResultHardcoded
		} else if err != nil {
			panic(err)
		} else {
			logger.Info().Msgf("Discover feeds for %s - created subscription", startUrl)
			return &discoveredSubscription{subscription: subscription}, models.TypedBlogUrlResultHardcoded
		}
	}

	httpClient := crawler.NewHttpClientImpl(tx.Context(), false)
	zlogger := crawler.ZeroLogger{Logger: logger}
	progressLogger := crawler.NewMockProgressLogger(&zlogger)
	crawlCtx := crawler.NewCrawlContext(httpClient, nil, &progressLogger)
	discoverFeedsResult := crawler.DiscoverFeedsAtUrl(startUrl, true, &crawlCtx, &zlogger)
	switch result := discoverFeedsResult.(type) {
	case *crawler.DiscoveredSingleFeed:
		logger.Info().Msgf("Discover feeds at %s - found single feed", startUrl)
		var maybeStartPageId *models.StartPageId
		if result.MaybeStartPage != nil {
			startPageId, err := models.StartPage_Create(tx, *result.MaybeStartPage)
			if err != nil {
				panic(err)
			}
			maybeStartPageId = &startPageId
		}
		startFeed, err := models.StartFeed_CreateFetched(tx, maybeStartPageId, result.Feed)
		if err != nil {
			panic(err)
		}
		updatedBlog, err := models.Blog_CreateOrUpdate(tx, startFeed, jobs.GuidedCrawlingJob_PerformNow)
		if err != nil {
			panic(err)
		}
		subscription, err := models.Subscription_CreateForBlog(tx, updatedBlog, currentUser, productUserId)
		if errors.Is(err, models.ErrBlogFailed) {
			logger.Info().Msgf("Discover feeds at %s - unsupported blog", startUrl)
			return &discoveredUnsupportedBlog{blog: updatedBlog}, models.TypedBlogUrlResultKnownUnsupported
		} else if err != nil {
			panic(err)
		} else {
			logger.Info().Msgf("Discover feeds at %s - created subscription", startUrl)
			return &discoveredSubscription{subscription: subscription}, models.TypedBlogUrlResultFeed
		}
	case *crawler.DiscoveredMultipleFeeds:
		logger.Info().Msgf("Discover feeds at %s - found %d feeds", startUrl, len(result.Feeds))
		startPageId, err := models.StartPage_Create(tx, result.StartPage)
		if err != nil {
			panic(err)
		}
		var startFeeds []*models.StartFeed
		for _, discoveredFeed := range result.Feeds {
			startFeed, err := models.StartFeed_Create(tx, startPageId, discoveredFeed)
			if err != nil {
				panic(err)
			}
			startFeeds = append(startFeeds, startFeed)
		}
		return &discoveredFeeds{feeds: startFeeds}, models.TypedBlogUrlResultPageWithMultipleFeeds
	case *crawler.DiscoverFeedsErrorNotAUrl:
		logger.Info().Msgf("Discover feeds at %s - not a url", startUrl)
		return &discoverError{}, models.TypedBlogUrlResultNotAUrl
	case *crawler.DiscoverFeedsErrorCouldNotReach:
		logger.Info().Msgf("Discover feeds at %s - could not reach", startUrl)
		return &discoverError{}, models.TypedBlogUrlResultCouldNotReach
	case *crawler.DiscoverFeedsErrorNoFeeds:
		logger.Info().Msgf("Discover feeds at %s - no feeds", startUrl)
		return &discoverError{}, models.TypedBlogUrlResultNoFeeds
	case *crawler.DiscoverFeedsErrorBadFeed:
		logger.Info().Msgf("Discover feeds at %s - bad feed", startUrl)
		return &discoverError{}, models.TypedBlogUrlResultBadFeed
	default:
		panic("unknown discover feeds result type")
	}
}
