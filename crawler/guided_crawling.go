package crawler

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"feedrewind.com/oops"
	"feedrewind.com/util"

	"github.com/temoto/robotstxt"
)

type Feed struct {
	Title    string
	Url      string
	FinalUrl string
	Content  string
}

type GuidedCrawlResult struct {
	FeedResult       FeedResult
	CuriEqCfg        *CanonicalEqualityConfig
	HistoricalResult *HistoricalResult
	HistoricalError  error
	HardcodedError   error
}

type FeedResult struct {
	Url                  string
	Links                int
	MatchingTitles       string
	MatchingTitlesStatus Status
}

type Status string

const (
	StatusNone    = "none"
	StatusFailure = "failure"
	StatusNeutral = "neutral"
	StatusSuccess = "success"
)

type HistoricalResult struct {
	BlogLink               Link
	MainLink               Link
	Pattern                string
	Links                  []*titledLink
	DiscardedFeedEntryUrls []string
	PostCategories         []HistoricalBlogPostCategory
	Extra                  []string
}

type HistoricalBlogPostCategory struct {
	Name      string
	IsTop     bool
	PostLinks []Link
}

func areCategoriesEqual(
	categories1, categories2 []pristineHistoricalBlogPostCategory, curiEqCfg *CanonicalEqualityConfig,
) bool {
	if len(categories1) != len(categories2) {
		return false
	}
	for i, category := range categories1 {
		otherCategory := categories2[i]
		if category.Name != otherCategory.Name {
			return false
		}
		if category.IsTop != otherCategory.IsTop {
			return false
		}
		if len(category.PostLinks) != len(otherCategory.PostLinks) {
			return false
		}
		for j, link := range category.PostLinks {
			otherLink := otherCategory.PostLinks[j]
			if !CanonicalUriEqual(link.Curi(), otherLink.Curi(), curiEqCfg) {
				return false
			}
		}
	}

	return true
}

func GuidedCrawl(
	maybeStartPage *DiscoveredStartPage, feed Feed, crawlCtx *CrawlContext, logger Logger,
) (*GuidedCrawlResult, error) {
	guidedCrawlResult := GuidedCrawlResult{} //nolint:exhaustruct
	feedResult := &guidedCrawlResult.FeedResult

	logger.Info("Feed url: %s", feed.FinalUrl)
	feedResult.Url = fmt.Sprintf(`<a href="%s">feed</a>`, feed.FinalUrl)

	feedLink, _ := ToCanonicalLink(feed.Url, logger, nil)
	feedFinalLink, _ := ToCanonicalLink(feed.FinalUrl, logger, nil)

	parsedFeed, err := ParseFeed(feed.Content, feedFinalLink.Uri, logger)
	if err != nil {
		return nil, err
	}
	feedResult.Links = parsedFeed.EntryLinks.Length
	if parsedFeed.EntryLinks.Length == 0 {
		return nil, oops.New("Feed is empty")
	} else if parsedFeed.EntryLinks.Length == 1 {
		return nil, oops.New("Feed only has 1 item")
	}

	crawlCtx.RobotsClient = NewRobotsClient(feedLink.Uri, crawlCtx.HttpClient, logger)

	var startPageLink *Link
	var startPageFinalLink *Link
	var startPage *htmlPage
	if maybeStartPage != nil {
		logger.Info("Discovered start page is present")
		var ok1, ok2 bool
		startPageLink, ok1 = ToCanonicalLink(maybeStartPage.Url, logger, nil)
		if !ok1 {
			return nil, oops.Newf("Bad start page url: %v", maybeStartPage.Url)
		}
		startPageFinalLink, ok2 = ToCanonicalLink(maybeStartPage.FinalUrl, logger, nil)
		if !ok2 {
			return nil, oops.Newf("Bad start page final url: %v", maybeStartPage.FinalUrl)
		}
		startPageDocument, err := parseHtml(maybeStartPage.Content, logger)
		if err != nil {
			return nil, err
		}
		startPage = &htmlPage{
			pageBase: pageBase{
				Curi:     startPageFinalLink.Curi,
				FetchUri: startPageFinalLink.Uri,
				Content:  maybeStartPage.Content,
			},
			Document:              startPageDocument,
			MaybeTopScreenshot:    nil,
			MaybeBottomScreenshot: nil,
		}
	} else {
		logger.Info("Discovered start page is absent")
		startPageLink, startPage, err = getFeedStartPage(feedLink, parsedFeed, crawlCtx, logger)
		if err != nil {
			return nil, err
		}
		var ok bool
		startPageFinalLink, ok = ToCanonicalLink(startPage.FetchUri.String(), logger, nil)
		if !ok {
			panic(oops.Newf("Couldn't parse start page fetch uri: %s", startPage.FetchUri))
		}
	}

	feedEntryLinksByHost := make(map[string][]Link)
	for _, entryLink := range parsedFeed.EntryLinks.ToSlice() {
		feedEntryLinksByHost[entryLink.Uri.Host] =
			append(feedEntryLinksByHost[entryLink.Uri.Host], entryLink.Link)
	}

	sameHosts := make(map[string]bool)
	startLinks := []*Link{startPageLink, feedLink}
	finalLinks := []*Link{startPageFinalLink, feedFinalLink}
	for i := 0; i < 2; i++ {
		if CanonicalUriPathEqual(startLinks[i].Curi, finalLinks[i].Curi) &&
			(len(feedEntryLinksByHost[startLinks[i].Uri.Host]) > 0 ||
				len(feedEntryLinksByHost[finalLinks[i].Uri.Host]) > 0) {

			sameHosts[startLinks[i].Uri.Host] = true
			sameHosts[finalLinks[i].Uri.Host] = true
		}
	}

	feedEntryLinksSameHost := false
	for host := range feedEntryLinksByHost {
		if sameHosts[host] {
			feedEntryLinksSameHost = true
			break
		}
	}
	if !feedEntryLinksSameHost {
		logger.Info("Feed entry links come from a different host than feed and start page")
		sortedFeedEntryHosts := util.Keys(feedEntryLinksByHost)
		slices.SortFunc(sortedFeedEntryHosts, func(a, b string) int {
			return len(feedEntryLinksByHost[b]) - len(feedEntryLinksByHost[a]) // descending
		})
		entryLinkFromPopularHost := feedEntryLinksByHost[sortedFeedEntryHosts[0]][0]
		entryPage, err := crawlPage(&entryLinkFromPopularHost, false, crawlCtx, logger)
		err2 := crawlCtx.ProgressLogger.SaveStatus()
		if err2 != nil {
			return nil, err2
		}
		if err != nil {
			return nil, err
		}

		entryPageBase := entryPage.base()
		if entryLinkFromPopularHost.Curi.TrimmedPath == entryPageBase.Curi.TrimmedPath {
			sameHosts[entryLinkFromPopularHost.Uri.Host] = true
			sameHosts[entryPageBase.FetchUri.Host] = true
		} else {
			logger.Info("Paths don't match: %s, %s", entryLinkFromPopularHost.Uri, entryPageBase.FetchUri)
		}
	}

	logger.Info("Same hosts: %v", sameHosts)

	curiEqCfg := CanonicalEqualityConfig{
		SameHosts:         sameHosts,
		ExpectTumblrPaths: parsedFeed.Generator == FeedGeneratorTumblr,
	}
	crawlCtx.FetchedCuris.updateEqualityConfig(&curiEqCfg)
	crawlCtx.PptrFetchedCuris.updateEqualityConfig(&curiEqCfg)
	guidedCrawlResult.CuriEqCfg = &curiEqCfg

	feedEntryCurisTitlesMap := NewCanonicalUriMap[MaybeLinkTitle](&curiEqCfg)
	for _, entryLink := range parsedFeed.EntryLinks.ToSlice() {
		feedEntryCurisTitlesMap.Add(entryLink.Link, entryLink.MaybeTitle)
	}
	initialBlogLink := startPageFinalLink
	if parsedFeed.RootLink != nil {
		initialBlogLink = parsedFeed.RootLink
	}

	var historicalResult *HistoricalResult
	var historicalMaybeTitledLinks []*maybeTitledLink
	var historicalError error
	shouldUseFeed := (parsedFeed.EntryLinks.Length > 50 &&
		parsedFeed.EntryLinks.Length%100 != 0 &&
		!CanonicalUriEqual(feedLink.Curi, HardcodedDanLuuFeed, &curiEqCfg)) ||
		CanonicalUriEqual(feedLink.Curi, hardcodedInkAndSwitchFeed, &curiEqCfg)
	if !shouldUseFeed {
		var postprocessedResult *postprocessedResult
		if parsedFeed.Generator != FeedGeneratorTumblr {
			postprocessedResult, historicalError = guidedCrawlHistorical(
				startPage, parsedFeed, feedEntryCurisTitlesMap, initialBlogLink, crawlCtx, &curiEqCfg, logger,
			)
		} else {
			postprocessedResult, historicalError = getTumblrApiHistorical(
				parsedFeed.RootLink.Uri.Hostname(), crawlCtx, logger,
			)
		}

		if postprocessedResult != nil {
			var historicalCuris []CanonicalUri
			for _, link := range postprocessedResult.Links {
				historicalCuris = append(historicalCuris, link.Curi())
			}
			historicalCurisSet := NewCanonicalUriSet(historicalCuris, &curiEqCfg)

			var discardedFeedEntryUrls []string
			for _, entryLink := range parsedFeed.EntryLinks.ToSlice() {
				if !historicalCurisSet.Contains(entryLink.Curi) {
					discardedFeedEntryUrls = append(discardedFeedEntryUrls, entryLink.Url)
				}
			}

			historicalMaybeTitledLinks = make([]*maybeTitledLink, len(postprocessedResult.Links))
			for i, link := range postprocessedResult.Links {
				historicalMaybeTitledLinks[i] = link.Unwrap()
			}
			postCategories := PristineHistoricalBlogPostCategoriesUnwrap(postprocessedResult.PostCategories)
			historicalResult = &HistoricalResult{
				BlogLink:               *postprocessedResult.MainLnk.Unwrap(),
				MainLink:               *postprocessedResult.MainLnk.Unwrap(),
				Pattern:                postprocessedResult.Pattern,
				Links:                  nil,
				DiscardedFeedEntryUrls: discardedFeedEntryUrls,
				PostCategories:         postCategories,
				Extra:                  postprocessedResult.Extra,
			}
		}
	} else {
		logger.Info("Feed is long with %d entries", parsedFeed.EntryLinks.Length)

		var postCategories []pristineHistoricalBlogPostCategory
		var postCategoriesExtra []string
		if CanonicalUriEqual(feedLink.Curi, hardcodedPaulGraham, &curiEqCfg) {
			postCategories = hardcodedPaulGrahamCategories
			postCategoriesStr := categoryCountsString(postCategories)
			logger.Info("Categories: %s", postCategoriesStr)
			appendLogLinef(&postCategoriesExtra, "categories: %s", postCategoriesStr)
		}

		historicalResult = &HistoricalResult{
			BlogLink:               *initialBlogLink,
			MainLink:               *feedLink,
			Pattern:                "long_feed",
			Links:                  nil,
			DiscardedFeedEntryUrls: nil,
			PostCategories:         PristineHistoricalBlogPostCategoriesUnwrap(postCategories),
			Extra:                  postCategoriesExtra,
		}

		if CanonicalUriEqual(feedLink.Curi, hardcodedInkAndSwitchFeed, &curiEqCfg) {
			historicalMaybeTitledLinks =
				extractInkAndSwitch(parsedFeed.EntryLinks.ToMaybeTitledSlice(), logger)
		} else {
			historicalMaybeTitledLinks = parsedFeed.EntryLinks.ToMaybeTitledSlice()
		}
	}

	if historicalResult != nil {
		historicalResult.Links, err = fetchMissingTitles(
			historicalMaybeTitledLinks, &parsedFeed.EntryLinks, &feedEntryCurisTitlesMap,
			parsedFeed.Generator, &curiEqCfg, crawlCtx, logger,
		)
		if err != nil {
			return nil, err
		}
		titleSources := countLinkTitleSources(historicalResult.Links)
		historicalResult.Extra = append(historicalResult.Extra, fmt.Sprintf("title_xpaths: %s", titleSources))
		historicalCuris := ToCanonicalUris(historicalMaybeTitledLinks)
		feedLinksMatchingResult, ok := parsedFeed.EntryLinks.sequenceMatch(historicalCuris, &curiEqCfg)
		feedTitlesPresent := true
		for _, link := range parsedFeed.EntryLinks.ToSlice() {
			if link.MaybeTitle == nil {
				feedTitlesPresent = false
			}
		}
		if ok && feedTitlesPresent {
			type TitleMismatch struct {
				FeedLinkTitle   LinkTitle
				ResultLinkTitle LinkTitle
			}
			var matchingTitleCount int
			var titleMismatches []TitleMismatch
			for i := 0; i < len(feedLinksMatchingResult); i++ {
				feedLinkTitle := feedLinksMatchingResult[i].MaybeTitle
				resultLinkTitle := historicalResult.Links[i].Title
				if feedLinkTitle == nil || resultLinkTitle.EqualizedValue == feedLinkTitle.EqualizedValue {
					matchingTitleCount++
				} else {
					titleMismatches = append(titleMismatches, TitleMismatch{
						FeedLinkTitle:   *feedLinkTitle,
						ResultLinkTitle: resultLinkTitle,
					})
				}
			}
			for _, titleMismatch := range titleMismatches {
				logger.Info(
					"Title mismatch with feed: %s (%s) != feed %q",
					titleMismatch.ResultLinkTitle.Value, titleMismatch.ResultLinkTitle.Source,
					titleMismatch.FeedLinkTitle.Value,
				)
			}
			if matchingTitleCount == len(feedLinksMatchingResult) {
				feedResult.MatchingTitlesStatus = StatusSuccess
				feedResult.MatchingTitles = fmt.Sprint(matchingTitleCount)
			} else {
				feedResult.MatchingTitlesStatus = StatusFailure
				feedResult.MatchingTitles =
					fmt.Sprintf("%d (%d)", matchingTitleCount, len(feedLinksMatchingResult))
			}
		} else {
			feedResult.MatchingTitlesStatus = StatusNeutral
		}
	} else {
		feedResult.MatchingTitlesStatus = StatusNeutral
	}

	guidedCrawlResult.HistoricalResult = historicalResult
	guidedCrawlResult.HistoricalError = historicalError

	return &guidedCrawlResult, nil
}

func getFeedStartPage(
	feedLink *Link, parsedFeed *ParsedFeed, crawlCtx *CrawlContext, logger Logger,
) (*Link, *htmlPage, error) {
	if parsedFeed.RootLink != nil {
		startPageLink := parsedFeed.RootLink
		startPage, err := crawlHtmlPage(startPageLink, crawlCtx, logger)
		err2 := crawlCtx.ProgressLogger.SaveStatus()
		if err2 != nil {
			return nil, nil, err2
		}
		if err != nil {
			logger.Info("Couldn't fetch root link: %v", err)
		} else {
			logger.Info("Using start page from root link: %s", startPageLink.Url)
			return startPageLink, startPage, nil
		}
	}

	logger.Info("Trying to discover start page")
	possibleStartUri := feedLink.Uri
	for {
		if possibleStartUri.Path == "" {
			return nil, nil, oops.Newf("Couldn't discover start link from %s", feedLink.Url)
		}

		possibleStartUri.Path = possibleStartUri.Path[:strings.LastIndex(possibleStartUri.Path, "/")]
		possibleStartPageLink, _ := ToCanonicalLink(possibleStartUri.String(), logger, nil)
		logger.Info("Possible start link: %s", possibleStartPageLink.Url)
		possibleStartPage, err := crawlHtmlPage(possibleStartPageLink, crawlCtx, logger)
		err2 := crawlCtx.ProgressLogger.SaveStatus()
		if err2 != nil {
			return nil, nil, err2
		}
		if err != nil {
			logger.Info("Couldn't fetch: %v", err)
			continue
		}

		return possibleStartPageLink, possibleStartPage, nil
	}
}

type crawlHistoricalResult interface {
	mainLink() pristineLink
	speculativeCount() int
	isSame(other crawlHistoricalResult, curiEqCfg *CanonicalEqualityConfig) bool
}

func areLinksEqual(links1, links2 []*pristineMaybeTitledLink, curiEqCfg *CanonicalEqualityConfig) bool {
	if len(links1) != len(links2) {
		return false
	}
	for i, link := range links1 {
		otherLink := links2[i]
		if !CanonicalUriEqual(link.Curi(), otherLink.Curi(), curiEqCfg) {
			return false
		}
		if (link.MaybeTitle == nil) != (otherLink.MaybeTitle == nil) {
			return false
		}
		if link.MaybeTitle != nil && otherLink.MaybeTitle != nil &&
			link.MaybeTitle.Title.EqualizedValue != otherLink.MaybeTitle.Title.EqualizedValue {
			return false
		}
	}

	return true
}

type postprocessedResult struct {
	MainLnk                 pristineLink
	Pattern                 string
	Links                   []*pristineMaybeTitledLink
	IsMatchingFeed          bool
	PostCategories          []pristineHistoricalBlogPostCategory
	Extra                   []string
	MaybePartialPagedResult *partialPagedResult
}

func (r *postprocessedResult) mainLink() pristineLink {
	return r.MainLnk
}

func (r *postprocessedResult) speculativeCount() int {
	if r.MaybePartialPagedResult != nil {
		return r.MaybePartialPagedResult.SpeculativeCount()
	}
	return len(r.Links)
}

func (r *postprocessedResult) isSame(other crawlHistoricalResult, curiEqCfg *CanonicalEqualityConfig) bool {
	ppOther, ok := other.(*postprocessedResult)
	if !ok {
		return false
	}
	return areLinksEqual(r.Links, ppOther.Links, curiEqCfg) &&
		r.IsMatchingFeed == ppOther.IsMatchingFeed &&
		areCategoriesEqual(r.PostCategories, ppOther.PostCategories, curiEqCfg) &&
		r.MaybePartialPagedResult == nil &&
		ppOther.MaybePartialPagedResult == nil
}

type linkOrHtmlPage interface {
	linkOrHtmlPageTag()
}

func (*pristineLink) linkOrHtmlPageTag() {}
func (*htmlPage) linkOrHtmlPageTag()     {}

type guidedCrawlQueue []linkOrHtmlPage

var archivesRegex *regexp.Regexp
var mainPageRegex *regexp.Regexp
var likelyPostRegex *regexp.Regexp

func init() {
	archivesRegex = regexp.MustCompile(`/(?:[a-z-]*archives?|posts?|all(?:-[a-z]+)?)(?:\.[a-z]+)?$`)
	mainPageRegex = regexp.MustCompile(`/(?:(?:blog|articles|writing|journal|essays)(?:\.[a-z]+)?|index)$`)
	likelyPostRegex = regexp.MustCompile(`/\d{4}(/\d{2})?(/\d{2})?$`)
}

var ErrPatternNotDetected = errors.New("pattern not detected")

func guidedCrawlHistorical(
	startPage *htmlPage, parsedFeed *ParsedFeed, feedEntryCurisTitlesMap CanonicalUriMap[MaybeLinkTitle],
	initialBlogLink *Link, crawlCtx *CrawlContext, curiEqCfg *CanonicalEqualityConfig, logger Logger,
) (*postprocessedResult, error) {
	progressLogger := crawlCtx.ProgressLogger

	seenCurisSet := newGuidedSeenCurisSet(curiEqCfg)
	allowedHosts := curiEqCfg.SameHosts
	if len(allowedHosts) == 0 {
		allowedHosts = map[string]bool{startPage.FetchUri.Host: true}
	}

	startPageAllLinks := extractLinks(
		startPage.Document, startPage.FetchUri, nil, crawlCtx.Redirects, logger,
		includeXPathAndClassXPath,
	)
	var startPageAllowedHostsLinks []*Link
	for _, link := range startPageAllLinks {
		if allowedHosts[link.Uri.Host] {
			startPageAllowedHostsLinks = append(startPageAllowedHostsLinks, &link.Link)
		}
	}

	archivesCategoriesState := newArchivesCategoriesState(initialBlogLink)

	var archivesQueue guidedCrawlQueue
	var mainPageQueue guidedCrawlQueue
	seenCurisSet.add(startPage.Curi)
	if archivesRegex.MatchString(startPage.Curi.TrimmedPath) {
		logger.Info("Start page uri matches archives: %s", startPage.FetchUri.String())
		archivesQueue = append(archivesQueue, startPage)
	} else {
		logger.Info(
			"Start page uri doesn't match archives, let it be a main page: %s", startPage.FetchUri.String(),
		)
		mainPageQueue = append(mainPageQueue, startPage)
	}

	var startPageOtherLinks []*Link
	for _, link := range startPageAllowedHostsLinks {
		if !isNewAndAllowed(link, &seenCurisSet, crawlCtx.RobotsClient, logger) {
			continue
		}

		switch {
		case archivesRegex.MatchString(link.Curi.TrimmedPath):
			seenCurisSet.add(link.Curi)
			archivesQueue = append(archivesQueue, NewPristineLink(link))
			logger.Info("Enqueued archives: %s", link.Url)
		case mainPageRegex.MatchString(link.Curi.TrimmedPath):
			seenCurisSet.add(link.Curi)
			mainPageQueue = append(mainPageQueue, NewPristineLink(link))
			logger.Info("Enqueued main page: %s", link.Url)
		default:
			startPageOtherLinks = append(startPageOtherLinks, link)
		}
	}

	if parsedFeed.Generator == FeedGeneratorSubstack && parsedFeed.RootLink != nil {
		archiveLink, _ := ToCanonicalLink("/archive", logger, parsedFeed.RootLink.Uri)
		if isNewAndAllowed(archiveLink, &seenCurisSet, crawlCtx.RobotsClient, logger) {
			seenCurisSet.add(archiveLink.Curi)
			archivesQueue = append(archivesQueue, NewPristineLink(archiveLink))
			logger.Info("Added missing substack archives: %s", archiveLink.Url)
		}
	}

	logger.Info(
		"Start page and links: %d archives, %d main page, %d others",
		len(archivesQueue), len(mainPageQueue), len(startPageOtherLinks),
	)

	guidedCtx := guidedCrawlContext{
		SeenCurisSet:            seenCurisSet,
		ArchivesCategoriesState: &archivesCategoriesState,
		FeedEntryLinks:          &parsedFeed.EntryLinks,
		FeedEntryCurisTitlesMap: feedEntryCurisTitlesMap,
		FeedGenerator:           parsedFeed.Generator,
		FeedRootLinkCuri:        parsedFeed.RootLink.Curi,
		CuriEqCfg:               curiEqCfg,
		AllowedHosts:            allowedHosts,
		HardcodedError:          nil,
	}

	result, err := guidedCrawlFetchLoop(
		[]*guidedCrawlQueue{&archivesQueue, &mainPageQueue}, nil, 1, &guidedCtx, crawlCtx, logger,
	)
	if errors.Is(err, ErrCrawlCanceled) || errors.Is(err, ErrBlogTooLong) {
		return nil, err
	} else if err == nil {
		if len(result.Links) >= 11 {
			logger.Info("Phase 1 succeeded")
			return result, nil
		} else {
			logger.Info(
				"Got a result with %d historical links but it looks too small. Continuing just in case",
				len(result.Links),
			)

			// NOTE: count will be out of sync with the next progress rect but it will also show up on the
			// admin dashboard
			err := progressLogger.LogAndSaveFetchedCount(nil)
			if err != nil {
				return nil, err
			}
		}
	}

	if parsedFeed.EntryLinks.Length < 2 {
		return nil, oops.Newf("Too few entries in feed: %d", parsedFeed.EntryLinks.Length)
	}

	feedEntryLinksSlice := parsedFeed.EntryLinks.ToSlice()
	entry1Page, err := crawlHtmlPage(&feedEntryLinksSlice[0].Link, crawlCtx, logger)
	err2 := progressLogger.SaveStatus()
	if err2 != nil {
		return nil, err2
	}
	if err != nil {
		return nil, oops.Wrap(err)
	}
	entry2Page, err := crawlHtmlPage(&feedEntryLinksSlice[1].Link, crawlCtx, logger)
	err2 = progressLogger.SaveStatus()
	if err2 != nil {
		return nil, err2
	}
	if err != nil {
		return nil, oops.Wrap(err)
	}

	var twoEntriesLinks []*pristineLink
	{
		entry1Links := extractLinks(
			entry1Page.Document, entry1Page.FetchUri, allowedHosts, crawlCtx.Redirects, logger, includeXPathNone,
		)
		entry2Links := extractLinks(
			entry2Page.Document, entry2Page.FetchUri, allowedHosts, crawlCtx.Redirects, logger, includeXPathNone,
		)
		entry1Curis := ToCanonicalUris(entry1Links)
		entry1CurisSet := NewCanonicalUriSet(entry1Curis, curiEqCfg)

		entry2CurisSet := NewCanonicalUriSet(nil, curiEqCfg)
		for _, entry2Link := range entry2Links {
			if entry2CurisSet.Contains(entry2Link.Curi) {
				continue
			}

			entry2CurisSet.add(entry2Link.Curi)
			if entry1CurisSet.Contains(entry2Link.Curi) {
				twoEntriesLinks = append(twoEntriesLinks, NewPristineLink(&entry2Link.Link))
			}
		}
	}

	var twoEntriesOtherLinks []*pristineLink
	for _, link := range twoEntriesLinks {
		if !isNewAndAllowed(link.Unwrap(), &seenCurisSet, crawlCtx.RobotsClient, logger) {
			continue
		}

		switch {
		case archivesRegex.MatchString(link.Curi().TrimmedPath):
			archivesQueue = append(archivesQueue, link)
		case mainPageRegex.MatchString(link.Curi().TrimmedPath):
			mainPageQueue = append(mainPageQueue, link)
		default:
			twoEntriesOtherLinks = append(twoEntriesOtherLinks, link)
		}
	}

	logger.Info(
		"Phase 2 (first two entries) start links: %d archives, %d main page, %d others",
		len(archivesQueue), len(mainPageQueue), len(twoEntriesOtherLinks),
	)

	result, err = guidedCrawlFetchLoop(
		[]*guidedCrawlQueue{&archivesQueue, &mainPageQueue}, result, 2, &guidedCtx, crawlCtx, logger,
	)
	phase2Ok := err == nil
	if errors.Is(err, ErrCrawlCanceled) || errors.Is(err, ErrBlogTooLong) {
		return nil, err
	} else if phase2Ok {
		if len(result.Links) >= 11 {
			logger.Info("Phase 2 succeeded")
			return result, nil
		} else {
			logger.Info(
				"Got a result with %d historical links but it looks too small. Continuing just in case",
				len(result.Links),
			)

			// NOTE: count will be out of sync with the next progress rect but it will also show up on the
			// admin dashboard
			err := progressLogger.LogAndSaveFetchedCount(nil)
			if err != nil {
				return nil, err
			}
		}
	}

	if parsedFeed.Generator == FeedGeneratorMedium {
		logger.Info("Skipping phase 3 because Medium")
		if phase2Ok {
			return result, nil
		} else {
			return nil, ErrPatternNotDetected
		}
	}

	var othersQueue guidedCrawlQueue
	var filteredTwoEntriesOtherLinks []*pristineLink
	for _, link := range twoEntriesOtherLinks {
		if !feedEntryCurisTitlesMap.Contains(link.Curi()) {
			filteredTwoEntriesOtherLinks = append(filteredTwoEntriesOtherLinks, link)
		}
	}
	var twiceFilteredTwoEntriesOtherLinks []*pristineLink
	if len(filteredTwoEntriesOtherLinks) > 10 {
		for _, link := range filteredTwoEntriesOtherLinks {
			if !likelyPostRegex.MatchString(link.Curi().TrimmedPath) {
				twiceFilteredTwoEntriesOtherLinks = append(twiceFilteredTwoEntriesOtherLinks, link)
			}
		}
		logger.Info(
			"Two entries other links: filtering %d -> %d",
			len(filteredTwoEntriesOtherLinks), len(twiceFilteredTwoEntriesOtherLinks),
		)
	} else {
		twiceFilteredTwoEntriesOtherLinks = filteredTwoEntriesOtherLinks
		logger.Info("Two entries other links: %d", len(twiceFilteredTwoEntriesOtherLinks))
	}

	for _, link := range twiceFilteredTwoEntriesOtherLinks {
		if !isNewAndAllowed(link.Unwrap(), &seenCurisSet, crawlCtx.RobotsClient, logger) {
			continue
		}
		othersQueue = append(othersQueue, link)
	}
	logger.Info("Phase 3 links from phase 2: %d", len(othersQueue))

	var filteredStartPageOtherLinks []*Link
	for _, link := range startPageOtherLinks {
		level := strings.Count(link.Curi.TrimmedPath, "/")
		if level <= 1 && !likelyPostRegex.MatchString(link.Curi.TrimmedPath) {
			filteredStartPageOtherLinks = append(filteredStartPageOtherLinks, link)
		}
	}
	areAnyFeedEntriesTopLevel := slices.ContainsFunc(
		parsedFeed.EntryLinks.ToSlice(), func(entryLink *FeedEntryLink) bool {
			return strings.Count(entryLink.Curi.TrimmedPath, "/") <= 1
		},
	)
	if areAnyFeedEntriesTopLevel {
		logger.Info("Skipping phase 1 other links because some feed entries are top level")
	} else {
		phase1OthersCount := 0
		for _, link := range filteredStartPageOtherLinks {
			if !isNewAndAllowed(link, &seenCurisSet, crawlCtx.RobotsClient, logger) {
				continue
			}
			othersQueue = append(othersQueue, NewPristineLink(link))
			phase1OthersCount++
		}
		logger.Info("Phase 3 links from phase 1: %d", phase1OthersCount)
	}

	result, err = guidedCrawlFetchLoop(
		[]*guidedCrawlQueue{&archivesQueue, &mainPageQueue, &othersQueue}, result, 3, &guidedCtx, crawlCtx,
		logger,
	)
	if errors.Is(err, ErrCrawlCanceled) || errors.Is(err, ErrBlogTooLong) {
		return nil, err
	} else if err == nil {
		logger.Info("Phase 3 succeeded")
		return result, nil
	}

	return nil, ErrPatternNotDetected
}

type guidedCrawlContext struct {
	SeenCurisSet            guidedSeenCurisSet
	ArchivesCategoriesState *archivesCategoriesState
	FeedEntryLinks          *FeedEntryLinks
	FeedEntryCurisTitlesMap CanonicalUriMap[MaybeLinkTitle]
	FeedGenerator           FeedGenerator
	FeedRootLinkCuri        CanonicalUri
	CuriEqCfg               *CanonicalEqualityConfig
	AllowedHosts            map[string]bool
	HardcodedError          error
}

type guidedSeenCurisSet struct {
	Set CanonicalUriSet
}

func newGuidedSeenCurisSet(curiEqCfg *CanonicalEqualityConfig) guidedSeenCurisSet {
	return guidedSeenCurisSet{
		Set: NewCanonicalUriSet(nil, curiEqCfg),
	}
}

func (s *guidedSeenCurisSet) contains(curi CanonicalUri) bool {
	return s.Set.Contains(removeQuery(curi))
}

func (s *guidedSeenCurisSet) add(curi CanonicalUri) {
	s.Set.add(removeQuery(curi))
}

func removeQuery(curi CanonicalUri) CanonicalUri {
	return CanonicalUri{
		Host:        curi.Host,
		Port:        curi.Port,
		Path:        curi.Path,
		TrimmedPath: curi.TrimmedPath,
		Query:       "",
	}
}

type RobotsClient struct {
	MaybeGroup            *robotstxt.Group
	MaybeBackupGroup      *robotstxt.Group
	FeedRewindBlockLogged bool
	LastRequestTimestamp  time.Time
}

func NewRobotsClient(rootUri *url.URL, httpClient HttpClient, logger Logger) *RobotsClient {
	robotsUri := *rootUri
	robotsUri.Path = "/robots.txt"
	robotsResp, err := httpClient.Request(&robotsUri, false, nil, logger)
	if err != nil {
		logger.Info("Couldn't reach robots.txt: %v", err)
		return &RobotsClient{} //nolint:exhaustruct
	} else if robotsResp.Code != "200" {
		logger.Info("Couldn't fetch robots.txt: status %s", robotsResp.Code)
		return &RobotsClient{} //nolint:exhaustruct
	}

	robotsData, err := robotstxt.FromBytes(robotsResp.Body)
	if err != nil {
		logger.Warn("Couldn't parse robots.txt: %v", err)
		return &RobotsClient{} //nolint:exhaustruct
	}

	group := robotsData.FindGroup("FeedRewindBot")
	backupGroup := robotsData.FindGroup("ACompletelyDifferentBot")
	if group.CrawlDelay > 10*time.Second {
		logger.Warn("Long crawl delay: %s (%s)", group.CrawlDelay.String(), robotsUri.String())
	} else {
		logger.Info("Crawl delay: %s (%s)", group.CrawlDelay.String(), robotsUri.String())
	}

	return &RobotsClient{
		MaybeGroup:            group,
		MaybeBackupGroup:      backupGroup,
		LastRequestTimestamp:  time.Time{}, //nolint:exhaustruct
		FeedRewindBlockLogged: false,
	}
}

func (c *RobotsClient) Test(uri *url.URL, logger Logger) bool {
	if c.MaybeGroup == nil {
		return true
	}
	result := c.MaybeGroup.Test(uri.Path)
	if !c.FeedRewindBlockLogged && !result && c.MaybeBackupGroup.Test(uri.Path) {
		logger.Warn("FeedRewindBot blocked: %s", uri.String())
		c.FeedRewindBlockLogged = true
	}
	return result
}

func (c *RobotsClient) Throttle(ctx context.Context, uri *url.URL) error {
	now := time.Now().UTC()
	if !c.LastRequestTimestamp.IsZero() {
		timeDelta := now.Sub(c.LastRequestTimestamp)
		crawlDelay := time.Second
		if c.MaybeGroup != nil && c.MaybeGroup.CrawlDelay > time.Second {
			crawlDelay = c.MaybeGroup.CrawlDelay
		}
		if uri.Host == HardcodedTheOldNewThingUri.Host &&
			strings.HasPrefix(uri.Path, HardcodedTheOldNewThingUri.Path) {
			// sorry Microsoft
			crawlDelay = 500 * time.Millisecond
		}
		if timeDelta < crawlDelay {
			sleepDelay := crawlDelay - timeDelta
			timer := time.NewTimer(sleepDelay)
			select {
			case <-ctx.Done():
				return ErrCrawlCanceled
			case <-timer.C:
			}
			now = time.Now().UTC()
		}
	}
	c.LastRequestTimestamp = now
	return nil
}

func isNewAndAllowed(
	link *Link, seenCurisSet *guidedSeenCurisSet, robotsClient *RobotsClient, logger Logger,
) bool {
	return !seenCurisSet.contains(link.Curi) && robotsClient.Test(link.Uri, logger)
}

func guidedCrawlFetchLoop(
	queues []*guidedCrawlQueue, maybeInitialResult *postprocessedResult, phaseNumber int,
	guidedCtx *guidedCrawlContext, crawlCtx *CrawlContext, logger Logger,
) (*postprocessedResult, error) {
	progressLogger := crawlCtx.ProgressLogger
	logger.Info("Guided crawl loop started (phase %d)", phaseNumber)

	var sortedResults []crawlHistoricalResult
	if maybeInitialResult != nil {
		sortedResults = append(sortedResults, maybeInitialResult)
	}

	archivesQueue := queues[0]
	mainPageQueue := queues[1]
	hadArchives := len(*archivesQueue) > 0
	archivesSeenCount := len(*archivesQueue)
	archivesProcessedCount := 0
	mainPagesSeenCount := len(*mainPageQueue)
	mainPagesProcessedCount := 0
	historicalMatchesCount := 0

	for {
		activeQueueIndex := slices.IndexFunc(queues, func(queue *guidedCrawlQueue) bool {
			return len(*queue) > 0
		})
		if activeQueueIndex == -1 {
			break
		}

		activeQueue := queues[activeQueueIndex]
		if activeQueue == archivesQueue {
			archivesProcessedCount++
		} else if activeQueue == mainPageQueue {
			mainPagesProcessedCount++
		}

		var linkOrPage linkOrHtmlPage
		linkOrPage, *activeQueue = (*activeQueue)[0], (*activeQueue)[1:]
		var link *pristineLink
		var page *htmlPage
		switch lop := linkOrPage.(type) {
		case *pristineLink:
			link = lop
			if crawlCtx.FetchedCuris.Contains(link.Curi()) {
				continue
			}

			var err error
			page, err = crawlHtmlPage(link.Unwrap(), crawlCtx, logger)
			err2 := progressLogger.SaveStatus()
			if err2 != nil {
				return nil, err2
			}
			if errors.Is(err, context.Canceled) {
				return nil, ErrCrawlCanceled
			} else if err != nil {
				logger.Info("Couldn't fetch link: %v", err)
				continue
			}
		case *htmlPage:
			page = lop
			rawLink, ok := ToCanonicalLink(page.FetchUri.String(), logger, nil)
			if !ok {
				panic("Couldn't parse page fetch uri as a link")
			}
			link = NewPristineLink(rawLink)
		default:
			panic("Unknown link or page type")
		}

		puppeteerPage, err := crawlWithPuppeteerIfMatch(
			page, guidedCtx.FeedGenerator, guidedCtx.FeedEntryCurisTitlesMap, crawlCtx, logger,
		)
		if err != nil {
			logger.Info("Couldn't crawl with Puppeteer: %v", err)
			continue
		}
		if puppeteerPage != page {
			err := progressLogger.SaveStatus()
			if err != nil {
				return nil, err
			}
		}

		pageAllLinks := extractLinks(
			puppeteerPage.Document, puppeteerPage.FetchUri, nil, crawlCtx.Redirects, logger,
			includeXPathAndClassXPath,
		)
		for _, pageLink := range pageAllLinks {
			if !guidedCtx.AllowedHosts[pageLink.Uri.Host] {
				continue
			}

			if !isNewAndAllowed(&pageLink.Link, &guidedCtx.SeenCurisSet, crawlCtx.RobotsClient, logger) {
				continue
			}

			if archivesRegex.MatchString(pageLink.Curi.TrimmedPath) {
				guidedCtx.SeenCurisSet.add(pageLink.Curi)
				*archivesQueue = append(*archivesQueue, NewPristineLink(&pageLink.Link))
				hadArchives = true
				archivesSeenCount++
				logger.Info("Enqueueing archives link: %s", pageLink.Curi)
			} else if mainPageRegex.MatchString(pageLink.Curi.TrimmedPath) {
				guidedCtx.SeenCurisSet.add(pageLink.Curi)
				*mainPageQueue = append(*mainPageQueue, NewPristineLink(&pageLink.Link))
				mainPagesSeenCount++
				logger.Info("Enqueueing main page link: %s", pageLink.Curi)
			}
		}

		var pageCuris []CanonicalUri
		for _, pageLink := range pageAllLinks {
			pageCuris = append(pageCuris, pageLink.Curi)
		}
		pageCurisSet := NewCanonicalUriSet(pageCuris, guidedCtx.CuriEqCfg)
		pageResults := tryExtractHistorical(
			link, puppeteerPage, pageAllLinks, &pageCurisSet, guidedCtx, logger,
		)
	pgResults:
		for _, pageResult := range pageResults {
			historicalMatchesCount++
			for _, sortedResult := range sortedResults {
				if sortedResult.isSame(pageResult, guidedCtx.CuriEqCfg) {
					continue pgResults
				}
			}
			insertSortedResult(&sortedResults, pageResult)
		}

		if (hadArchives || crawlCtx.RequestsMade >= 60) &&
			len(*archivesQueue) == 0 && len(sortedResults) > 0 {

			ppResult, err := postprocessResults(&sortedResults, guidedCtx, crawlCtx, logger)
			if errors.Is(err, ErrCrawlCanceled) {
				return nil, err
			}
			if err == nil {
				if len(ppResult.Links) >= 21 {
					logger.Info(
						"Processed/seen: archives %d/%d, main pages %d/%d",
						archivesProcessedCount, archivesSeenCount, mainPagesProcessedCount,
						mainPagesSeenCount,
					)
					logger.Info("Historical matches: %d", historicalMatchesCount)
					logger.Info(
						"Guided crawl loop finished (phase %d) after archives with best result of %d links",
						phaseNumber, len(ppResult.Links),
					)
					return ppResult, nil
				} else {
					logger.Info(
						"Best result after archives only has %d links. Checking others just in case",
						len(ppResult.Links),
					)
					sortedResults = slices.Insert(
						sortedResults, 0, (crawlHistoricalResult)(ppResult),
					)
				}
			}
		}
	}

	ppResult, err := postprocessResults(&sortedResults, guidedCtx, crawlCtx, logger)
	logger.Info(
		"Processed/seen: archives %d/%d, main pages %d/%d",
		archivesProcessedCount, archivesSeenCount, mainPagesProcessedCount, mainPagesSeenCount,
	)
	logger.Info("Historical matches: %d", historicalMatchesCount)
	if errors.Is(err, ErrCrawlCanceled) {
		return nil, err
	} else if err != nil {
		logger.Info("Guided crawl loop finished (phase %d), no result (%v)", phaseNumber, err)
		return nil, err
	}

	logger.Info(
		"Guided crawl loop finished (phase %d) with best result of %d links",
		phaseNumber, len(ppResult.Links),
	)
	return ppResult, nil
}

func tryExtractHistorical(
	fetchLink *pristineLink, page *htmlPage, pageLinks []*xpathLink, pageCurisSet *CanonicalUriSet,
	guidedCtx *guidedCrawlContext, logger Logger,
) []crawlHistoricalResult {
	logger.Info("Trying to extract historical from %s", page.FetchUri)
	var results []crawlHistoricalResult

	archivesAlmostMatchThreshold := getArchivesAlmostMatchThreshold(guidedCtx.FeedEntryLinks.Length)
	extractionsByStarCount := getExtractionsByStarCount(
		pageLinks, guidedCtx.FeedGenerator, guidedCtx.FeedEntryLinks, &guidedCtx.FeedEntryCurisTitlesMap,
		guidedCtx.CuriEqCfg, archivesAlmostMatchThreshold, logger,
	)

	archivesResults := tryExtractArchives(
		fetchLink, page, pageLinks, pageCurisSet, extractionsByStarCount, archivesAlmostMatchThreshold,
		guidedCtx, logger,
	)
	results = append(results, archivesResults...)
	if len(archivesResults) == 0 {
		if page.MaybeTopScreenshot != nil {
			logger.Screenshot(page.FetchUri.String(), "top", page.MaybeTopScreenshot)
		}
		if page.MaybeBottomScreenshot != nil {
			logger.Screenshot(page.FetchUri.String(), "bottom", page.MaybeBottomScreenshot)
		}
	}

	if archivesCategoriesResult, ok := tryExtractArchivesCategories(
		page, pageCurisSet, extractionsByStarCount, guidedCtx, logger,
	); ok {
		results = append(results, archivesCategoriesResult)
	}

	if page1Result, ok := tryExtractPage1(
		fetchLink, page, pageLinks, pageCurisSet, extractionsByStarCount, guidedCtx, logger,
	); ok {
		results = append(results, page1Result)
	}

	return results
}

func insertSortedResult(sortedResults *[]crawlHistoricalResult, newResult crawlHistoricalResult) {
	insertIndex := slices.IndexFunc(*sortedResults, func(result crawlHistoricalResult) bool {
		return speculativeCountBetterThan(newResult, result)
	})
	if insertIndex >= 0 {
		*sortedResults = slices.Insert(*sortedResults, insertIndex, newResult)
	} else {
		*sortedResults = append(*sortedResults, newResult)
	}
}

func speculativeCountBetterThan(result1, result2 crawlHistoricalResult) bool {
	result1MatchingFeed := false
	result2MatchingFeed := false
	if pp1, ok := result1.(*postprocessedResult); !(ok && !pp1.IsMatchingFeed) {
		result1MatchingFeed = true
	}
	if pp2, ok := result2.(*postprocessedResult); !(ok && !pp2.IsMatchingFeed) {
		result2MatchingFeed = true
	}

	return (result1MatchingFeed && !result2MatchingFeed) ||
		(!(!result1MatchingFeed && result2MatchingFeed) &&
			result1.speculativeCount() > result2.speculativeCount())
}

func speculativeCountEqual(result1, result2 crawlHistoricalResult) bool {
	result1MatchingFeed := false
	result2MatchingFeed := false
	if pp1, ok := result1.(*postprocessedResult); !(ok && !pp1.IsMatchingFeed) {
		result1MatchingFeed = true
	}
	if pp2, ok := result2.(*postprocessedResult); !(ok && !pp2.IsMatchingFeed) {
		result2MatchingFeed = true
	}

	return result1MatchingFeed == result2MatchingFeed &&
		result1.speculativeCount() == result2.speculativeCount()
}

var errPostprocessingFailed = errors.New("postprocessing failed")

func postprocessResults(
	sortedResults *[]crawlHistoricalResult, guidedCtx *guidedCrawlContext, crawlCtx *CrawlContext,
	logger Logger,
) (*postprocessedResult, error) {
	var sortedResultsLog []string
	for _, result := range *sortedResults {
		sortedResultsLog = append(sortedResultsLog, printResult(result))
	}
	logger.Info("Postprocessing %d results: %v", len(*sortedResults), sortedResultsLog)

	for len(*sortedResults) > 0 {
		var result crawlHistoricalResult
		result, *sortedResults = (*sortedResults)[0], (*sortedResults)[1:]
		logger.Info("Postprocessing %v", printResult(result))

		var ppResult *postprocessedResult
		var ppErr error
		switch res := result.(type) {
		case *postprocessedResult:
			if res.MaybePartialPagedResult == nil {
				ppResult, ppErr = res, nil
			} else {
				ppResult, ppErr = postprocessPartialPagedResult(
					res.MaybePartialPagedResult, guidedCtx, crawlCtx, logger,
				)
			}
		case *archivesSortedResult:
			ppResult, ppErr = postprocessArchivesSortedResult(res, guidedCtx, crawlCtx, logger)
		case *archivesShuffledResults:
			ppResult, ppErr = postprocessArchivesShuffledResults(res, guidedCtx, crawlCtx, logger)
		case *archivesMediumPinnedEntryResult:
			ppResult, ppErr = postprocessArchivesMediumPinnedEntryResult(res, guidedCtx, crawlCtx, logger)
		case *archivesLongFeedResult:
			ppResult, ppErr = postprocessArchivesLongFeedResult(res), nil
		case *ArchivesCategoriesResult:
			ppResult, ppErr = postprocessArchviesCategoriesResult(res, guidedCtx, crawlCtx, logger)
		case *page1Result:
			// If page 1 result looks the best, check just the page 2 in case it was a scam
			ppResult, ppErr = postprocessPage1Result(res, guidedCtx, crawlCtx, logger)
		default:
			panic("Unknown result type")
		}

		if errors.Is(ppErr, ErrCrawlCanceled) || errors.Is(ppErr, ErrBlogTooLong) {
			return nil, ppErr
		} else if ppErr != nil {
			logger.Info("Postprocessing failed for %s, continuing", result.mainLink().Url)
			continue
		}

		if len(*sortedResults) == 0 ||
			speculativeCountBetterThan(ppResult, (*sortedResults)[0]) ||
			(ppResult.MaybePartialPagedResult == nil &&
				speculativeCountEqual(ppResult, (*sortedResults)[0])) {

			if ppResult.MaybePartialPagedResult != nil {
				var err error
				ppResult, err = postprocessPartialPagedResult(
					ppResult.MaybePartialPagedResult, guidedCtx, crawlCtx, logger,
				)
				if errors.Is(err, ErrCrawlCanceled) || errors.Is(err, ErrBlogTooLong) {
					return nil, err
				} else if err != nil {
					logger.Info("Postprocessing failed for %s, continuing", result.mainLink().Url)
					continue
				}
			}

			logger.Info("Postprocessing succeeded")
			return ppResult, nil
		}

		ppNotMatchingFeed := ""
		if !ppResult.IsMatchingFeed {
			ppNotMatchingFeed = ", not matching feed"
		}
		logger.Info("Inserting back postprocessed %v%s", printResult(ppResult), ppNotMatchingFeed)
		insertSortedResult(sortedResults, ppResult)
	}

	logger.Info("Postprocessing failed")
	return nil, errPostprocessingFailed
}

func printResult(result crawlHistoricalResult) string {
	name := reflect.TypeOf(result).Elem().Name()
	return fmt.Sprintf("[%s, %s, %d]", name, result.mainLink().Url, result.speculativeCount())
}

func postprocessArchivesSortedResult(
	archivesSortedResult *archivesSortedResult, guidedCtx *guidedCrawlContext, crawlCtx *CrawlContext,
	logger Logger,
) (*postprocessedResult, error) {
	progressLogger := crawlCtx.ProgressLogger
	if CanonicalUriEqual(archivesSortedResult.MainLnk.Curi(), hardcodedBenKuhnArchives, guidedCtx.CuriEqCfg) {
		logger.Info("Postprocess archives sorted result start")
		logger.Info("Extra request for Ben Kuhn categories")
		titleCount := countLinkTitles(archivesSortedResult.Links)
		err := progressLogger.LogAndSaveFetchedCount(&titleCount)
		if err != nil {
			return nil, err
		}
		page, err := crawlHtmlPage(hardcodedBenKuhn, crawlCtx, logger)
		err2 := progressLogger.LogAndSavePostprocessing()
		if err2 != nil {
			return nil, err2
		}
		if err != nil {
			logger.Info("Ben Kuhn categories fetch error: %v", err)
			guidedCtx.HardcodedError = oops.Wrapf(err, "Ben Kuhn categories fetch error")
		} else {
			postCategories, err := extractBenKuhnCategories(page, logger)
			if err != nil {
				logger.Warn("Ben Kuhn categories extract error: %v", err)
				guidedCtx.HardcodedError = err
			} else {
				logger.Info("Categories: %s", categoryCountsString(postCategories))
				logger.Info("Postprocess archives sorted result finish")
				return &postprocessedResult{
					MainLnk:                 archivesSortedResult.MainLnk,
					Pattern:                 archivesSortedResult.Pattern,
					Links:                   archivesSortedResult.Links,
					IsMatchingFeed:          true,
					PostCategories:          postCategories,
					Extra:                   archivesSortedResult.Extra,
					MaybePartialPagedResult: nil,
				}, nil
			}
		}
	} else if CanonicalUriEqual(
		archivesSortedResult.MainLnk.Curi(), hardcodedTransformerCircuits, guidedCtx.CuriEqCfg,
	) {
		logger.Info("Removing an extra Transformer Circuits link")
		filteredLinks := make([]*pristineMaybeTitledLink, 0, len(archivesSortedResult.Links))
		for _, link := range archivesSortedResult.Links {
			if !hardcodedTransformerCircuitsEntriesToExclude.Contains(link.Curi()) {
				filteredLinks = append(filteredLinks, link)
			}
		}
		return &postprocessedResult{
			MainLnk:                 archivesSortedResult.MainLnk,
			Pattern:                 archivesSortedResult.Pattern,
			Links:                   filteredLinks,
			IsMatchingFeed:          true,
			PostCategories:          archivesSortedResult.PostCategories,
			Extra:                   archivesSortedResult.Extra,
			MaybePartialPagedResult: nil,
		}, nil
	}

	return &postprocessedResult{
		MainLnk:                 archivesSortedResult.MainLnk,
		Pattern:                 archivesSortedResult.Pattern,
		Links:                   archivesSortedResult.Links,
		IsMatchingFeed:          true,
		PostCategories:          archivesSortedResult.PostCategories,
		Extra:                   archivesSortedResult.Extra,
		MaybePartialPagedResult: nil,
	}, nil
}

func postprocessArchivesShuffledResults(
	archivesShuffledResults *archivesShuffledResults, guidedCtx *guidedCrawlContext, crawlCtx *CrawlContext,
	logger Logger,
) (*postprocessedResult, error) {
	logger.Info("Postprocess archives shuffled results start")
	sortedTentativeResults := slices.Clone(archivesShuffledResults.Results)
	slices.SortFunc(sortedTentativeResults, func(a, b *archivesShuffledResult) int {
		return len(a.Links) - len(b.Links)
	})
	archivesShuffledCounts := make([]int, len(sortedTentativeResults))
	for i, tentativeResult := range sortedTentativeResults {
		archivesShuffledCounts[i] = tentativeResult.SpeculativeCount()
	}
	logger.Info("Archives shuffled counts: %v", archivesShuffledCounts)

	var bestResult *postprocessedResult
	bestResultErr := errPostprocessingFailed
	sortablePagesByCanonicalUrl := make(map[string]sortablePage)
	for _, tentativeResult := range sortedTentativeResults {
		logger.Info("Postprocessing archives shuffled result of %d", tentativeResult.SpeculativeCount())
		sortedLinks, err := postprocessSortLinksMaybeDates(
			tentativeResult.Links, tentativeResult.MaybeDates, sortablePagesByCanonicalUrl, guidedCtx,
			crawlCtx, logger,
		)
		if errors.Is(err, ErrCrawlCanceled) {
			return nil, err
		} else if err != nil {
			logger.Info("Postprocess archives shuffled results finish (iteration failed)")
			return bestResult, bestResultErr
		}

		extra := slices.Clone(tentativeResult.Extra)
		appendLogLinef(&extra, "sort_date_source: %s", sortedLinks.DateSource)
		appendLogLinef(&extra, "are_matching_feed: %t", sortedLinks.AreMatchingFeed)
		bestResult = &postprocessedResult{
			MainLnk:                 archivesShuffledResults.MainLnk,
			Pattern:                 tentativeResult.Pattern,
			Links:                   sortedLinks.Links,
			IsMatchingFeed:          sortedLinks.AreMatchingFeed,
			PostCategories:          tentativeResult.PostCategories,
			Extra:                   extra,
			MaybePartialPagedResult: nil,
		}
		bestResultErr = nil
	}

	logger.Info("Postprocess archives shuffled results finish")
	return bestResult, bestResultErr
}

func postprocessArchivesMediumPinnedEntryResult(
	mediumResult *archivesMediumPinnedEntryResult, guidedCtx *guidedCrawlContext, crawlCtx *CrawlContext,
	logger Logger,
) (*postprocessedResult, error) {
	progressLogger := crawlCtx.ProgressLogger
	logger.Info("Postprocess archives medium pinned entry result start")
	pinnedEntryPage, err := crawlHtmlPage(mediumResult.PinnedEntryLink.Link.Unwrap(), crawlCtx, logger)
	err2 := progressLogger.LogAndSavePostprocessing()
	if err2 != nil {
		return nil, err2
	}
	if err != nil {
		logger.Info(
			"Couldn't fetch first Medium link during result postprocess: %s (%v)",
			mediumResult.PinnedEntryLink.Curi, err,
		)
		return nil, errPostprocessingFailed
	}

	allowedHosts := map[string]bool{pinnedEntryPage.FetchUri.Host: true}
	pinnedEntryPageLinks := extractLinks(
		pinnedEntryPage.Document, pinnedEntryPage.FetchUri, allowedHosts, crawlCtx.Redirects, logger,
		includeXPathNone,
	)

	sortedLinks, ok := historicalArchivesMediumSortFinish(
		mediumResult.PinnedEntryLink, pinnedEntryPageLinks, mediumResult.OtherLinksDates,
		guidedCtx.CuriEqCfg, logger,
	)
	if !ok {
		logger.Info("Couldn't sort links during postprocess archives medium pinned entry result finish")
		return nil, errPostprocessingFailed
	}

	if !compareWithFeed(sortedLinks, guidedCtx.FeedEntryLinks, guidedCtx.CuriEqCfg, logger) {
		logger.Info("Postprocess archives medium pinned entry result not matching feed")
		return nil, errPostprocessingFailed
	}

	logger.Info("Postprocess archives medium pinned entry result finish")
	return &postprocessedResult{
		MainLnk:                 mediumResult.MainLnk,
		Pattern:                 mediumResult.Pattern,
		Links:                   sortedLinks,
		IsMatchingFeed:          true,
		PostCategories:          nil,
		Extra:                   mediumResult.Extra,
		MaybePartialPagedResult: nil,
	}, nil
}

func postprocessArchivesLongFeedResult(archivesLongFeedResult *archivesLongFeedResult) *postprocessedResult {
	return &postprocessedResult{
		MainLnk:                 archivesLongFeedResult.MainLnk,
		Pattern:                 archivesLongFeedResult.Pattern,
		Links:                   archivesLongFeedResult.Links,
		IsMatchingFeed:          true,
		PostCategories:          nil,
		Extra:                   archivesLongFeedResult.Extra,
		MaybePartialPagedResult: nil,
	}
}

func postprocessArchviesCategoriesResult(
	archivesCategoriesResult *ArchivesCategoriesResult, guidedCtx *guidedCrawlContext, crawlCtx *CrawlContext,
	logger Logger,
) (*postprocessedResult, error) {
	logger.Info("Postprocess archives categories results start")

	sortedLinks, err := postprocessSortLinksMaybeDates(
		archivesCategoriesResult.Links, archivesCategoriesResult.MaybeDates, make(map[string]sortablePage),
		guidedCtx, crawlCtx, logger,
	)
	if errors.Is(err, ErrCrawlCanceled) {
		return nil, err
	} else if err != nil {
		logger.Info("Postporcess archives categories results failed")
		return nil, errPostprocessingFailed
	}

	logger.Info("Postprocess archives categories results finish")
	extra := slices.Clone(archivesCategoriesResult.Extra)
	appendLogLinef(&extra, "sort_date_source: %v", sortedLinks.DateSource)
	appendLogLinef(&extra, "are_matching_feed: %t", sortedLinks.AreMatchingFeed)
	return &postprocessedResult{
		MainLnk:                 archivesCategoriesResult.MainLnk,
		Pattern:                 archivesCategoriesResult.Pattern,
		Links:                   sortedLinks.Links,
		PostCategories:          nil,
		IsMatchingFeed:          sortedLinks.AreMatchingFeed,
		Extra:                   extra,
		MaybePartialPagedResult: nil,
	}, nil
}

func postprocessPage1Result(
	page1Result *page1Result, guidedCtx *guidedCrawlContext, crawlCtx *CrawlContext, logger Logger,
) (*postprocessedResult, error) {
	progressLogger := crawlCtx.ProgressLogger
	logger.Info("Postprocess page1 result start")
	page2, err := crawlHtmlPage(&page1Result.LinkToPage2, crawlCtx, logger)
	err2 := progressLogger.LogAndSavePostprocessing()
	if err2 != nil {
		return nil, err2
	}
	if err != nil {
		logger.Info("Page 2 is not a page: %s (%v)", page1Result.LinkToPage2, err)
		return nil, errPostprocessingFailed
	}

	pagedResult, ok := tryExtractPage2(page2, &page1Result.PagedState, guidedCtx, logger)
	if ok {
		titleCount := countLinkTitles(pagedResult.links())
		err := progressLogger.LogAndSaveFetchedCount(&titleCount)
		if err != nil {
			return nil, err
		}
		logger.Info("Postprocess page1 result finish")

		switch r := pagedResult.(type) {
		case *partialPagedResult:
			return &postprocessedResult{
				MainLnk:                 r.MainLnk,
				Pattern:                 "paged_partial",
				Links:                   r.Lnks,
				IsMatchingFeed:          true,
				PostCategories:          nil,
				Extra:                   nil,
				MaybePartialPagedResult: r,
			}, nil
		case *fullPagedResult:
			return &postprocessedResult{
				MainLnk:                 r.MainLnk,
				Pattern:                 r.Pattern,
				Links:                   r.Lnks,
				IsMatchingFeed:          true,
				PostCategories:          r.PostCategories,
				Extra:                   r.Extra,
				MaybePartialPagedResult: nil,
			}, nil
		default:
			panic("Unknown paged result type")
		}
	}

	logger.Info("Postprocess page1 result finish (failed)")
	return nil, errPostprocessingFailed
}

var ErrBlogTooLong = errors.New("blog too long")

func postprocessPartialPagedResult(
	partialResult *partialPagedResult, guidedCtx *guidedCrawlContext, crawlCtx *CrawlContext, logger Logger,
) (*postprocessedResult, error) {
	progressLogger := crawlCtx.ProgressLogger
	logger.Info("Postprocess paged result start")
	var fullResult *fullPagedResult
	for {
		titleCount := countLinkTitles(partialResult.links())
		err := progressLogger.LogAndSaveFetchedCount(&titleCount)
		if err != nil {
			return nil, err
		}
		page, err := crawlHtmlPage(partialResult.LinkToNextPage.Unwrap(), crawlCtx, logger)
		err2 := progressLogger.LogAndSavePostprocessing()
		if err2 != nil {
			return nil, err2
		}
		if err != nil {
			logger.Info(
				"Postprocess paged result failed, page %d is not a page: %s (%v)",
				partialResult.PagedState.PageNumber, partialResult.LinkToNextPage, err,
			)
			return nil, errPostprocessingFailed
		}

		pagedResult, ok := tryExtractNextPage(page, partialResult, guidedCtx, logger)
		if !ok {
			logger.Info("Postprocess paged result failed")
			return nil, errPostprocessingFailed
		}
		switch r := pagedResult.(type) {
		case *fullPagedResult:
			fullResult = r
		case *partialPagedResult:
			partialResult = r
		default:
			panic("Unknown paged result type")
		}
		if fullResult != nil {
			break
		}

		if partialResult.SpeculativeCount() > 10000 {
			return nil, ErrBlogTooLong
		}
	}

	var postCategories []pristineHistoricalBlogPostCategory
	var links []*pristineMaybeTitledLink
	if CanonicalUriEqual(fullResult.MainLnk.Curi(), hardcodedCaseyHandmer, guidedCtx.CuriEqCfg) {
		var err error
		postCategories, err = crawlCaseyHandmerCategories(fullResult, guidedCtx, crawlCtx, logger)
		if err != nil {
			logger.Warn("Couldn't fetch Casey Handmer categories: %v", err)
			guidedCtx.HardcodedError = err
		} else {
			logger.Info("Categories: %s", categoryCountsString(postCategories))
		}
	} else if CanonicalUriEqual(guidedCtx.FeedRootLinkCuri, hardcodedTheOldNewThing, guidedCtx.CuriEqCfg) {
		var err error
		postCategories, links, err =
			crawlTheOldNewThingCategories(fullResult, guidedCtx, crawlCtx, logger)
		if err != nil {
			logger.Warn("Couldn't fetch The Old New Thing categories: %v", err)
			guidedCtx.HardcodedError = err
		} else {
			logger.Info("Categories: %s", categoryCountsString(postCategories))
			logger.Info("Links: %d -> %d", len(fullResult.Lnks), len(links))
		}
	}
	if len(links) != 0 {
		fullResult.Lnks = links
	}
	if len(postCategories) != 0 {
		fullResult.PostCategories = postCategories
	}

	titleCount := countLinkTitles(fullResult.Lnks)
	err := progressLogger.LogAndSaveFetchedCount(&titleCount)
	if err != nil {
		return nil, err
	}
	logger.Info("Postprocess paged result finish")
	return &postprocessedResult{
		MainLnk:                 fullResult.MainLnk,
		Pattern:                 fullResult.Pattern,
		Links:                   fullResult.Lnks,
		IsMatchingFeed:          true,
		PostCategories:          fullResult.PostCategories,
		Extra:                   fullResult.Extra,
		MaybePartialPagedResult: nil,
	}, nil
}

func crawlCaseyHandmerCategories(
	pagedResult *fullPagedResult, guidedCtx *guidedCrawlContext, crawlCtx *CrawlContext, logger Logger,
) ([]pristineHistoricalBlogPostCategory, error) {
	progressLogger := crawlCtx.ProgressLogger
	logger.Info("Extra requests for Casey Handmer categories")
	titleCount := countLinkTitles(pagedResult.Lnks)
	err := progressLogger.LogAndSaveFetchedCount(&titleCount)
	if err != nil {
		return nil, err
	}

	spaceMisconceptions, err := crawlHtmlPage(hardcodedCaseyHandmerSpaceMisconceptions, crawlCtx, logger)
	err2 := progressLogger.LogAndSavePostprocessing()
	if err2 != nil {
		return nil, err2
	}
	if err != nil {
		return nil, err
	}

	marsTrilogy, err := crawlHtmlPage(hardcodedCaseyHandmerMarsTrilogy, crawlCtx, logger)
	err2 = progressLogger.LogAndSavePostprocessing()
	if err2 != nil {
		return nil, err2
	}
	if err != nil {
		return nil, err
	}

	futureOfEnergy, err := crawlHtmlPage(hardcodedCaseyHandmerFutureOfEnergy, crawlCtx, logger)
	err2 = progressLogger.LogAndSavePostprocessing()
	if err2 != nil {
		return nil, err2
	}
	if err != nil {
		return nil, err
	}

	postCategories, err := extractCaseyHandmerCategories(
		spaceMisconceptions, marsTrilogy, futureOfEnergy, pagedResult.Lnks, guidedCtx.CuriEqCfg, logger,
	)
	if err != nil {
		return nil, err
	}

	return postCategories, nil
}

func crawlTheOldNewThingCategories(
	pagedResult *fullPagedResult, guidedCtx *guidedCrawlContext, crawlCtx *CrawlContext, logger Logger,
) ([]pristineHistoricalBlogPostCategory, []*pristineMaybeTitledLink, error) {
	progressLogger := crawlCtx.ProgressLogger
	logger.Info("Extra requests for The Old New Thing categories")
	titleCount := countLinkTitles(pagedResult.Lnks)
	err := progressLogger.LogAndSaveFetchedCount(&titleCount)
	if err != nil {
		return nil, nil, err
	}

	theOldNewThingWin32ApiPage, err := crawlNonHtmlPage(hardcodedTheOldNewThingWin32Api, crawlCtx, logger)
	err2 := progressLogger.LogAndSavePostprocessing()
	if err2 != nil {
		return nil, nil, err2
	}
	if err != nil {
		return nil, nil, err
	}

	postCategories, mergedLinks, err := ExtractTheOldNewThingCategories(
		theOldNewThingWin32ApiPage, pagedResult.Lnks, guidedCtx.CuriEqCfg, logger,
	)
	if err != nil {
		return nil, nil, err
	}

	return postCategories, mergedLinks, nil
}

type sortedLinks struct {
	Links           []*pristineMaybeTitledLink
	AreMatchingFeed bool
	DateXPath       string
	DateSource      dateSourceKind
}

var errSortFailed = errors.New("sort failed")

func postprocessSortLinksMaybeDates(
	links []*pristineMaybeTitledLink, maybeDates []*date, sortablePagesByCanonicalUrl map[string]sortablePage,
	guidedCtx *guidedCrawlContext, crawlCtx *CrawlContext, logger Logger,
) (*sortedLinks, error) {
	feedGenerator := guidedCtx.FeedGenerator
	progressLogger := crawlCtx.ProgressLogger
	logger.Info("Postprocess sort links, maybe dates start")

	var linksWithDates []linkDate[pristineMaybeTitledLink]
	var linksWithoutDates []*pristineMaybeTitledLink
	alreadyFetchedTitlesCount := 0
	for i, link := range links {
		date := maybeDates[i]
		if date != nil {
			linksWithDates = append(linksWithDates, linkDate[pristineMaybeTitledLink]{
				Link: *link,
				Date: *date,
			})
			if link.MaybeTitle != nil {
				alreadyFetchedTitlesCount++
			}
		} else {
			linksWithoutDates = append(linksWithoutDates, link)
		}
	}
	remainingTitlesCount := len(linksWithDates) - alreadyFetchedTitlesCount

	var crawledLinks []*pristineMaybeTitledLink
	var linksToCrawl []*pristineMaybeTitledLink
	for _, link := range linksWithoutDates {
		if _, ok := sortablePagesByCanonicalUrl[link.Curi().String()]; ok {
			crawledLinks = append(crawledLinks, link)
		} else {
			linksToCrawl = append(linksToCrawl, link)
		}
	}

	var sortState *sortState
	for _, link := range crawledLinks {
		page := sortablePagesByCanonicalUrl[link.Curi().String()]
		var ok bool
		sortState, ok = historicalArchivesSortAdd(page, sortState, logger)
		if !ok {
			logger.Info("Postprocess sort links, maybe dates failed during add already crawled")
			return nil, errSortFailed
		}
	}

	for linkIdx, link := range linksToCrawl {
		page, err := crawlHtmlPage(link.Link.Unwrap(), crawlCtx, logger)
		if err != nil {
			err2 := progressLogger.LogAndSavePostprocessingResetCount()
			if err2 != nil {
				return nil, err2
			}
			logger.Info("Couldn't fetch link during result postprocess: %s (%v)", link.Unwrap().Url, err)
			return nil, errSortFailed
		}
		sortablePage := historicalArchivesToSortablePage(page, feedGenerator, logger)

		var ok bool
		sortState, ok = historicalArchivesSortAdd(sortablePage, sortState, logger)
		if !ok {
			err := progressLogger.LogAndSavePostprocessingResetCount()
			if err != nil {
				return nil, err
			}
			logger.Info("Postprocess sort links, maybe dates failed during add")
			return nil, errSortFailed
		}

		fetchedCount := alreadyFetchedTitlesCount + len(crawledLinks) + linkIdx + 1
		remainingCount := remainingTitlesCount + len(linksToCrawl) - linkIdx - 1
		err = progressLogger.LogAndSavePostprocessingCounts(fetchedCount, remainingCount)
		if err != nil {
			return nil, err
		}
		sortablePagesByCanonicalUrl[page.Curi.String()] = sortablePage
	}

	resultLinks, dateSource, ok := historicalArchivesSortFinish(
		linksWithDates, linksWithoutDates, sortState, logger,
	)
	if !ok {
		logger.Info("Postprocess sort links, maybe dates failed during finish")
		return nil, errSortFailed
	}

	areMatchingFeed := compareWithFeed(resultLinks, guidedCtx.FeedEntryLinks, guidedCtx.CuriEqCfg, logger)

	logger.Info("Postprocess sort links, maybe dates finish")
	return &sortedLinks{
		Links:           resultLinks,
		AreMatchingFeed: areMatchingFeed,
		DateXPath:       dateSource.XPath,
		DateSource:      dateSource.DateSource,
	}, nil
}

func compareWithFeed(
	sortedLinks []*pristineMaybeTitledLink, feedEntryLinks *FeedEntryLinks,
	curiEqCfg *CanonicalEqualityConfig, logger Logger,
) bool {
	if !feedEntryLinks.IsOrderCertain {
		return true
	}

	sortedCuris := ToCanonicalUris(sortedLinks)
	curisSet := NewCanonicalUriSet(sortedCuris, curiEqCfg)
	presentFeedEntryLinks := feedEntryLinks.filterIncluded(&curisSet)
	if _, ok := presentFeedEntryLinks.sequenceMatch(sortedCuris, curiEqCfg); ok {
		return true
	}

	logger.Info("Sorted links")
	sortedLinksTrimmedByFeed := sortedCuris[:presentFeedEntryLinks.Length]
	sortedLinksEllipsis := ""
	if len(sortedCuris) > presentFeedEntryLinks.Length {
		sortedLinksEllipsis = " ..."
	}
	logger.Info("%v%s", sortedLinksTrimmedByFeed, sortedLinksEllipsis)
	logger.Info("are not matching filtered feed:")
	presentCuris := ToCanonicalUris(presentFeedEntryLinks.ToSlice())
	logger.Info("%v", presentCuris)
	return false
}

func fetchMissingTitles(
	links []*maybeTitledLink, feedEntryLinks *FeedEntryLinks,
	feedEntryCurisTitlesMap *CanonicalUriMap[MaybeLinkTitle], feedGenerator FeedGenerator,
	curiEqCfg *CanonicalEqualityConfig, crawlCtx *CrawlContext, logger Logger,
) ([]*titledLink, error) {
	progressLogger := crawlCtx.ProgressLogger
	startTime := time.Now()
	presentTitlesCount := 0
	for _, link := range links {
		if link.MaybeTitle != nil {
			presentTitlesCount++
		}
	}
	missingTitlesCount := len(links) - presentTitlesCount

	if missingTitlesCount == 0 {
		logger.Info("All titles are present")
		crawlCtx.TitleRequestsMade = 0
		titledLinks := make([]*titledLink, len(links))
		for i, link := range links {
			titledLinks[i] = &titledLink{
				Link:  link.Link,
				Title: *link.MaybeTitle,
			}
		}
		return titledLinks, nil
	}

	logger.Info("Fetch missing titles start: %d", missingTitlesCount)
	var linksWithFeedTitles []*maybeTitledLink
	var feedPresentTitlesCount, feedMissingTitlesCount int
	if len(links) <= feedEntryLinks.Length {
		linksWithFeedTitles = slices.Clone(links)
		for _, link := range linksWithFeedTitles {
			if link.MaybeTitle == nil {
				if feedTitle, ok := feedEntryCurisTitlesMap.Get(link.Curi); ok {
					link.MaybeTitle = feedTitle
				}
			}
		}

		feedPresentTitlesCount = 0
		for _, link := range linksWithFeedTitles {
			if link.MaybeTitle != nil {
				feedPresentTitlesCount++
			}
		}
		feedMissingTitlesCount = len(linksWithFeedTitles) - feedPresentTitlesCount
		if missingTitlesCount != feedMissingTitlesCount {
			logger.Info(
				"Filled %d/%d missing titles from feeds",
				missingTitlesCount-feedMissingTitlesCount, missingTitlesCount,
			)
		}

		if feedMissingTitlesCount == 0 {
			logger.Info("Fetch missing titles finish")
			crawlCtx.TitleRequestsMade = 0
			titledLinks := make([]*titledLink, len(linksWithFeedTitles))
			for i, link := range linksWithFeedTitles {
				titledLinks[i] = &titledLink{
					Link:  link.Link,
					Title: *link.MaybeTitle,
				}
			}
			return titledLinks, nil
		}
	} else {
		linksWithFeedTitles = links
		feedPresentTitlesCount = presentTitlesCount
		feedMissingTitlesCount = missingTitlesCount
	}

	err := progressLogger.LogAndSaveFetchedCount(&feedPresentTitlesCount)
	if err != nil {
		return nil, err
	}
	requestsMadeStart := crawlCtx.RequestsMade

	titledLinks := make([]*titledLink, len(linksWithFeedTitles))
	var pageTitleLinks []*Link
	var pageTitles []string
	fetchedTitlesCount := 0
	for linkIdx, link := range linksWithFeedTitles {
		var title LinkTitle
		if link.MaybeTitle != nil {
			title = *link.MaybeTitle
		} else {
			// Always making a request may produce some duplicate requests, but hopefully not too many
			page, err := crawlHtmlPage(&link.Link, crawlCtx, logger)
			if errors.Is(err, ErrCrawlCanceled) {
				return nil, err
			} else if err != nil {
				logger.Info("Couldn't fetch link title, going with url: %s (%v)", link.Url, err)
				title = NewLinkTitle(link.Url, LinkTitleSourceUrl, nil)
			} else {
				pageTitle := strings.Clone(getPageTitle(page, feedGenerator, logger))
				pageTitles = append(pageTitles, pageTitle)
				pageTitleLinks = append(pageTitleLinks, &link.Link)
				title = NewLinkTitle(pageTitle, LinkTitleSourcePageTitle, nil)
			}

			fetchedTitlesCount++
			err = progressLogger.LogAndSavePostprocessingCounts(
				feedPresentTitlesCount+fetchedTitlesCount, feedMissingTitlesCount-fetchedTitlesCount,
			)
			if err != nil {
				return nil, err
			}
		}

		titledLinks[linkIdx] = &titledLink{
			Link:  link.Link,
			Title: title,
		}
	}

	crawlCtx.TitleRequestsMade = crawlCtx.RequestsMade - requestsMadeStart
	finishTime := time.Now()
	crawlCtx.TitleFetchDuration = finishTime.Sub(startTime).Seconds()

	if len(pageTitles) == 0 {
		logger.Info("Page titles are empty, skipped the prefix/suffix discovery")
		logger.Info("Fetch missing titles finish")
		return titledLinks, nil
	}

	// Find out if page titles have a common prefix/suffix that needs to be removed
	// Lengths and indices are in bytes which makes for a valid utf8 comparison,
	// then we ensure no rune got cut in half
	firstPageTitle := pageTitles[0]
	prefixLength := len(firstPageTitle)
	suffixLength := len(firstPageTitle)
	for _, pageTitle := range pageTitles[1:] {
		if len(pageTitle) < prefixLength {
			prefixLength = len(pageTitle)
		}
		for i := 0; i < prefixLength; i++ {
			if pageTitle[i] != firstPageTitle[i] {
				prefixLength = i
				break
			}
		}

		if len(pageTitle) < suffixLength {
			suffixLength = len(pageTitle)
		}
		for i := 0; i < suffixLength; i++ {
			if pageTitle[len(pageTitle)-i-1] != firstPageTitle[len(firstPageTitle)-i-1] {
				suffixLength = i
				break
			}
		}
	}

	for !utf8.ValidString(firstPageTitle[:prefixLength]) {
		prefixLength--
	}
	for !utf8.ValidString(firstPageTitle[len(firstPageTitle)-suffixLength:]) {
		suffixLength--
	}

	prefix := firstPageTitle[:prefixLength]
	suffix := firstPageTitle[len(firstPageTitle)-suffixLength:]
	for _, ellipsis := range []string{"...", "…"} {
		if strings.HasPrefix(suffix, ellipsis) {
			suffix = suffix[len(ellipsis):]
			suffixLength -= len(ellipsis)
		}
	}

	minTitle := firstPageTitle
	minTitleLength := len(firstPageTitle)
	for _, pageTitle := range pageTitles[1:] {
		if len(pageTitle) < minTitleLength {
			minTitleLength = len(pageTitle)
			minTitle = pageTitle
		}
	}
	if prefixLength+suffixLength >= minTitleLength {
		logger.Info("Title prefix and suffix overlap: %q + %q > %q. Resetting them", prefix, suffix, minTitle)
		prefix = ""
		prefixLength = 0
		suffix = ""
		suffixLength = 0
	}

	if prefixLength > 0 {
		logger.Info("Found common prefix for page titles: %s", prefix)
	}
	if suffixLength > 0 {
		logger.Info("Found common suffix for page titles: %s", suffix)
	}
	if prefixLength > 0 || suffixLength > 0 {
		curis := ToCanonicalUris(pageTitleLinks)
		curisSet := NewCanonicalUriSet(curis, curiEqCfg)
		var testLink Link
		for _, link := range links {
			if !curisSet.Contains(link.Curi) {
				testLink = link.Link
				break
			}
		}

		arePrefixSuffixValid := false
		if len(pageTitles) == len(links) {
			logger.Info(
				"All links needed title fetching, can't validate prefix/suffix but proceeding with it",
			)
			arePrefixSuffixValid = true
		} else if len(pageTitles) >= 15 {
			logger.Info(
				"%d titles is enough to use prefix/suffix without an extra fetch", len(pageTitles),
			)
			arePrefixSuffixValid = true
		} else {
			fetchedCount := feedPresentTitlesCount + fetchedTitlesCount + 1
			err := progressLogger.LogAndSavePostprocessingCounts(fetchedCount, 1)
			if err != nil {
				return nil, err
			}
			page, err := crawlHtmlPage(&testLink, crawlCtx, logger)
			err2 := progressLogger.LogAndSavePostprocessingCounts(fetchedCount, 0)
			if err2 != nil {
				return nil, err2
			}
			if err != nil {
				logger.Info("Couldn't fetch first link title: %s (%v)", testLink.Uri, err)
			} else {
				testPageTitle := getPageTitle(page, feedGenerator, logger)
				if !strings.HasPrefix(testPageTitle, prefix) {
					logger.Info("Test link prefix doesn't check out: %q, %q", testPageTitle, prefix)
				} else if !strings.HasSuffix(testPageTitle, suffix) {
					logger.Info("Test link suffix doesn't check out: %q, %q", testPageTitle, suffix)
				} else {
					logger.Info("Title prefix and suffix check out with the test link")
					arePrefixSuffixValid = true
				}
			}
		}

		if arePrefixSuffixValid {
			for _, titledLink := range titledLinks {
				if titledLink.Title.Source != LinkTitleSourcePageTitle {
					continue
				}

				oldTitle := titledLink.Title.Value
				titledLink.Title = NewLinkTitle(
					oldTitle[prefixLength:len(oldTitle)-suffixLength],
					titledLink.Title.Source, nil,
				)
			}
		}
	}

	logger.Info("Fetch missing titles finish")
	return titledLinks, nil
}

func countLinkTitles(links []*pristineMaybeTitledLink) int {
	titleCount := 0
	for _, link := range links {
		if link.MaybeTitle != nil {
			titleCount++
		}
	}
	return titleCount
}

func countLinkTitleSources(links []*titledLink) string {
	countsBySource := make(map[linkTitleSource]int)
	for _, link := range links {
		countsBySource[link.Title.Source] += 1
	}

	type SourceCount = struct {
		Source linkTitleSource
		Count  int
	}
	sourceCounts := make([]SourceCount, 0, len(countsBySource))
	for source, count := range countsBySource {
		sourceCounts = append(sourceCounts, SourceCount{
			Source: source,
			Count:  count,
		})
	}
	slices.SortFunc(sourceCounts, func(a, b SourceCount) int {
		return b.Count - a.Count // descending
	})

	tokens := make([]string, 0, len(sourceCounts))
	for _, sourceCount := range sourceCounts {
		quote := `"`
		if strings.HasPrefix(string(sourceCount.Source), "[") {
			quote = ""
		}
		tokens = append(
			tokens, fmt.Sprintf("%s%s%s: %d", quote, sourceCount.Source, quote, sourceCount.Count),
		)
	}

	return fmt.Sprintf("{%s}", strings.Join(tokens, ", "))
}

func ExtractSubstackPublicAndTotalCounts(
	rootUrl string, crawlCtx *CrawlContext, logger Logger,
) (publicCount, totalCount int, err error) {
	rootUrl = strings.TrimRight(rootUrl, "/")
	curiEqCfg := &CanonicalEqualityConfig{SameHosts: nil, ExpectTumblrPaths: false}

	feedUrl := rootUrl + "/feed"
	feedLink, ok := ToCanonicalLink(feedUrl, logger, nil)
	if !ok {
		return 0, 0, oops.Newf("Couldn't parse feed url: %s", feedUrl)
	}
	feedOrHtmlPage, err := crawlFeedOrHtmlPage(feedLink, crawlCtx, logger)
	if err != nil {
		return 0, 0, err
	}
	feedPage, ok := feedOrHtmlPage.(*feedPage)
	if !ok {
		return 0, 0, oops.Newf("Link is not a feed: %v", feedLink.Url)
	}
	parsedFeed, err := ParseFeed(feedPage.Content, feedLink.Uri, logger)
	if err != nil {
		return 0, 0, err
	}
	feedEntryCurisTitlesMap := NewCanonicalUriMap[MaybeLinkTitle](curiEqCfg)
	for _, entryLink := range parsedFeed.EntryLinks.ToSlice() {
		feedEntryCurisTitlesMap.Add(entryLink.Link, entryLink.MaybeTitle)
	}
	guidedCtx := guidedCrawlContext{
		SeenCurisSet:            newGuidedSeenCurisSet(curiEqCfg),
		ArchivesCategoriesState: nil,
		FeedEntryLinks:          &parsedFeed.EntryLinks,
		FeedEntryCurisTitlesMap: feedEntryCurisTitlesMap,
		FeedGenerator:           parsedFeed.Generator,
		FeedRootLinkCuri:        parsedFeed.RootLink.Curi,
		CuriEqCfg:               curiEqCfg,
		AllowedHosts:            nil,
		HardcodedError:          nil,
	}

	archivesUrl := rootUrl + "/archive"
	archivesLink, ok := ToCanonicalLink(archivesUrl, logger, nil)
	pristineArchivesLink := NewPristineLink(archivesLink)
	if !ok {
		return 0, 0, oops.Newf("Couldn't parse archives url: %s", feedUrl)
	}
	archivesHtmlPage, err := crawlHtmlPage(archivesLink, crawlCtx, logger)
	if err != nil {
		return 0, 0, err
	}
	pageAllLinks := extractLinks(
		archivesHtmlPage.Document, archivesHtmlPage.FetchUri, nil, crawlCtx.Redirects, logger,
		includeXPathAndClassXPath,
	)
	var pageCuris []CanonicalUri
	for _, pageLink := range pageAllLinks {
		pageCuris = append(pageCuris, pageLink.Curi)
	}
	pageCurisSet := NewCanonicalUriSet(pageCuris, curiEqCfg)

	filteredFeedEntryLinks := FeedEntryLinks{
		LinkBuckets:    [][]FeedEntryLink{},
		Length:         0,
		IsOrderCertain: parsedFeed.EntryLinks.IsOrderCertain,
	}
	for _, bucket := range guidedCtx.FeedEntryLinks.LinkBuckets {
		var filteredBucket []FeedEntryLink
		for _, link := range bucket {
			if pageCurisSet.Contains(link.Curi) {
				filteredBucket = append(filteredBucket, link)
			}
		}
		if len(filteredBucket) > 0 {
			filteredFeedEntryLinks.LinkBuckets = append(filteredFeedEntryLinks.LinkBuckets, filteredBucket)
			filteredFeedEntryLinks.Length += len(filteredBucket)
		}
	}
	guidedCtx.FeedEntryLinks = &filteredFeedEntryLinks

	archivesAlmostMatchThreshold := getArchivesAlmostMatchThreshold(filteredFeedEntryLinks.Length)
	extractionsByStarCount := getExtractionsByStarCount(
		pageAllLinks, guidedCtx.FeedGenerator, &filteredFeedEntryLinks, &feedEntryCurisTitlesMap, curiEqCfg,
		archivesAlmostMatchThreshold, logger,
	)
	historicalResults := tryExtractArchives(
		pristineArchivesLink, archivesHtmlPage, pageAllLinks, &pageCurisSet, extractionsByStarCount,
		archivesAlmostMatchThreshold, &guidedCtx, logger,
	)
	if len(historicalResults) != 1 {
		return 0, 0, oops.Newf("Expected 1 historical result, got %d", len(historicalResults))
	}
	historicalResult := historicalResults[0]

	archivesSortedResult, ok := historicalResult.(*archivesSortedResult)
	if !ok {
		return 0, 0, oops.Newf("Expected archivesSortedResult, got %T", historicalResult)
	}
	if len(archivesSortedResult.PostCategories) != 1 {
		return 0, 0, oops.Newf("Expected 1 category, got %d", len(archivesSortedResult.PostCategories))
	}
	category := archivesSortedResult.PostCategories[0]
	if category.Name != "Public" {
		return 0, 0, oops.Newf("Expected the category to be named Public, got %s", category.Name)
	}

	return len(category.PostLinks), len(archivesSortedResult.Links), nil
}
