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

func ProductUserId_MustNew() ProductUserId {
	guid, err := uuid.NewRandom()
	if err != nil {
		panic(err)
	}

	return ProductUserId(guid.String())
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
	args.Tx.MustExec(`
		insert into product_events (
			event_type, event_properties, user_properties, user_ip, product_user_id, browser, os_name,
			os_version, bot_name
		)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`,
		args.EventType, args.EventProperties, args.UserProperties, anonIp, args.ProductUserId,
		platform.Browser, platform.OsName, platform.OsVersion, platform.BotName,
	)
}

func ProductEvent_MustEmitAddPage(
	tx pgw.Queryable, r *http.Request, productUserId ProductUserId, path string, userIsAnonymous bool,
) {
	referer := collapseReferer(r.Referer())
	ProductEvent_MustEmitFromRequest(ProductEventRequestArgs{
		Tx:            tx,
		Request:       r,
		ProductUserId: productUserId,
		EventType:     "visit add page",
		EventProperties: map[string]any{
			"path":              path,
			"referer":           referer,
			"user_is_anonymous": userIsAnonymous,
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
