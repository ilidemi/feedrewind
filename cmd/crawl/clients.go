package crawl

import (
	"context"
	"errors"
	"feedrewind/crawler"
	"feedrewind/db/pgw"
	"net/url"

	"github.com/jackc/pgx/v5"
)

type MockHttpClient struct {
	NetworkRequestsMade int

	conn        *pgw.Conn
	startLinkId int
	httpClient  crawler.HttpClient
}

func NewMockHttpClient(conn *pgw.Conn, startLinkId int) MockHttpClient {
	return MockHttpClient{
		NetworkRequestsMade: 0,
		conn:                conn,
		startLinkId:         startLinkId,
		httpClient:          crawler.NewHttpClientImplCtx(context.Background(), true),
	}
}

func (c *MockHttpClient) Request(
	uri *url.URL, shouldThrottle bool, maybeRobotsClient *crawler.RobotsClient, logger crawler.Logger,
) (*crawler.HttpResponse, error) {
	fetchUrl := uri.String()
	row := c.conn.QueryRow(`
		select code, content_type, location, body from mock_responses
		where start_link_id = $1 and fetch_url = $2
	`, c.startLinkId, fetchUrl)

	var r crawler.HttpResponse
	err := row.Scan(&r.Code, &r.MaybeContentType, &r.MaybeLocation, &r.Body)
	if errors.Is(err, pgx.ErrNoRows) {
		logger.Info("URI not in mock tables, falling back on http client: %s", fetchUrl)
		c.NetworkRequestsMade++
		r, err := c.httpClient.Request(uri, shouldThrottle, maybeRobotsClient, logger)
		if err != nil {
			return nil, err
		}

		_, err = c.conn.Exec(`
			insert into mock_responses (start_link_id, fetch_url, code, content_type, location, body)
			values ($1, $2, $3, $4, $5, $6) 
		`, c.startLinkId, fetchUrl, r.Code, r.MaybeContentType, r.MaybeLocation, r.Body)
		if err != nil {
			return nil, err
		}

		return r, nil
	} else if err != nil {
		return nil, err
	}

	return &r, nil
}

func (c *MockHttpClient) GetRetryDelay(attemptsMade int) float64 {
	switch attemptsMade {
	case 0:
		return 0.01
	case 1:
		return 0.05
	default:
		return 0.15
	}
}

type CachingPuppeteerClient struct {
	Conn        *pgw.Conn
	StartLinkId int
	Impl        *crawler.PuppeteerClientImpl
}

func NewCachingPuppeteerClient(conn *pgw.Conn, startLinkId int) *CachingPuppeteerClient {
	return &CachingPuppeteerClient{
		Conn:        conn,
		StartLinkId: startLinkId,
		Impl:        crawler.NewPuppeteerClientImpl(),
	}
}

func (c *CachingPuppeteerClient) Fetch(
	uri *url.URL, feedEntryCurisTitlesMap crawler.CanonicalUriMap[crawler.MaybeLinkTitle],
	crawlCtx *crawler.CrawlContext, logger crawler.Logger,
	findLoadMoreButton crawler.PuppeteerFindLoadMoreButton, extendedScrollTime bool,
) (*crawler.PuppeteerPage, error) {
	page, err := c.Impl.Fetch(
		uri, feedEntryCurisTitlesMap, crawlCtx, logger, findLoadMoreButton, extendedScrollTime,
	)
	if err != nil {
		return nil, err
	}

	_, err = c.Conn.Exec(`
		insert into mock_puppeteer_pages (start_link_id, fetch_url, body) values ($1, $2, $3)
	`, c.StartLinkId, uri.String(), []byte(page.Content))
	if err != nil {
		return nil, err
	}

	return page, nil
}

type MockPuppeteerClient struct {
	Conn        *pgw.Conn
	StartLinkId int
	Impl        *CachingPuppeteerClient
}

func NewMockPuppeteerClient(conn *pgw.Conn, startLinkId int) *MockPuppeteerClient {
	return &MockPuppeteerClient{
		Conn:        conn,
		StartLinkId: startLinkId,
		Impl:        NewCachingPuppeteerClient(conn, startLinkId),
	}
}

func (c *MockPuppeteerClient) Fetch(
	uri *url.URL, feedEntryCurisTitlesMap crawler.CanonicalUriMap[crawler.MaybeLinkTitle],
	crawlCtx *crawler.CrawlContext, logger crawler.Logger,
	findLoadMoreButton crawler.PuppeteerFindLoadMoreButton, extendedScrollTime bool,
) (*crawler.PuppeteerPage, error) {
	fetchUrl := uri.String()
	row := c.Conn.QueryRow(`
		select body from mock_puppeteer_pages
		where start_link_id = $1 and fetch_url = $2
	`, c.StartLinkId, fetchUrl)

	var body []byte
	err := row.Scan(&body)
	if errors.Is(err, pgx.ErrNoRows) {
		return c.Impl.Fetch(
			uri, feedEntryCurisTitlesMap, crawlCtx, logger, findLoadMoreButton, extendedScrollTime,
		)
	} else if err != nil {
		return nil, err
	}

	return &crawler.PuppeteerPage{
		Content:               string(body),
		MaybeTopScreenshot:    nil,
		MaybeBottomScreenshot: nil,
	}, nil
}
