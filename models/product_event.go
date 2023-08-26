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

type ProductEventContext struct {
	Tx            pgw.Queryable
	Request       *http.Request
	ProductUserId ProductUserId
}

func ProductEvent_MustEmit(
	tx pgw.Queryable, productUserId ProductUserId, eventType string, eventProperties map[string]any,
	userProperties map[string]any,
) {
	_, err := tx.Exec(`
		insert into product_events (
			event_type, event_properties, user_properties, product_user_id
		)
		values ($1, $2, $3, $4)
	`, eventType, eventProperties, userProperties, productUserId)
	if err != nil {
		panic(err)
	}
}

func NewProductEventContext(
	tx pgw.Queryable, request *http.Request, productUserId ProductUserId,
) ProductEventContext {
	return ProductEventContext{
		Tx:            tx,
		Request:       request,
		ProductUserId: productUserId,
	}
}

func ProductEvent_MustEmitFromRequest(
	pc ProductEventContext, eventType string, eventProperties map[string]any, userProperties map[string]any,
) {
	platform := resolveUserAgent(pc.Request.UserAgent())
	anonIp := anonymizeUserIp(util.UserIp(pc.Request))
	_, err := pc.Tx.Exec(`
		insert into product_events (
			event_type, event_properties, user_properties, user_ip, product_user_id, browser, os_name,
			os_version, bot_name
		)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`,
		eventType, eventProperties, userProperties, anonIp, pc.ProductUserId, platform.Browser,
		platform.OsName, platform.OsVersion, platform.BotName,
	)
	if err != nil {
		panic(err)
	}
}

func ProductEvent_MustEmitSchedule(
	pc ProductEventContext, eventType string, subscriptionId SubscriptionId, blogBestUrl string,
	weeklyCount int, activeDays int,
) {
	ProductEvent_MustEmitFromRequest(pc, eventType, map[string]any{
		"subscription_id":      subscriptionId,
		"blog_url":             blogBestUrl,
		"weekly_count":         weeklyCount,
		"active_days":          activeDays,
		"posts_per_active_day": float64(weeklyCount) / float64(activeDays),
	}, nil)
}

func ProductEvent_MustEmitVisitAddPage(
	pc ProductEventContext, path string, userIsAnonymous bool, extra map[string]any,
) {
	referer := collapseReferer(pc.Request.Referer())
	eventProperties := map[string]any{
		"path":              path,
		"referer":           referer,
		"user_is_anonymous": userIsAnonymous,
	}
	for key, value := range extra {
		eventProperties[key] = value
	}
	ProductEvent_MustEmitFromRequest(pc, "visit add page", eventProperties, nil)
}

func ProductEvent_MustEmitDiscoverFeeds(
	pc ProductEventContext, blogUrl string, typedResult TypedBlogUrlResult, userIsAnonymous bool,
) {
	ProductEvent_MustEmitFromRequest(pc, "discover feeds", map[string]any{
		"blog_url":          blogUrl,
		"result":            typedResult,
		"user_is_anonymous": userIsAnonymous,
	}, nil)
}

func ProductEvent_MustEmitCreateSubscription(
	pc ProductEventContext, subscription *SubscriptionCreateResult, userIsAnonymous bool,
) {
	ProductEvent_MustEmitFromRequest(pc, "create subscription", map[string]any{
		"subscription_id":   subscription.Id,
		"blog_url":          subscription.BlogBestUrl,
		"is_blog_crawled":   BlogCrawledStatuses[subscription.BlogStatus],
		"user_is_anonymous": userIsAnonymous,
	}, nil)
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
