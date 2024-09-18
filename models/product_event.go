package models

import (
	"context"
	"feedrewind/db"
	"feedrewind/db/pgw"
	"feedrewind/log"
	"feedrewind/oops"
	"feedrewind/util"
	"fmt"
	"math"
	"net/http"
	"sync"
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
	Qu            pgw.Queryable
	Request       *http.Request
	ProductUserId ProductUserId
}

func ProductEvent_MustEmit(
	qu pgw.Queryable, productUserId ProductUserId, eventType string, eventProperties map[string]any,
	userProperties map[string]any,
) {
	err := ProductEvent_Emit(qu, productUserId, eventType, eventProperties, userProperties)
	if err != nil {
		panic(err)
	}
}

func ProductEvent_Emit(
	qu pgw.Queryable, productUserId ProductUserId, eventType string, eventProperties map[string]any,
	userProperties map[string]any,
) error {
	_, err := qu.Exec(`
		insert into product_events (
			event_type, event_properties, user_properties, product_user_id
		)
		values ($1, $2, $3, $4)
	`, eventType, eventProperties, userProperties, productUserId)
	return err
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
	qu pgw.Queryable, request *http.Request, productUserId ProductUserId,
) ProductEventContext {
	return ProductEventContext{
		Qu:            qu,
		Request:       request,
		ProductUserId: productUserId,
	}
}

func ProductEvent_MustEmitFromRequest(
	pc ProductEventContext, eventType string, eventProperties map[string]any, userProperties map[string]any,
) {
	platform := resolveUserAgent(pc.Request.UserAgent())
	anonIp := util.AnonUserIp(pc.Request)
	_, err := pc.Qu.Exec(`
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

type dummyEvent struct {
	EventType       string
	EventProperties map[string]any
	UserProperties  map[string]any
	AnonIp          string
	Platform        userPlatform
}

var dummyEmitChan = make(chan dummyEvent, 100000)

func ProductEvent_QueueDummyEmit(
	request *http.Request, allowBots bool, eventType string, eventProperties map[string]any,
) {
	userProperties := map[string]any{
		"is_dummy":       true,
		"bot_is_allowed": allowBots,
	}
	platform := resolveUserAgent(request.UserAgent())
	anonIp := util.AnonUserIp(request)
	dummyEmitChan <- dummyEvent{
		EventType:       eventType,
		EventProperties: eventProperties,
		UserProperties:  userProperties,
		AnonIp:          anonIp,
		Platform:        platform,
	}
}

func ProductEvent_StartDummyEventsSync(ctx context.Context, wg *sync.WaitGroup) {
	go func() {
		logger := &log.TaskLogger{TaskName: "emit_dummy_events"}
		ticker := time.NewTicker(time.Second)
		errorCount := 0
		backoffUntil := time.Now()
		var maybeCanceledAt *time.Time
		var events []dummyEvent
		defer wg.Done()

	tick:
		for range ticker.C {
			if ctx.Err() != nil {
				if maybeCanceledAt == nil {
					now := time.Now()
					maybeCanceledAt = &now
				} else if time.Since(*maybeCanceledAt) >= 30*time.Second {
					logger.Error().Msgf(
						"Couldn't emit remaining events for 30 seconds, dropping (%d events)", len(events),
					)
					return
				}
			}

			for {
				select {
				case event := <-dummyEmitChan:
					events = append(events, event)
					continue
				default:
				}
				break
			}

			if ctx.Err() == nil && time.Now().Before(backoffUntil) {
				continue
			}

			if len(events) > 0 {
				pool := db.RootPool
				batch := pool.NewBatch()
				for _, e := range events {
					uuid, err := uuid.NewRandom()
					if err != nil {
						// 1 5 25 125 600 600 600
						backoffInterval := int(math.Min(math.Pow(5, float64(errorCount)), 600))
						backoffUntil = time.Now().Add(time.Duration(backoffInterval) * time.Second)
						errorCount++
						err := oops.Wrap(err)
						logger.Error().Err(err).Msgf(
							"Product event generate id error (%d attempts)", errorCount,
						)
						continue tick
					}
					productUserId := fmt.Sprintf("dummy-%s", uuid.String())
					// created_at will be a bit off (a lot off when db is down for a while) but that's ok
					// for now
					batch.Queue(`
						insert into product_events (
							product_user_id, event_type, event_properties, user_properties, user_ip, browser,
							os_name, os_version, bot_name
						)
						values ($1, $2, $3, $4, $5, $6, $7, $8, $9)
					`, productUserId, e.EventType, e.EventProperties, e.UserProperties, e.AnonIp,
						e.Platform.Browser, e.Platform.OsName, e.Platform.OsVersion, e.Platform.MaybeBotName,
					)
				}
				results := pool.SendBatch(batch)
				err := results.Close()
				if err != nil {
					// 1 5 25 125 600 600 600
					backoffInterval := int(math.Min(math.Pow(5, float64(errorCount)), 600))
					backoffUntil = time.Now().Add(time.Duration(backoffInterval) * time.Second)
					errorCount++
					logger.Error().Err(err).Msgf("Product event emit error (%d attempts)", errorCount)
					continue
				}
				logger.Info().Msgf("Emitted %d dummy events", len(events))
				errorCount = 0
				events = nil
			}

			if ctx.Err() != nil {
				return
			}
		}
	}()
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

func ProductEvent_GetNotDispatched(qu pgw.Queryable) ([]ProductEvent, error) {
	rows, err := qu.Query(`
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
	qu pgw.Queryable, productEventId ProductEventId, dispatchedAt time.Time,
) error {
	_, err := qu.Exec(`
		update product_events
		set dispatched_at = $1
		where id = $2
	`, dispatchedAt, productEventId)
	return err
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
