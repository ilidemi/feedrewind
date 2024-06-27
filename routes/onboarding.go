package routes

import (
	"feedrewind/config"
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
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pkg/errors"
	"github.com/stripe/stripe-go/v78"
	"github.com/stripe/stripe-go/v78/checkout/session"
	"github.com/stripe/stripe-go/v78/customer"
	"github.com/stripe/stripe-go/v78/testhelpers/testclock"
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
	logger := rutil.Logger(r)
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
			if !models.BlogFailedStatuses[discoverResult.subscription.BlogStatus] {
				models.ProductEvent_MustEmitCreateSubscription(
					pc, discoverResult.subscription, userIsAnonymous,
				)
			}
			redirectPath := rutil.SubscriptionSetupPath(discoverResult.subscription.Id)
			http.Redirect(w, r, redirectPath, http.StatusSeeOther)
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
			if util.SuggestionFeedUrls[startUrl] {
				blog, err := models.Blog_GetLatestByFeedUrl(conn, startUrl)
				if errors.Is(err, models.ErrBlogNotFound) {
					logger.Info().Msgf(
						"Tried to use a cached suggestion but could not find any: %s", startUrl,
					)
				} else if err != nil {
					panic(err)
				} else if blog.Status == models.BlogStatusCrawlInProgress ||
					models.BlogCrawledStatuses[blog.Status] {

					subscription, err := models.Subscription_CreateForBlog(
						conn, blog, currentUser, productUserId,
					)
					if err != nil {
						panic(err)
					}
					logger.Info().Msgf("Using a cached suggestion: %s", startUrl)
					models.ProductEvent_MustEmitCreateSubscription(pc, subscription, userIsAnonymous)
					http.Redirect(w, r, rutil.SubscriptionSetupPath(subscription.Id), http.StatusSeeOther)
					return
				} else {
					logger.Info().Msgf(
						"Tried to use a cached suggestion but its status was bad: %s (%s)",
						startUrl, blog.Status,
					)
				}
			}
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
	logger := rutil.Logger(r)
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
		if !models.BlogFailedStatuses[discoverResult.subscription.BlogStatus] {
			models.ProductEvent_MustEmitCreateSubscription(pc, discoverResult.subscription, userIsAnonymous)
		}
		redirectPath := rutil.SubscriptionSetupPath(discoverResult.subscription.Id)
		http.Redirect(w, r, redirectPath, http.StatusSeeOther)
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
		if util.SuggestionFeedUrls[startUrl] {
			blog, err := models.Blog_GetLatestByFeedUrl(conn, startUrl)
			if errors.Is(err, models.ErrBlogNotFound) {
				logger.Info().Msgf(
					"Tried to use a cached suggestion but could not find any: %s", startUrl,
				)
			} else if err != nil {
				panic(err)
			} else if blog.Status == models.BlogStatusCrawlInProgress ||
				models.BlogCrawledStatuses[blog.Status] {

				subscription, err := models.Subscription_CreateForBlog(
					conn, blog, currentUser, productUserId,
				)
				if err != nil {
					panic(err)
				}
				logger.Info().Msgf("Using a cached suggestion: %s", startUrl)
				models.ProductEvent_MustEmitCreateSubscription(pc, subscription, userIsAnonymous)
				http.Redirect(w, r, rutil.SubscriptionSetupPath(subscription.Id), http.StatusSeeOther)
				return
			} else {
				logger.Info().Msgf(
					"Tried to use a cached suggestion but its status was bad: %s (%s)",
					startUrl, blog.Status,
				)
			}
		}
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
		if !models.BlogFailedStatuses[discoverResult.subscription.BlogStatus] {
			models.ProductEvent_MustEmitCreateSubscription(pc, discoverResult.subscription, userIsAnonymous)
		}
		util.MustWrite(w, rutil.SubscriptionSetupPath(discoverResult.subscription.Id))
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
		http.Redirect(w, r, "/", http.StatusSeeOther)
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

type discoveredFeeds struct {
	feeds []*models.StartFeed
}

type discoverError struct{}

type discoverResult interface {
	discoverResultTag()
}

func (*discoveredSubscription) discoverResultTag() {}
func (*discoveredFeeds) discoverResultTag()        {}
func (*discoverError) discoverResultTag()          {}

func onboarding_MustDiscoverFeeds(
	conn *pgw.Conn, startUrl string, currentUser *models.User, productUserId models.ProductUserId,
) (discoverResult, models.TypedBlogUrlResult) {
	logger := conn.Logger()
	if startUrl == crawler.HardcodedOurMachinery || startUrl == crawler.HardcodedSequences {
		blog, err := models.Blog_GetLatestByFeedUrl(conn, startUrl)
		if errors.Is(err, models.ErrBlogNotFound) {
			panic(fmt.Errorf("Blog not found for %s", startUrl))
		} else if err != nil {
			panic(err)
		}
		subscription, err := models.Subscription_CreateForBlog(conn, blog, currentUser, productUserId)
		if err != nil {
			panic(err)
		}
		logger.Info().Msgf("Discover feeds for %s - created subscription", startUrl)
		return &discoveredSubscription{subscription: subscription}, models.TypedBlogUrlResultHardcoded
	}

	httpClient := crawler.NewHttpClientImpl(conn.Context(), false)
	zlogger := crawler.ZeroLogger{Logger: logger}
	progressLogger := crawler.NewMockProgressLogger(&zlogger)
	crawlCtx := crawler.NewCrawlContext(httpClient, nil, &progressLogger)
	discoverFeedsResult := crawler.DiscoverFeedsAtUrl(startUrl, true, &crawlCtx, &zlogger)
	switch result := discoverFeedsResult.(type) {
	case *crawler.DiscoveredSingleFeed:
		logger.Info().Msgf("Discover feeds at %s - found single feed", startUrl)
		var maybeStartPageId *models.StartPageId
		if result.MaybeStartPage != nil {
			startPageId, err := models.StartPage_Create(conn, *result.MaybeStartPage)
			if err != nil {
				panic(err)
			}
			maybeStartPageId = &startPageId
		}
		startFeed, err := models.StartFeed_CreateFetched(conn, maybeStartPageId, result.Feed)
		if err != nil {
			panic(err)
		}
		updatedBlog, err := models.Blog_CreateOrUpdate(conn, startFeed, jobs.GuidedCrawlingJob_PerformNow)
		if err != nil {
			panic(err)
		}
		subscription, err := models.Subscription_CreateForBlog(conn, updatedBlog, currentUser, productUserId)
		if err != nil {
			panic(err)
		}
		logger.Info().Msgf("Discover feeds at %s - created subscription", startUrl)
		return &discoveredSubscription{subscription: subscription}, models.TypedBlogUrlResultFeed
	case *crawler.DiscoveredMultipleFeeds:
		logger.Info().Msgf("Discover feeds at %s - found %d feeds", startUrl, len(result.Feeds))
		startPageId, err := models.StartPage_Create(conn, result.StartPage)
		if err != nil {
			panic(err)
		}
		var startFeeds []*models.StartFeed
		for _, discoveredFeed := range result.Feeds {
			startFeed, err := models.StartFeed_Create(conn, startPageId, discoveredFeed)
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

func Onboarding_Pricing(w http.ResponseWriter, r *http.Request) {
	currentUser := rutil.CurrentUser(r)
	conn := rutil.DBConn(r)
	isOnFreePlan := false
	isOnSupporterPlan := false
	isOnPatronPlan := false
	onboardingBlogName := ""
	if currentUser != nil {
		row := conn.QueryRow(`
			select plan_id from pricing_offers
			where id = (select offer_id from users_without_discarded where id = $1)
		`, currentUser.Id)
		var currentPlanId models.PlanId
		err := row.Scan(&currentPlanId)
		if err != nil {
			panic(err)
		}

		switch currentPlanId {
		case models.PlanIdFree:
			isOnFreePlan = true
		case models.PlanIdSupporter:
			isOnSupporterPlan = true
		case models.PlanIdPatron:
			isOnPatronPlan = true
		default:
			panic(fmt.Errorf("Unknown plan id: %s", currentPlanId))
		}
	} else {
		subscriptionId := rutil.MustExtractAnonymousSubscriptionId(w, r)
		if subscriptionId != 0 {
			row := conn.QueryRow(`
				select name from subscriptions_without_discarded where id = $1 and user_id is null
			`, subscriptionId)
			err := row.Scan(&onboardingBlogName)
			if errors.Is(err, pgx.ErrNoRows) {
				// no-op
			} else if err != nil {
				panic(err)
			}
		}
	}

	row := conn.QueryRow(`
		select * from (
			select id, monthly_rate::numeric, yearly_rate::numeric from pricing_offers
			where id = (select default_offer_id from pricing_plans where id = $1)
		) s
		left join lateral (
			select id, monthly_rate::numeric, yearly_rate::numeric from pricing_offers
			where id = (select default_offer_id from pricing_plans where id = $2)
		) p
		on 1=1
	`, models.PlanIdSupporter, models.PlanIdPatron)
	var supporterOfferId, patronOfferId string
	var supporterMonthlyRate, supporterYearlyRate int
	var patronMonthlyRate, patronYearlyRate int
	err := row.Scan(
		&supporterOfferId, &supporterMonthlyRate, &supporterYearlyRate,
		&patronOfferId, &patronMonthlyRate, &patronYearlyRate,
	)
	if err != nil {
		panic(err)
	}

	type PricingResult struct {
		Title                string
		Session              *util.Session
		IsNewUser            bool
		IsOnFreePlan         bool
		IsOnPaidPlan         bool
		IsOnSupporterPlan    bool
		IsOnPatronPlan       bool
		OnboardingBlogName   string
		SupporterOfferId     string
		PatronOfferId        string
		MonthlyIntervalName  models.BillingInterval
		YearlyIntervalName   models.BillingInterval
		SupporterMonthlyRate int
		SupporterYearlyRate  int
		PatronMonthlyRate    int
		PatronYearlyRate     int
	}
	templates.MustWrite(w, "onboarding/pricing", PricingResult{
		Title:                util.DecorateTitle("Pricing"),
		Session:              rutil.Session(r),
		IsNewUser:            currentUser == nil,
		IsOnFreePlan:         isOnFreePlan,
		IsOnPaidPlan:         isOnSupporterPlan || isOnPatronPlan,
		IsOnSupporterPlan:    isOnSupporterPlan,
		IsOnPatronPlan:       isOnPatronPlan,
		OnboardingBlogName:   onboardingBlogName,
		SupporterOfferId:     supporterOfferId,
		PatronOfferId:        patronOfferId,
		MonthlyIntervalName:  models.BillingIntervalMonthly,
		YearlyIntervalName:   models.BillingIntervalYearly,
		SupporterMonthlyRate: supporterMonthlyRate,
		SupporterYearlyRate:  supporterYearlyRate,
		PatronMonthlyRate:    patronMonthlyRate,
		PatronYearlyRate:     patronYearlyRate,
	})
}

func Onboarding_Checkout(w http.ResponseWriter, r *http.Request) {
	conn := rutil.DBConn(r)
	currentUser := rutil.CurrentUser(r)
	var maybeCustomerEmail *string
	var maybeMetadata map[string]string
	successPath := "/signup"
	if currentUser != nil {
		row := conn.QueryRow(`
			select plan_id from pricing_offers
			where id = (select offer_id from users_without_discarded where id = $1)
		`, currentUser.Id)
		var planId models.PlanId
		err := row.Scan(&planId)
		if err != nil {
			panic(err)
		}

		switch planId {
		case models.PlanIdFree:
			maybeCustomerEmail = stripe.String(currentUser.Email)
			maybeMetadata = map[string]string{"user_id": fmt.Sprint(currentUser.Id)}
			successPath = "/upgrade"
		case models.PlanIdSupporter, models.PlanIdPatron:
			http.Redirect(w, r, "/subscriptions", http.StatusSeeOther)
			return
		default:
			panic(fmt.Errorf("Unknown plan: %s", planId))
		}
	}
	interval := util.EnsureParamStr(r, "interval")
	if interval != "monthly" && interval != "yearly" {
		panic(fmt.Errorf("Unknown interval: %s", interval))
	}
	offerId := util.EnsureParamStr(r, "offer_id")
	row := conn.QueryRow(`
		select stripe_`+interval+`_price_id from pricing_offers
		where id = $1
	`, offerId)
	var priceId string
	err := row.Scan(&priceId)
	if err != nil {
		panic(err)
	}

	var maybeCustomerId *string
	var maybeCustomerUpdate *stripe.CheckoutSessionCustomerUpdateParams
	if config.Cfg.Env.IsDevOrTest() {
		maybeTestClock, err := models.TestSingleton_GetValue(conn, "test_clock")
		if err != nil {
			panic(err)
		}
		if maybeTestClock != nil && *maybeTestClock == "yes" {
			//nolint:exhaustruct
			clock, err := testclock.New(&stripe.TestHelpersTestClockParams{
				FrozenTime: stripe.Int64(time.Now().Unix()),
			})
			if err != nil {
				panic(err)
			}
			//nolint:exhaustruct
			cus, err := customer.New(&stripe.CustomerParams{
				Email:     maybeCustomerEmail,
				TestClock: &clock.ID,
			})
			if err != nil {
				panic(err)
			}
			maybeCustomerEmail = nil
			maybeCustomerId = &cus.ID
			//nolint:exhaustruct
			maybeCustomerUpdate = &stripe.CheckoutSessionCustomerUpdateParams{
				Address: stripe.String(string(stripe.CheckoutSessionBillingAddressCollectionAuto)),
			}
		}
	}

	successUrl := fmt.Sprintf("%s%s?session_id={CHECKOUT_SESSION_ID}", config.Cfg.RootUrl, successPath)
	cancelUrl := fmt.Sprintf("%s/pricing", config.Cfg.RootUrl)
	//nolint:exhaustruct
	params := &stripe.CheckoutSessionParams{
		AllowPromotionCodes: stripe.Bool(true),
		CustomerEmail:       maybeCustomerEmail,
		Customer:            maybeCustomerId,
		SuccessURL:          stripe.String(successUrl),
		CancelURL:           stripe.String(cancelUrl),
		Mode:                stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		LineItems: []*stripe.CheckoutSessionLineItemParams{{
			Price:    stripe.String(priceId),
			Quantity: stripe.Int64(1),
		}},
		AutomaticTax: &stripe.CheckoutSessionAutomaticTaxParams{
			Enabled: stripe.Bool(true),
		},
		CustomerUpdate: maybeCustomerUpdate,
		Metadata:       maybeMetadata,
	}

	sesh, err := session.New(params)
	if err != nil {
		panic(err)
	}

	http.Redirect(w, r, sesh.URL, http.StatusSeeOther)
}
