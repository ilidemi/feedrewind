package models

import (
	"bytes"
	"feedrewind/crawler"
	"feedrewind/db/pgw"
	"feedrewind/models/mutil"
	"feedrewind/oops"
	"feedrewind/util"
	"fmt"
)

func MustInit(tx pgw.Queryable) {
	logger := tx.Logger()
	var timezoneInExpr bytes.Buffer
	timezoneInExpr.WriteString("('")
	isFirst := true
	for groupId := range util.GroupIdByTimezoneId {
		if isFirst {
			isFirst = false
		} else {
			timezoneInExpr.WriteString("', '")
		}
		timezoneInExpr.WriteString(groupId)
	}
	for groupId := range util.UnfriendlyGroupIds {
		timezoneInExpr.WriteString("', '")
		timezoneInExpr.WriteString(groupId)
	}
	timezoneInExpr.WriteString("')")
	query := fmt.Sprintf(
		"select user_id, timezone from user_settings where timezone not in %s", timezoneInExpr.String(),
	)

	rows, err := tx.Query(query)
	if err != nil {
		panic(err)
	}
	for rows.Next() {
		var userId UserId
		var timezone string
		err := rows.Scan(&userId, &timezone)
		if err != nil {
			panic(err)
		}
		logger.Warn().Msgf("User timezone not found in tzdb: %s (user %d)", timezone, userId)
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}
}

// TypedBlogUrl

type TypedBlogUrlResult string

const (
	TypedBlogUrlResultHardcoded             TypedBlogUrlResult = "hardcoded"
	TypedBlogUrlResultFeed                  TypedBlogUrlResult = "feed"
	TypedBlogUrlResultKnownUnsupported      TypedBlogUrlResult = "known_unsupported"
	TypedBlogUrlResultPageWithMultipleFeeds TypedBlogUrlResult = "page_with_multiple_feeds"
	TypedBlogUrlResultNotAUrl               TypedBlogUrlResult = "not_a_url"
	TypedBlogUrlResultNoFeeds               TypedBlogUrlResult = "no_feeds"
	TypedBlogUrlResultCouldNotReach         TypedBlogUrlResult = "could_not_reach"
	TypedBlogUrlResultBadFeed               TypedBlogUrlResult = "bad_feed"
)

func TypedBlogUrl_Create(
	tx pgw.Queryable, typedUrl string, strippedUrl string, source string, result TypedBlogUrlResult,
	maybeUserId *UserId,
) error {
	_, err := tx.Exec(`
		insert into typed_blog_urls (typed_url, stripped_url, source, result, user_id)
		values ($1, $2, $3, $4, $5)
	`, typedUrl, strippedUrl, source, result, maybeUserId)
	return err
}

// StartFeed

type StartFeedId int64

type StartFeed struct {
	Id              StartFeedId
	Title           string
	Url             string
	MaybeParsedFeed *crawler.ParsedFeed
}

func StartFeed_CreateFetched(
	tx pgw.Queryable, startPageId *StartPageId, discoveredFetchedFeed crawler.DiscoveredFetchedFeed,
) (*StartFeed, error) {
	idInt, err := mutil.RandomId(tx, "start_feeds")
	if err != nil {
		return nil, err
	}
	id := StartFeedId(idInt)
	_, err = tx.Exec(`
		insert into start_feeds (id, start_page_id, title, url, final_url, content)
		values ($1, $2, $3, $4, $5, $6)
	`, id, startPageId, discoveredFetchedFeed.Title, discoveredFetchedFeed.Url,
		discoveredFetchedFeed.FinalUrl, []byte(discoveredFetchedFeed.Content),
	)
	if err != nil {
		return nil, err
	}

	return &StartFeed{
		Id:              id,
		Title:           discoveredFetchedFeed.Title,
		Url:             discoveredFetchedFeed.FinalUrl,
		MaybeParsedFeed: discoveredFetchedFeed.ParsedFeed,
	}, nil
}

func StartFeed_Create(
	tx pgw.Queryable, startPageId StartPageId, discoveredFeed crawler.DiscoveredFeed,
) (*StartFeed, error) {
	idInt, err := mutil.RandomId(tx, "start_feeds")
	if err != nil {
		return nil, err
	}
	id := StartFeedId(idInt)
	_, err = tx.Exec(`
		insert into start_feeds (id, start_page_id, title, url, final_url, content)
		values ($1, $2, $3, $4, null, null)
		returning id
	`, id, startPageId, discoveredFeed.Title, discoveredFeed.Url)
	if err != nil {
		return nil, err
	}

	return &StartFeed{
		Id:              id,
		Title:           discoveredFeed.Title,
		Url:             discoveredFeed.Url,
		MaybeParsedFeed: nil,
	}, nil
}

func StartFeed_GetUnfetched(tx pgw.Queryable, id StartFeedId) (*StartFeed, error) {
	row := tx.QueryRow(`select title, url from start_feeds where id = $1`, id)
	var title, url string
	err := row.Scan(&title, &url)
	if err != nil {
		return nil, err
	}

	return &StartFeed{
		Id:              id,
		Title:           title,
		Url:             url,
		MaybeParsedFeed: nil,
	}, err
}

func StartFeed_UpdateFetched(
	tx pgw.Queryable, startFeed *StartFeed, finalUrl string, content string, parsedFeed *crawler.ParsedFeed,
) (*StartFeed, error) {
	_, err := tx.Exec(`
		update start_feeds set final_url = $1, content = $2 where id = $3
	`, finalUrl, content, startFeed.Id)
	if err != nil {
		return nil, err
	}

	return &StartFeed{
		Id:              startFeed.Id,
		Title:           startFeed.Title,
		Url:             finalUrl,
		MaybeParsedFeed: parsedFeed,
	}, nil
}

// StartPage

type StartPageId int64

func StartPage_Create(
	tx pgw.Queryable, discoveredStartPage crawler.DiscoveredStartPage,
) (StartPageId, error) {
	row := tx.QueryRow(`
		insert into start_pages (url, final_url, content)
		values ($1, $2, $3)
		returning id
	`, discoveredStartPage.Url, discoveredStartPage.FinalUrl, []byte(discoveredStartPage.Content))
	var id StartPageId
	err := row.Scan(&id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

// RSS

func UserRss_GetBody(tx pgw.Queryable, userId UserId) (string, error) {
	row := tx.QueryRow(`select body from user_rsses where user_id = $1`, userId)
	var body string
	err := row.Scan(&body)
	return body, err
}

func UserRss_Upsert(tx pgw.Queryable, userId UserId, body string) error {
	_, err := tx.Exec(`
		insert into user_rsses (user_id, body) values ($1, $2)
		on conflict (user_id)
		do update set body = $2
	`, userId, body)
	return err
}

func SubscriptionRss_GetBody(tx pgw.Queryable, subscriptionId SubscriptionId) (string, error) {
	row := tx.QueryRow(`select body from subscription_rsses where subscription_id = $1`, subscriptionId)
	var body string
	err := row.Scan(&body)
	return body, err
}

func SubscriptionRss_Upsert(tx pgw.Queryable, subscriptionId SubscriptionId, body string) error {
	_, err := tx.Exec(`
		insert into subscription_rsses (subscription_id, body) values ($1, $2)
		on conflict (subscription_id)
		do update set body = $2
	`, subscriptionId, body)
	return err
}

// Billing

type PlanId string

const (
	PlanIdFree      PlanId = "free"
	PlanIdSupporter PlanId = "supporter"
)

type OfferId string

type BillingInterval string

const (
	BillingIntervalMonthly BillingInterval = "monthly"
	BillingIntervalYearly  BillingInterval = "yearly"
)

func BillingInterval_GetByOffer(
	tx pgw.Queryable, stripeProductId, stripePriceId string,
) (BillingInterval, error) {
	row := tx.QueryRow(`
		select stripe_monthly_price_id, stripe_yearly_price_id from pricing_offers
		where stripe_product_id = $1
	`, stripeProductId)
	var monthlyPriceId, yearlyPriceId string
	err := row.Scan(&monthlyPriceId, &yearlyPriceId)
	if err != nil {
		return "", err
	}

	switch stripePriceId {
	case monthlyPriceId:
		return BillingIntervalMonthly, nil
	case yearlyPriceId:
		return BillingIntervalYearly, nil
	default:
		return "", oops.Newf("Unknown price id for stripe product %s: %s", stripeProductId, stripePriceId)
	}
}

// AdminTelemetry

func AdminTelemetry_Create(tx pgw.Queryable, key string, value float64, extra map[string]any) error {
	_, err := tx.Exec(`
		insert into admin_telemetries (key, value, extra) values ($1, $2, $3)
	`, key, value, extra)
	return err
}

// TestSingleton

func TestSingleton_GetValue(tx pgw.Queryable, key string) (*string, error) {
	row := tx.QueryRow(`select value from test_singletons where key = $1`, key)
	var maybeValue *string
	err := row.Scan(&maybeValue)
	return maybeValue, err
}

func TestSingleton_SetValue(tx pgw.Queryable, key, value string) error {
	tag, err := tx.Exec(`
		update test_singletons
		set value = $1
		where key = $2
	`, value, key)
	if err != nil {
		return err
	}

	rowsAffected := tag.RowsAffected()
	if rowsAffected != 1 {
		return oops.Newf("Expected to update 1 row, got %d", rowsAffected)
	}

	return nil
}
