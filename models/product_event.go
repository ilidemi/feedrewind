package models

import (
	"feedrewind/db/pgw"
	"feedrewind/util"
	"net/http"
	"net/url"
	"regexp"

	"github.com/google/uuid"
	"github.com/mileusna/useragent"
)

type ProductUserId string

func ProductUserId_New() (ProductUserId, error) {
	guid, err := uuid.NewRandom()
	if err != nil {
		return "", err
	}

	return ProductUserId(guid.String()), nil
}

type ProductEventRequestArgs struct {
	Tx              pgw.Queryable
	Request         *http.Request
	ProductUserId   ProductUserId
	EventType       string
	EventProperties map[string]any
	UserProperties  map[string]any
}

func ProductEvent_MustEmitFromRequest(args ProductEventRequestArgs) {
	platform := resolveUserAgent(args.Request.UserAgent())
	anonIp := anonymizeUserIp(util.UserIp(args.Request))
	_, err := args.Tx.Exec(`
		insert into product_events (
			event_type, event_properties, user_properties, user_ip, product_user_id, browser, os_name,
			os_version, bot_name
		)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`,
		args.EventType, args.EventProperties, args.UserProperties, anonIp, args.ProductUserId,
		platform.Browser, platform.OsName, platform.OsVersion, platform.BotName,
	)
	if err != nil {
		panic(err)
	}
}

type ProductEventScheduleArgs struct {
	Tx             pgw.Queryable
	Request        *http.Request
	ProductUserId  ProductUserId
	EventType      string
	SubscriptionId SubscriptionId
	BlogBestUrl    string
	WeeklyCount    int
	ActiveDays     int
}

func ProductEvent_MustEmitSchedule(args ProductEventScheduleArgs) {
	ProductEvent_MustEmitFromRequest(ProductEventRequestArgs{
		Tx:            args.Tx,
		Request:       args.Request,
		ProductUserId: args.ProductUserId,
		EventType:     args.EventType,
		EventProperties: map[string]any{
			"subscription_id":      args.SubscriptionId,
			"blog_url":             args.BlogBestUrl,
			"weekly_count":         args.WeeklyCount,
			"active_days":          args.ActiveDays,
			"posts_per_active_day": float64(args.WeeklyCount) / float64(args.ActiveDays),
		},
		UserProperties: nil,
	})
}

type ProductEventVisitAddPageArgs struct {
	Tx              pgw.Queryable
	Request         *http.Request
	ProductUserId   ProductUserId
	Path            string
	UserIsAnonymous bool
	Extra           map[string]any
}

func ProductEvent_MustEmitVisitAddPage(args ProductEventVisitAddPageArgs) {
	referer := collapseReferer(args.Request.Referer())
	eventProperties := map[string]any{
		"path":              args.Path,
		"referer":           referer,
		"user_is_anonymous": args.UserIsAnonymous,
	}
	for key, value := range args.Extra {
		eventProperties[key] = value
	}

	ProductEvent_MustEmitFromRequest(ProductEventRequestArgs{
		Tx:              args.Tx,
		Request:         args.Request,
		ProductUserId:   args.ProductUserId,
		EventType:       "visit add page",
		EventProperties: eventProperties,
		UserProperties:  nil,
	})
}

type ProductEventDiscoverFeedArgs struct {
	Tx              pgw.Queryable
	Request         *http.Request
	ProductUserId   ProductUserId
	BlogUrl         string
	Result          TypedBlogUrlResult
	UserIsAnonymous bool
}

func ProductEvent_MustEmitDiscoverFeeds(args ProductEventDiscoverFeedArgs) {
	ProductEvent_MustEmitFromRequest(ProductEventRequestArgs{
		Tx:            args.Tx,
		Request:       args.Request,
		ProductUserId: args.ProductUserId,
		EventType:     "discover feeds",
		EventProperties: map[string]any{
			"blog_url":          args.BlogUrl,
			"result":            args.Result,
			"user_is_anonymous": args.UserIsAnonymous,
		},
		UserProperties: nil,
	})
}

type ProductEventCreateSubscriptionArgs struct {
	Tx              pgw.Queryable
	Request         *http.Request
	ProductUserId   ProductUserId
	Subscription    *SubscriptionCreateResult
	UserIsAnonymous bool
}

func ProductEvent_MustEmitCreateSubscription(args ProductEventCreateSubscriptionArgs) {
	ProductEvent_MustEmitFromRequest(ProductEventRequestArgs{
		Tx:            args.Tx,
		Request:       args.Request,
		ProductUserId: args.ProductUserId,
		EventType:     "create subscription",
		EventProperties: map[string]any{
			"subscription_id":   args.Subscription.Id,
			"blog_url":          args.Subscription.BlogBestUrl,
			"is_blog_crawled":   BlogCrawledStatuses[args.Subscription.BlogStatus],
			"user_is_anonymous": args.UserIsAnonymous,
		},
		UserProperties: nil,
	})
}

var userIpRegex = regexp.MustCompile(`.\d+.\d+$`)

func anonymizeUserIp(userIp string) string {
	return userIpRegex.ReplaceAllString(userIp, ".0.1")
}

type userPlatform struct {
	Browser   string
	OsName    string
	OsVersion string
	BotName   *string
}

func resolveUserAgent(userAgentStr string) userPlatform {
	userAgent := useragent.Parse(userAgentStr)
	if userAgent.Bot {
		return userPlatform{
			Browser:   "Crawler",
			OsName:    "Crawler",
			OsVersion: "Crawler",
			BotName:   &userAgent.Name,
		}
	} else {
		return userPlatform{
			Browser:   userAgent.Name,
			OsName:    userAgent.OS,
			OsVersion: userAgent.OSVersion,
			BotName:   nil,
		}
	}
}

func collapseReferer(referer string) *string {
	if referer == "" {
		return nil
	}

	refererUrl, err := url.Parse(referer)
	if err != nil {
		return &referer
	}

	if refererUrl.Host == "feedrewind.com" ||
		refererUrl.Host == "www.feedrewind.com" ||
		refererUrl.Host == "feedrewind.herokuapp.com" {

		result := "FeedRewind"
		return &result
	}

	return &referer
}
