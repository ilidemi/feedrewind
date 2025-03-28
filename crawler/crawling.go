package crawler

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"feedrewind.com/oops"

	"github.com/antchfx/htmlquery"
	"github.com/antchfx/xpath"
	"github.com/go-rod/rod"
	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
	"golang.org/x/text/encoding"
)

type CrawlContext struct {
	FetchedCuris          CanonicalUriSet
	PptrFetchedCuris      CanonicalUriSet
	Redirects             map[string]*Link
	RequestsMade          int
	PuppeteerRequestsMade int
	DuplicateFetches      int
	TitleRequestsMade     int
	TitleFetchDuration    float64
	HttpClient            HttpClient
	MaybePuppeteerClient  PuppeteerClient
	ProgressLogger        *ProgressLogger
	RobotsClient          *RobotsClient // initialized by the crawler and not the caller
}

func NewCrawlContext(
	httpClient HttpClient, maybePuppeteerClient PuppeteerClient, progressLogger *ProgressLogger,
) CrawlContext {
	curiEqCfg := NewCanonicalEqualityConfig()
	return CrawlContext{
		FetchedCuris:          NewCanonicalUriSet(nil, &curiEqCfg),
		PptrFetchedCuris:      NewCanonicalUriSet(nil, &curiEqCfg),
		Redirects:             make(map[string]*Link),
		RequestsMade:          0,
		PuppeteerRequestsMade: 0,
		DuplicateFetches:      0,
		TitleRequestsMade:     0,
		TitleFetchDuration:    0,
		HttpClient:            httpClient,
		MaybePuppeteerClient:  maybePuppeteerClient,
		ProgressLogger:        progressLogger,
		RobotsClient:          nil,
	}
}

type pageBase struct {
	Curi     CanonicalUri
	FetchUri *url.URL
	Content  string
}

type nonHtmlPage pageBase

type feedPage pageBase

type htmlPage struct {
	pageBase
	Document              *html.Node
	MaybeTopScreenshot    []byte
	MaybeBottomScreenshot []byte
}

type page interface {
	pageTag()
	base() *pageBase
}

func (p *htmlPage) pageTag()    {}
func (p *feedPage) pageTag()    {}
func (p *nonHtmlPage) pageTag() {}

func (p *htmlPage) base() *pageBase {
	return &p.pageBase
}
func (p *feedPage) base() *pageBase {
	return (*pageBase)(p)
}
func (p *nonHtmlPage) base() *pageBase {
	return (*pageBase)(p)
}

type feedOrHtmlPage interface {
	feedOrHtmlPageTag()
}

func (p *htmlPage) feedOrHtmlPageTag() {}
func (p *feedPage) feedOrHtmlPageTag() {}

var ErrNotAnHtmlPage = errors.New("not an html page")
var ErrNotAFeedOrHtmlPage = errors.New("not a feed or html page")
var ErrNotAnNonHtmlPage = errors.New("not an non-html page")

func crawlHtmlPage(initialLink *Link, crawlCtx *CrawlContext, logger Logger) (*htmlPage, error) {
	page, err := crawlPage(initialLink, false, crawlCtx, logger)
	if err != nil {
		return nil, err
	}

	if htmlPage, ok := page.(*htmlPage); ok {
		return htmlPage, nil
	}
	return nil, ErrNotAnHtmlPage
}

func crawlFeedOrHtmlPage(initialLink *Link, crawlCtx *CrawlContext, logger Logger) (feedOrHtmlPage, error) {
	page, err := crawlPage(initialLink, true, crawlCtx, logger)
	if err != nil {
		return nil, err
	}

	switch p := page.(type) {
	case *feedPage:
		return p, nil
	case *htmlPage:
		return p, nil
	default:
		return nil, ErrNotAFeedOrHtmlPage
	}
}

func crawlNonHtmlPage(initialLink *Link, crawlCtx *CrawlContext, logger Logger) (*nonHtmlPage, error) {
	page, err := crawlPage(initialLink, false, crawlCtx, logger)
	if err != nil {
		return nil, err
	}

	if nonHtmlPage, ok := page.(*nonHtmlPage); ok {
		return nonHtmlPage, nil
	}
	return nil, ErrNotAnNonHtmlPage
}

var metaRefreshContentRegex *regexp.Regexp
var permanentErrorCodes map[string]bool

func init() {
	metaRefreshContentRegex = regexp.MustCompile(`(\d+); *(?:URL|url)=(.+)`)
	codes := []string{
		"400", "401", "402", "403", "404", "405", "406", "407", "410", "411", "412", "413", "414", "415",
		"416", "417", "418", "451", codeResponseBodyTooBig,
	}
	permanentErrorCodes = make(map[string]bool, len(codes))
	for _, code := range codes {
		permanentErrorCodes[code] = true
	}
}

func crawlPage(initialLink *Link, isFeedExpected bool, crawlCtx *CrawlContext, logger Logger) (page, error) {
	link := initialLink
	seenUrls := map[string]bool{
		link.Url: true,
	}
	link, err := followCachedRedirects(link, crawlCtx.Redirects, seenUrls)
	if err != nil {
		return nil, err
	}
	shouldThrottle := true
	httpErrorsCount := 0

	for {
		requestStart := time.Now()
		resp, err := crawlCtx.HttpClient.Request(link.Uri, shouldThrottle, crawlCtx.RobotsClient, logger)
		if err != nil {
			return nil, err
		}
		requestMs := time.Since(requestStart).Milliseconds()
		crawlCtx.RequestsMade++
		if shouldThrottle {
			crawlCtx.ProgressLogger.LogHtml()
		}
		shouldThrottle = true

		duplicateFetchLog := ""
		if crawlCtx.FetchedCuris.Contains(link.Curi) {
			duplicateFetchLog = " (duplicate fetch)"
			crawlCtx.DuplicateFetches++
		}

		switch {
		case resp.Code[0] == '3' && resp.MaybeLocation != nil:
			redirectionUrl := *resp.MaybeLocation
			redirectionLink, err := processRedirect(
				redirectionUrl, initialLink, link, resp.Code, requestMs, duplicateFetchLog, seenUrls,
				crawlCtx, logger,
			)
			if err != nil {
				return nil, err
			}

			link = redirectionLink
			shouldThrottle = false
		case resp.Code == "200" || (resp.Code[0] == '3' && resp.MaybeLocation == nil):
			var contentType string
			var body string
			if resp.MaybeContentType != nil {
				tokens := strings.Split(*resp.MaybeContentType, ";")
				contentType = strings.TrimSpace(tokens[0])
				enc, name, certain := charset.DetermineEncoding(resp.Body, *resp.MaybeContentType)
				if name == "windows-1252" && !certain {
					// Undo the bad default
					enc = encoding.Nop
				}
				decoder := enc.NewDecoder()
				body, err = decoder.String(string(resp.Body))
				if err != nil {
					body = string(resp.Body)
				}
			} else {
				contentType = ""
				body = string(resp.Body)
			}

			pageBase := pageBase{
				Curi:     link.Curi,
				FetchUri: link.Uri,
				Content:  body,
			}
			var page page
			switch {
			case isFeedExpected && isFeed(body, logger):
				page = (*feedPage)(&pageBase)
			case contentType == "text/html":
				document, err := parseHtml(body, logger)
				if err != nil {
					return nil, err
				}
				page = &htmlPage{
					pageBase:              pageBase,
					Document:              document,
					MaybeTopScreenshot:    nil,
					MaybeBottomScreenshot: nil,
				}
			default:
				page = (*nonHtmlPage)(&pageBase)
			}

			if htmlPage, ok := page.(*htmlPage); ok {
				metaRefreshElement :=
					htmlquery.FindOne(htmlPage.Document, "/html/head/meta[@http-equiv='refresh']")
				if metaRefreshElement != nil {
					var metaRefreshMatch []string
					for _, attr := range metaRefreshElement.Attr {
						if attr.Key == "content" {
							metaRefreshMatch = metaRefreshContentRegex.FindStringSubmatch(attr.Val)
							if metaRefreshMatch != nil {
								break
							}
						}
					}

					if metaRefreshMatch != nil {
						intervalStr := metaRefreshMatch[1]
						metaRedirectionUrl := metaRefreshMatch[2]
						logCode := fmt.Sprintf("%s_meta_refresh_%s", resp.Code, intervalStr)
						metaRedirectionLink, err := processRedirect(
							metaRedirectionUrl, initialLink, link, logCode, requestMs, duplicateFetchLog,
							seenUrls, crawlCtx, logger,
						)
						if err != nil {
							return nil, err
						}

						link = metaRedirectionLink
						if intervalStr == "0" {
							shouldThrottle = false
						}
						continue
					}
				}
			}

			crawlCtx.FetchedCuris.add(link.Curi)
			logger.Info("%s %s %dms %s%s", resp.Code, contentType, requestMs, link.Url, duplicateFetchLog)
			return page, nil
		case resp.Code == codeSSLError:
			if strings.HasPrefix(link.Uri.Host, "www.") {
				newUri := *link.Uri
				newUri.Host = newUri.Host[4:]
				newUrl := newUri.String()
				logger.Info("SSLError_www %dms %s -> %s", requestMs, link.Url, newUrl)
				link, _ = ToCanonicalLink(newUrl, logger, nil)
				shouldThrottle = false
				continue
			} else {
				logger.Info("SSLError %dms %s", requestMs, link.Url)
				return nil, oops.New("SSLError")
			}
		case permanentErrorCodes[resp.Code] || httpErrorsCount >= 3:
			crawlCtx.FetchedCuris.add(link.Curi)
			logger.Info("%s %dms %s - permanent error", resp.Code, requestMs, link.Url)
			return nil, oops.Newf("Permanent error (%s): %s", resp.Code, link.Url)
		case httpErrorsCount < 3:
			sleepInterval := crawlCtx.HttpClient.GetRetryDelay(httpErrorsCount)
			logger.Info("%s %dms %s - sleeping %fs", resp.Code, requestMs, link.Url, sleepInterval)
			time.Sleep(time.Duration(sleepInterval * float64(time.Second)))
			httpErrorsCount++
			continue
		default:
			return nil, oops.New("Unexpected crawling branch")
		}
	}
}

func processRedirect(
	redirectionUrl string, initialLink *Link, requestLink *Link, code string, requestMs int64,
	duplicateFetchLog string, seenUrls map[string]bool, crawlCtx *CrawlContext, logger Logger,
) (*Link, error) {
	redirectionLink, ok := ToCanonicalLink(redirectionUrl, logger, requestLink.Uri)
	if !ok {
		logger.Info("%s %dms %s -> bad redirection link", code, requestMs, requestLink.Url)
		return nil, oops.Newf("Bad redirection: %s", redirectionUrl)
	}
	if seenUrls[redirectionLink.Url] {
		return nil, oops.Newf(
			"Circular redirect for %s: %v -> %s", initialLink.Url, seenUrls, redirectionLink.Url,
		)
	}

	seenUrls[redirectionLink.Url] = true
	crawlCtx.Redirects[requestLink.Url] = redirectionLink
	redirectionLink, err := followCachedRedirects(redirectionLink, crawlCtx.Redirects, seenUrls)
	if err != nil {
		return nil, err
	}

	// Not marking intermediate canonical urls as fetched because Medium redirect key is a query param
	// not included in canonical url

	logger.Info(
		"%s %dms %s%s -> %s", code, requestMs, requestLink.Url, duplicateFetchLog, redirectionLink.Url,
	)
	return redirectionLink, nil
}

func followCachedRedirects(
	initialLink *Link, redirects map[string]*Link, maybeSeenUrls map[string]bool,
) (*Link, error) {
	link := initialLink
	if maybeSeenUrls == nil {
		maybeSeenUrls = map[string]bool{link.Url: true}
	}

	for len(redirects) > 0 && redirects[link.Url] != nil && link.Url != redirects[link.Url].Url {
		redirectionLink := redirects[link.Url]
		if maybeSeenUrls[redirectionLink.Url] {
			return nil, oops.Newf(
				"Circular redirect for %s: %v -> %s", initialLink.Url, maybeSeenUrls, redirectionLink.Url,
			)
		}
		maybeSeenUrls[redirectionLink.Url] = true
		link = redirectionLink
	}

	return link, nil
}

var loadMoreXPathStr string
var loadMoreXPath *xpath.Expr
var mediumFeedLinkXPath *xpath.Expr
var buttondownTwitterXPath *xpath.Expr

func init() {
	loadMoreXPathStr = `//*[(self::a or self::button)][contains(@class, "load-more")]`
	loadMoreXPath = xpath.MustCompile(loadMoreXPathStr)
	mediumFeedLinkXPath = xpath.MustCompile(`//link[@rel="alternate"][@type="application/rss+xml"][starts-with(@href, "https://medium.")]`)
	buttondownTwitterXPath = xpath.MustCompile(`/html/head/meta[@name="twitter:site"][@content="@buttondown"]`)
}

func crawlWithPuppeteerIfMatch(
	page *htmlPage, feedGenerator FeedGenerator, feedEntryCurisTitlesMap CanonicalUriMap[MaybeLinkTitle],
	crawlCtx *CrawlContext, logger Logger,
) (*htmlPage, error) {
	if crawlCtx.MaybePuppeteerClient == nil {
		return page, nil
	}

	var maybeFindLoadMoreButton PuppeteerFindLoadMoreButton
	var maybeValidate PuppeteerValidate
	puppeteerMatch := false
	extendedScrollTime := false
	switch {
	case htmlquery.QuerySelector(page.Document, loadMoreXPath) != nil:
		logger.Info("Found load more button, rerunning with puppeteer")
		puppeteerMatch = true
		maybeFindLoadMoreButton = func(page *rod.Page) (*rod.Element, error) {
			element, err := page.Sleeper(rod.NotFoundSleeper).ElementX(loadMoreXPathStr)
			if err != nil {
				return nil, err
			}
			isVisible, err := element.Visible()
			if err != nil {
				return nil, err
			}
			if !isVisible {
				return nil, errors.New("load more button is not visible")
			}
			return element, nil
		}
	case htmlquery.QuerySelector(page.Document, mediumFeedLinkXPath) != nil &&
		len(htmlquery.Find(page.Document, "//article")) == 10:

		logger.Info("Spotted Medium page, rerunning with puppeteer")
		puppeteerMatch = true
		extendedScrollTime = true
	case strings.HasSuffix(page.Curi.TrimmedPath, "/archive") && feedGenerator == FeedGeneratorSubstack:
		logger.Info("Spotted Substack archives, rerunning with puppeteer")
		puppeteerMatch = true
		maybeValidate = func(page *rod.Page) error {
			element, err := page.Sleeper(rod.NotFoundSleeper).Element(".portable-archive")
			if err != nil {
				return errors.Join(fmt.Errorf("Substack archive element not found"), err)
			}
			links, err := element.Sleeper(rod.NotFoundSleeper).ElementsX("//a")
			if err != nil {
				return errors.Join(fmt.Errorf("Substack archive couldn't query links"), err)
			}
			if len(links) == 0 {
				return errors.New("Substack empty archive")
			}
			return nil
		}
		extendedScrollTime = true
	case htmlquery.QuerySelector(page.Document, buttondownTwitterXPath) != nil:
		logger.Info("Spotted Buttondown page, rerunning with puppeteer")
		puppeteerMatch = true
	}

	if !puppeteerMatch {
		return page, nil
	}

	puppeteerPage, err := crawlCtx.MaybePuppeteerClient.Fetch(
		page.FetchUri, feedEntryCurisTitlesMap, crawlCtx, logger, maybeFindLoadMoreButton, maybeValidate,
		extendedScrollTime,
	)
	if err != nil {
		return nil, err
	}

	if !crawlCtx.PptrFetchedCuris.Contains(page.Curi) {
		crawlCtx.PptrFetchedCuris.add(page.Curi)
		logger.Info("Puppeteer page saved")
	} else {
		logger.Info("Puppeteer page saved - canonical uri already seen")
	}

	document, err := parseHtml(puppeteerPage.Content, logger)
	if err != nil {
		return nil, err
	}
	return &htmlPage{
		pageBase: pageBase{
			Curi:     page.Curi,
			FetchUri: page.FetchUri,
			Content:  puppeteerPage.Content,
		},
		Document:              document,
		MaybeTopScreenshot:    puppeteerPage.MaybeTopScreenshot,
		MaybeBottomScreenshot: puppeteerPage.MaybeBottomScreenshot,
	}, nil
}
