package crawler

import (
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/antchfx/htmlquery"
)

type DiscoveredSingleFeed struct {
	MaybeStartPage *DiscoveredStartPage
	Feed           DiscoveredFetchedFeed
}

type DiscoveredStartPage struct {
	Url      string
	FinalUrl string
	Content  string
}

type DiscoveredFetchedFeed struct {
	Title      string
	Url        string
	FinalUrl   string
	Content    string
	ParsedFeed *ParsedFeed
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

type DiscoverFeedsErrorCouldNotReach struct {
	Error error
}

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

func DiscoverFeedsAtUrl(
	startUrl string, enforceTimeout bool, crawlCtx *CrawlContext, logger Logger,
) DiscoverFeedsResult {
	var fullStartUrl string
	switch {
	case strings.HasPrefix(startUrl, "http://") || strings.HasPrefix(startUrl, "https://"):
		fullStartUrl = startUrl
	case strings.Contains(startUrl, "."):
		fullStartUrl = "http://" + startUrl
	default:
		return &DiscoverFeedsErrorNotAUrl{}
	}

	startLink, ok := ToCanonicalLink(fullStartUrl, logger, nil)
	if !ok {
		logger.Info("Bad start url: %s", startUrl)
		if fullStartUrl == startUrl {
			return &DiscoverFeedsErrorCouldNotReach{
				Error: errors.New("couldn't canonicalize start url"),
			}
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

	startPage, err := crawlFeedWithTimeout(startLink, enforceTimeout, crawlCtx, logger)
	if errors.Is(err, ErrNotAFeedOrHtmlPage) {
		logger.Info("Page is not a feed or html: %s", startLink.Url)
		return &DiscoverFeedsErrorNoFeeds{}
	} else if err != nil {
		logger.Info("Error while getting start_link: %v", err)
		return &DiscoverFeedsErrorCouldNotReach{
			Error: err,
		}
	}

	curiEqCfg := NewCanonicalEqualityConfig()
	switch p := startPage.(type) {
	case *feedPage:
		parsedFeed, err := ParseFeed(p.Content, startLink.Uri, logger)
		if err != nil {
			logger.Info("Parse feed error: %v", err)
			return &DiscoverFeedsErrorBadFeed{}
		}

		title := parsedFeed.Title
		if CanonicalUriEqual(p.Curi, HardcodedDanLuuFeed, &curiEqCfg) {
			title = HardcodedDanLuuFeedName
		}

		feed := DiscoveredFetchedFeed{
			Title:      title,
			Url:        startLink.Url,
			FinalUrl:   p.FetchUri.String(),
			Content:    p.Content,
			ParsedFeed: parsedFeed,
		}
		return &DiscoveredSingleFeed{
			MaybeStartPage: nil,
			Feed:           feed,
		}
	case *htmlPage:
		startPage := DiscoveredStartPage{
			Url:      startLink.Url,
			FinalUrl: p.FetchUri.String(),
			Content:  p.Content,
		}

		linkNodes := htmlquery.Find(
			p.Document,
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

		for i := range dedupFeeds {
			feed := &dedupFeeds[i]
			lowercaseTitle := strings.ToLower(feed.Title)
			if feed.Title == "" || lowercaseTitle == "rss" || lowercaseTitle == "atom" {
				feed.Title = findTitle(p.Document)
			}
			if feed.Title == "" {
				feed.Title = p.FetchUri.Host
			}
		}

		switch len(dedupFeeds) {
		case 0:
			return &DiscoverFeedsErrorNoFeeds{}
		case 1:
			singleFeedResult := FetchFeedAtUrl(dedupFeeds[0].Url, enforceTimeout, crawlCtx, logger)
			switch r := singleFeedResult.(type) {
			case *FetchedPage:
				parsedFeed, err := ParseFeed(r.Page.Content, r.Page.FetchUri, logger)
				if err != nil {
					return &DiscoverFeedsErrorBadFeed{}
				}

				title := parsedFeed.Title
				if CanonicalUriEqual(r.Page.Curi, HardcodedDanLuuFeed, &curiEqCfg) {
					title = HardcodedDanLuuFeedName
				}

				fetchedFeed := DiscoveredFetchedFeed{
					Title:      title,
					Url:        dedupFeeds[0].Url,
					FinalUrl:   r.Page.FetchUri.String(),
					Content:    r.Page.Content,
					ParsedFeed: parsedFeed,
				}
				return &DiscoveredSingleFeed{
					MaybeStartPage: &startPage,
					Feed:           fetchedFeed,
				}
			case *FetchFeedErrorBadFeed:
				return &DiscoverFeedsErrorBadFeed{}
			case *FetchFeedErrorCouldNotReach:
				return &DiscoverFeedsErrorCouldNotReach{
					Error: r.Error,
				}
			default:
				panic("unknown fetch feed result type")
			}
		default:
			return &DiscoveredMultipleFeeds{
				StartPage: startPage,
				Feeds:     dedupFeeds,
			}
		}
	default:
		panic("Unknown page type")
	}
}

type FetchedPage struct {
	Page *feedPage
}

type FetchFeedErrorBadFeed struct{}

type FetchFeedErrorCouldNotReach struct {
	Error error
}

type FetchFeedResult interface {
	fetchedFeedTag()
}

func (*FetchedPage) fetchedFeedTag()                 {}
func (*FetchFeedErrorBadFeed) fetchedFeedTag()       {}
func (*FetchFeedErrorCouldNotReach) fetchedFeedTag() {}

func FetchFeedAtUrl(
	feedUrl string, enforceTimeout bool, crawlCtx *CrawlContext, logger Logger,
) FetchFeedResult {
	feedLink, ok := ToCanonicalLink(feedUrl, logger, nil)
	if !ok {
		logger.Info("Bad feed url: %s", feedUrl)
		return &FetchFeedErrorBadFeed{}
	}

	crawlResult, err := crawlFeedWithTimeout(feedLink, enforceTimeout, crawlCtx, logger)
	if errors.Is(err, ErrNotAFeedOrHtmlPage) {
		logger.Info("Page is not a feed: %s", feedLink.Url)
		return &FetchFeedErrorBadFeed{}
	} else if err != nil {
		logger.Info("Error when fetching a feed at %s: %v", feedLink.Url, err)
		return &FetchFeedErrorCouldNotReach{
			Error: err,
		}
	} else if feedPage, ok := crawlResult.(*feedPage); ok {
		return &FetchedPage{Page: feedPage}
	} else {
		logger.Info("Page is not a feed: %s", feedLink.Url)
		return &FetchFeedErrorBadFeed{}
	}
}

var ErrTimeout = errors.New("timeout")

func crawlFeedWithTimeout(
	link *Link, enforceTimeout bool, crawlCtx *CrawlContext, logger Logger,
) (feedOrHtmlPage, error) {
	if enforceTimeout {
		type CrawlResult struct {
			Page  feedOrHtmlPage
			Error error
		}
		ch := make(chan CrawlResult)
		go func() {
			page, err := crawlFeedOrHtmlPage(link, crawlCtx, logger)
			ch <- CrawlResult{
				Page:  page,
				Error: err,
			}
		}()

		select {
		case <-time.After(10 * time.Second):
			return nil, ErrTimeout
		case result := <-ch:
			return result.Page, result.Error
		}
	} else {
		return crawlFeedOrHtmlPage(link, crawlCtx, logger)
	}
}
