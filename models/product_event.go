package models

import (
	"feedrewind/db/pgw"
	"feedrewind/log"
	"feedrewind/oops"
	"feedrewind/util"
	"fmt"
	"net/http"
	"regexp"
	"time"

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
	err := ProductEvent_Emit(tx, productUserId, eventType, eventProperties, userProperties)
	if err != nil {
		panic(err)
	}
}

func ProductEvent_Emit(
	tx pgw.Queryable, productUserId ProductUserId, eventType string, eventProperties map[string]any,
	userProperties map[string]any,
) error {
	_, err := tx.Exec(`
		insert into product_events (
			event_type, event_properties, user_properties, product_user_id
		)
		values ($1, $2, $3, $4)
	`, eventType, eventProperties, userProperties, productUserId)
	return err
}

func ProductEvent_DummyEmitOrLog(
	tx pgw.Queryable, request *http.Request, allowBots bool, eventType string,
	eventProperties map[string]any, errorLogger log.Logger,
) {
	uuid, err := uuid.NewRandom()
	if err != nil {
		err := oops.Wrap(err)
		errorLogger.Error().Err(err).Msg("Product event emit error")
	}
	productUserId := fmt.Sprintf("dummy-%s", uuid.String())
	userProperties := map[string]any{
		"is_dummy":       true,
		"bot_is_allowed": allowBots,
	}
	platform := resolveUserAgent(request.UserAgent())
	anonIp := anonymizeUserIp(util.UserIp(request))
	_, err = tx.Exec(`
		insert into product_events (
			product_user_id, event_type, event_properties, user_properties, user_ip, browser, os_name,
			os_version, bot_name
		)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, productUserId, eventType, eventProperties, userProperties, anonIp, platform.Browser, platform.OsName,
		platform.OsVersion, platform.MaybeBotName,
	)
	if err != nil {
		errorLogger.Error().Err(err).Msg("Product event emit error")
	}
}

func ProductEvent_EmitToBatch(
	batch *pgw.Batch, productUserId ProductUserId, eventType string, eventProperties map[string]any,
	userProperties map[string]any,
) {
	batch.Queue(`
		insert into product_events (
			event_type, event_properties, user_properties, product_user_id
		)
		values ($1, $2, $3, $4)
	`, eventType, eventProperties, userProperties, productUserId)
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
		platform.OsName, platform.OsVersion, platform.MaybeBotName,
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
	referer := util.CollapseReferer(pc.Request)
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

type ProductEventId int64

type ProductEvent struct {
	Id                   ProductEventId
	ProductUserId        ProductUserId
	EventType            string
	CreatedAt            time.Time
	MaybeEventProperties map[string]any
	MaybeUserProperties  map[string]any
	MaybeUserIp          *string
	MaybeBrowser         *string
	MaybeOsName          *string
	MaybeOsVersion       *string
	MaybeBotName         *string
}

func ProductEvent_GetNotDispatched(tx pgw.Queryable) ([]ProductEvent, error) {
	rows, err := tx.Query(`
		select id, product_user_id, event_type, created_at, event_properties, user_properties, user_ip,
			browser, os_name, os_version, bot_name
		from product_events
		where dispatched_at is null
	`)
	if err != nil {
		return nil, err
	}

	var productEvents []ProductEvent
	for rows.Next() {
		var e ProductEvent
		err := rows.Scan(
			&e.Id, &e.ProductUserId, &e.EventType, &e.CreatedAt, &e.MaybeEventProperties,
			&e.MaybeUserProperties, &e.MaybeUserIp, &e.MaybeBrowser, &e.MaybeOsName, &e.MaybeOsVersion,
			&e.MaybeBotName,
		)
		if err != nil {
			return nil, err
		}

		productEvents = append(productEvents, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return productEvents, nil
}

func ProductEvent_MarkAsDispatched(
	tx pgw.Queryable, productEventId ProductEventId, dispatchedAt time.Time,
) error {
	_, err := tx.Exec(`
		update product_events
		set dispatched_at = $1
		where id = $2
	`, dispatchedAt, productEventId)
	return err
}

var userIpRegex = regexp.MustCompile(`.\d+.\d+$`)

func anonymizeUserIp(userIp string) string {
	return userIpRegex.ReplaceAllString(userIp, ".0.1")
}

type userPlatform struct {
	Browser      string
	OsName       string
	OsVersion    string
	MaybeBotName *string
}

func resolveUserAgent(userAgentStr string) userPlatform {
	userAgent := useragent.Parse(userAgentStr)
	if userAgent.Bot {
		return userPlatform{
			Browser:      "Crawler",
			OsName:       "Crawler",
			OsVersion:    "Crawler",
			MaybeBotName: &userAgent.Name,
		}
	} else {
		return userPlatform{
			Browser:      userAgent.Name,
			OsName:       userAgent.OS,
			OsVersion:    userAgent.OSVersion,
			MaybeBotName: nil,
		}
	}
}
