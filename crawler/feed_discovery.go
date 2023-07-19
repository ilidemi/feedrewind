package crawler

import (
	"regexp"
	"strings"
	"time"

	"github.com/antchfx/htmlquery"
)

type DiscoveredSingleFeed struct {
	StartPage DiscoveredStartPage
	Feed      DiscoveredFetchedFeed
}

type DiscoveredStartPage struct {
	Url      string
	FinalUrl string
	Content  string
}

type DiscoveredFetchedFeed struct {
	Title    string
	Url      string
	FinalUrl string
	Content  string
}

type DiscoveredMultipleFeeds struct {
	StartPage DiscoveredStartPage
	Feeds     []DiscoveredFeed
}

type DiscoveredFeed struct {
	Title string
	Url   string
}

type DiscoverFeedsErrorNotAUrl struct{}

type DiscoverFeedsErrorCouldNotReach struct{}

type DiscoverFeedsErrorNoFeeds struct{}

type DiscoverFeedsErrorBadFeed struct{}

type DiscoverFeedsResult interface {
	discoverFeedsResultTag()
}

func (*DiscoveredSingleFeed) discoverFeedsResultTag()            {}
func (*DiscoveredMultipleFeeds) discoverFeedsResultTag()         {}
func (*DiscoverFeedsErrorNotAUrl) discoverFeedsResultTag()       {}
func (*DiscoverFeedsErrorCouldNotReach) discoverFeedsResultTag() {}
func (*DiscoverFeedsErrorNoFeeds) discoverFeedsResultTag()       {}
func (*DiscoverFeedsErrorBadFeed) discoverFeedsResultTag()       {}

var commentsFeedRegex *regexp.Regexp
var atomUrlRegex *regexp.Regexp
var rssUrlRegex *regexp.Regexp

func init() {
	commentsFeedRegex = regexp.MustCompile("/comments/(feed|default)/?$")
	atomUrlRegex = regexp.MustCompile("(.+)atom(.*)") // Last occurrence of "atom"
	rssUrlRegex = regexp.MustCompile("(.+)rss(.*)")   // Last occurrence of "rss"
}

const atomUrlReplacement = "$1atom$2"
const rssUrlReplacement = "$1rss$2"

func MustDiscoverFeedsAtUrl(
	startUrl string, enforceTimeout bool, crawlCtx *CrawlContext, httpClient *HttpClient, logger Logger,
) DiscoverFeedsResult {
	var fullStartUrl string
	if strings.HasPrefix(startUrl, "http://") || strings.HasPrefix(startUrl, "https://") {
		fullStartUrl = startUrl
	} else if strings.Contains(startUrl, ".") {
		fullStartUrl = "http://" + startUrl
	} else {
		return &DiscoverFeedsErrorNotAUrl{}
	}

	startLink, ok := ToCanonicalLink(fullStartUrl, logger, nil)
	if !ok {
		logger.Info("Bad start url: %s", startUrl)
		if fullStartUrl == startUrl {
			return &DiscoverFeedsErrorCouldNotReach{}
		} else {
			return &DiscoverFeedsErrorNotAUrl{}
		}
	}

	if strings.HasSuffix(startLink.Uri.Host, "substack.com") &&
		(startLink.Uri.Path == "" || startLink.Uri.Path == "/") {
		logger.Info("Substack link detected, going to feed right away to avoid CloudFlare: %s", fullStartUrl)
		feedUrl := strings.TrimRight(fullStartUrl, "/") + "/feed"
		startLink, _ = ToCanonicalLink(feedUrl, logger, nil)
	}

	// TODO mock_progress_logger
	var startResult page
	var result DiscoverFeedsResult
	func() {
		defer func() {
			if err := recover(); err != nil {
				logger.Info("Error while getting start_link: %v", err)
				result = &DiscoverFeedsErrorCouldNotReach{}
			}
		}()

		startResult = mustCrawlFeedWithTimeout(startLink, enforceTimeout, crawlCtx, httpClient, logger)

		if startResult.content == "" {
			logger.Info("Page without content: %+v", startResult)
			result = &DiscoverFeedsErrorNoFeeds{}
			return
		}

		// TODO: DiscoverFeedsErrorCouldNotReach if not a page
	}()
	if result != nil {
		return result
	}

	if isFeed(startResult.content, logger) {
		parsedFeed, err := ParseFeed(startResult.content, startLink.Uri, logger)
		if err != nil {
			logger.Info("Parse feed error: %v", err)
			return &DiscoverFeedsErrorBadFeed{}
		}

		feed := DiscoveredFetchedFeed{
			Title:    parsedFeed.Title,
			Url:      startLink.Url,
			FinalUrl: startResult.fetchUri.String(),
			Content:  startResult.content,
		}
		return &DiscoveredSingleFeed{
			StartPage: DiscoveredStartPage{}, //nolint:exhaustruct
			Feed:      feed,
		}
	} else if startResult.document == nil {
		logger.Info("Page without document")
		return &DiscoverFeedsErrorNoFeeds{}
	} else {
		startPage := DiscoveredStartPage{
			Url:      startLink.Url,
			FinalUrl: startResult.fetchUri.String(),
			Content:  startResult.content,
		}

		linkNodes := htmlquery.Find(
			startResult.document,
			"//*[self::a or self::area or self::link][@rel='alternate'][@type='application/rss+xml' or @type='application/atom+xml']",
		)
		var feeds []DiscoveredFeed
		for _, linkNode := range linkNodes {
			var title string
			switch linkNode.Data {
			case "a":
				title = innerText(linkNode)
			case "area":
				title = findAttr(linkNode, "alt")
			case "link":
				title = findAttr(linkNode, "title")
			default:
				// unreachable
			}

			href := findAttr(linkNode, "href")

			canonicalLink, ok := ToCanonicalLink(href, logger, startLink.Uri)
			if !ok {
				continue
			}

			if strings.HasSuffix(canonicalLink.Url, "?alt=rss") {
				continue
			}
			if commentsFeedRegex.MatchString(canonicalLink.Uri.Path) {
				continue
			}

			feeds = append(feeds, DiscoveredFeed{
				Title: title,
				Url:   canonicalLink.Url,
			})
		}

		var dedupFeeds []DiscoveredFeed
		seenTitles := make(map[string]bool)
		seenUrls := make(map[string]bool)
		for _, feed := range feeds {
			if seenUrls[feed.Url] {
				continue
			}

			lowercaseUrl := strings.ToLower(feed.Url)
			if strings.Contains(lowercaseUrl, "atom") &&
				seenUrls[atomUrlRegex.ReplaceAllString(lowercaseUrl, rssUrlReplacement)] { // atom -> rss
				continue
			}
			if strings.Contains(lowercaseUrl, "rss") &&
				seenUrls[rssUrlRegex.ReplaceAllString(lowercaseUrl, atomUrlReplacement)] { // rss -> atom
				continue
			}

			lowercaseTitle := strings.ToLower(feed.Title)
			if lowercaseTitle == "atom" && seenTitles["rss"] {
				continue
			}
			if lowercaseTitle == "rss" && seenTitles["atom"] {
				continue
			}

			dedupFeeds = append(dedupFeeds, feed)
			seenTitles[lowercaseTitle] = true
			seenUrls[lowercaseUrl] = true
		}

		for _, feed := range dedupFeeds {
			lowercaseTitle := strings.ToLower(feed.Title)
			if feed.Title == "" || lowercaseTitle == "rss" || lowercaseTitle == "atom" {
				feed.Title = findTitle(startResult.document)
			}
			if feed.Title == "" {
				feed.Title = startResult.fetchUri.Host
			}
		}

		if len(dedupFeeds) == 0 {
			return &DiscoverFeedsErrorNoFeeds{}
		} else if len(dedupFeeds) == 1 {
			singleFeedResult := FetchFeedAtUrl(
				dedupFeeds[0].Url, enforceTimeout, crawlCtx, httpClient, logger,
			)
			switch r := singleFeedResult.(type) {
			case *FetchedPage:
				parsedFeed, err := ParseFeed(r.Page.content, r.Page.fetchUri, logger)
				if err != nil {
					return &DiscoverFeedsErrorBadFeed{}
				}
				fetchedFeed := DiscoveredFetchedFeed{
					Title:    parsedFeed.Title,
					Url:      dedupFeeds[0].Url,
					FinalUrl: r.Page.fetchUri.String(),
					Content:  r.Page.content,
				}
				return &DiscoveredSingleFeed{
					StartPage: startPage,
					Feed:      fetchedFeed,
				}
			case *FetchFeedErrorBadFeed:
				return &DiscoverFeedsErrorBadFeed{}
			case *FetchFeedErrorCouldNotReach:
				return &DiscoverFeedsErrorCouldNotReach{}
			default:
				panic("unknown fetch feed result type")
			}
		} else {
			return &DiscoveredMultipleFeeds{
				StartPage: startPage,
				Feeds:     dedupFeeds,
			}
		}
	}
}

type FetchedPage struct {
	Page page
}

type FetchFeedErrorBadFeed struct{}

type FetchFeedErrorCouldNotReach struct{}

type FetchFeedResult interface {
	fetchedFeedTag()
}

func (*FetchedPage) fetchedFeedTag()                 {}
func (*FetchFeedErrorBadFeed) fetchedFeedTag()       {}
func (*FetchFeedErrorCouldNotReach) fetchedFeedTag() {}

func FetchFeedAtUrl(
	feedUrl string, enforceTimeout bool, crawlCtx *CrawlContext, httpClient *HttpClient, logger Logger,
) FetchFeedResult {
	// TODO mock progress logger

	feedLink, ok := ToCanonicalLink(feedUrl, logger, nil)
	if !ok {
		logger.Info("Bad feed url: %s", feedUrl)
		return &FetchFeedErrorBadFeed{}
	}

	var result FetchFeedResult
	func() {
		defer func() {
			if err := recover(); err != nil {
				logger.Info("Error while getting start_link: %v", err)
				result = &FetchFeedErrorCouldNotReach{}
			}
		}()

		crawlResult := mustCrawlFeedWithTimeout(feedLink, enforceTimeout, crawlCtx, httpClient, logger)

		// TODO: DiscoverFeedsErrorBadFeed if not a page
		if crawlResult.content == "" {
			logger.Info("Unexpected crawl result")
			result = &FetchFeedErrorBadFeed{}
			return
		}

		if !isFeed(crawlResult.content, logger) {
			logger.Info("Page is not a feed")
			result = &FetchFeedErrorBadFeed{}
			return
		}

		result = &FetchedPage{Page: crawlResult}
	}()
	return result
}

// TODO ProgressLogger
func mustCrawlFeedWithTimeout(
	link Link, enforceTimeout bool, crawlCtx *CrawlContext, httpClient *HttpClient, logger Logger,
) page {
	if enforceTimeout {
		type crawlResult struct {
			Page  page
			Error error
		}
		ch := make(chan crawlResult)
		go func() {
			page, err := crawlRequest(link, true, crawlCtx, httpClient, logger)
			ch <- crawlResult{
				Page:  page,
				Error: err,
			}
		}()

		select {
		case <-time.After(10 * time.Second):
			panic("timeout")
		case result := <-ch:
			if result.Error != nil {
				panic(result.Error)
			}
			return result.Page
		}
	} else {
		page, err := crawlRequest(link, true, crawlCtx, httpClient, logger)
		if err != nil {
			panic(err)
		}
		return page
	}
}
