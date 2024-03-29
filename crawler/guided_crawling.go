package crawler

import (
	"errors"
	"feedrewind/oops"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/exp/slices"
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

func categoryCountsString(categories []HistoricalBlogPostCategory) string {
	var sb strings.Builder
	for i, category := range categories {
		if i > 0 {
			sb.WriteString(", ")
		}
		if category.IsTop {
			sb.WriteRune('!')
		}
		sb.WriteString(category.Name)
		fmt.Fprintf(&sb, " (%d)", len(category.PostLinks))
	}
	return sb.String()
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
			},
			Content:  maybeStartPage.Content,
			Document: startPageDocument,
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
		sortedFeedEntryHosts := make([]string, 0, len(feedEntryLinksByHost))
		for host := range feedEntryLinksByHost {
			sortedFeedEntryHosts = append(sortedFeedEntryHosts, host)
		}
		sort.Slice(sortedFeedEntryHosts, func(i, j int) bool {
			return len(feedEntryLinksByHost[sortedFeedEntryHosts[i]]) >
				len(feedEntryLinksByHost[sortedFeedEntryHosts[j]])
		})
		entryLinkFromPopularHost := feedEntryLinksByHost[sortedFeedEntryHosts[0]][0]
		entryPage, err := crawlPage(&entryLinkFromPopularHost, false, crawlCtx, logger)
		crawlCtx.ProgressLogger.SaveStatus()
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

	feedEntryCurisTitlesMap := NewCanonicalUriMap[*LinkTitle](&curiEqCfg)
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
	if parsedFeed.EntryLinks.Length <= 50 ||
		parsedFeed.EntryLinks.Length%100 == 0 ||
		CanonicalUriEqual(feedLink.Curi, HardcodedDanLuuFeed, &curiEqCfg) {

		var postprocessedResult *postprocessedResult
		if parsedFeed.Generator != FeedGeneratorTumblr {
			postprocessedResult, historicalError = guidedCrawlHistorical(
				startPage, &parsedFeed.EntryLinks, feedEntryCurisTitlesMap, parsedFeed.Generator,
				initialBlogLink, crawlCtx, &curiEqCfg, logger,
			)
		} else {
			postprocessedResult, historicalError = getTumblrApiHistorical(
				parsedFeed.RootLink.Uri.Hostname(), crawlCtx, logger,
			)
		}

		if postprocessedResult != nil {
			var historicalCuris []CanonicalUri
			for _, link := range postprocessedResult.Links {
				historicalCuris = append(historicalCuris, link.Curi)
			}
			historicalCurisSet := NewCanonicalUriSet(historicalCuris, &curiEqCfg)

			var discardedFeedEntryUrls []string
			for _, entryLink := range parsedFeed.EntryLinks.ToSlice() {
				if !historicalCurisSet.Contains(entryLink.Curi) {
					discardedFeedEntryUrls = append(discardedFeedEntryUrls, entryLink.Url)
				}
			}

			historicalMaybeTitledLinks = postprocessedResult.Links
			historicalResult = &HistoricalResult{
				BlogLink:               postprocessedResult.MainLnk,
				MainLink:               postprocessedResult.MainLnk,
				Pattern:                postprocessedResult.Pattern,
				Links:                  nil,
				DiscardedFeedEntryUrls: discardedFeedEntryUrls,
				PostCategories:         postprocessedResult.PostCategories,
				Extra:                  postprocessedResult.Extra,
			}
		}
	} else {
		logger.Info("Feed is long with %d entries", parsedFeed.EntryLinks.Length)

		var postCategories []HistoricalBlogPostCategory
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
			PostCategories:         postCategories,
			Extra:                  postCategoriesExtra,
		}
		historicalMaybeTitledLinks = parsedFeed.EntryLinks.ToMaybeTitledSlice()
	}

	if historicalResult != nil {
		historicalResult.Links = fetchMissingTitles(
			historicalMaybeTitledLinks, &parsedFeed.EntryLinks, &feedEntryCurisTitlesMap,
			parsedFeed.Generator, &curiEqCfg, crawlCtx, logger,
		)
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
		crawlCtx.ProgressLogger.SaveStatus()
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
		crawlCtx.ProgressLogger.SaveStatus()
		if err != nil {
			logger.Info("Couldn't fetch: %v", err)
			continue
		}

		return possibleStartPageLink, possibleStartPage, nil
	}
}

type postprocessedResult struct {
	MainLnk                 Link
	Pattern                 string
	Links                   []*maybeTitledLink
	IsMatchingFeed          bool
	PostCategories          []HistoricalBlogPostCategory
	Extra                   []string
	MaybePartialPagedResult *partialPagedResult
}

func (r *postprocessedResult) mainLink() Link {
	return r.MainLnk
}

func (r *postprocessedResult) speculativeCount() int {
	if r.MaybePartialPagedResult != nil {
		return r.MaybePartialPagedResult.SpeculativeCount()
	}
	return len(r.Links)
}

type linkOrHtmlPage interface {
	linkOrHtmlPageTag()
}

func (*Link) linkOrHtmlPageTag()     {}
func (*htmlPage) linkOrHtmlPageTag() {}

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
	startPage *htmlPage, feedEntryLinks *FeedEntryLinks, feedEntryCurisTitlesMap CanonicalUriMap[*LinkTitle],
	feedGenerator FeedGenerator, initialBlogLink *Link, crawlCtx *CrawlContext,
	curiEqCfg *CanonicalEqualityConfig, logger Logger,
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
		if seenCurisSet.contains(link.Curi) {
			continue
		}

		if archivesRegex.MatchString(link.Curi.TrimmedPath) {
			seenCurisSet.add(link.Curi)
			archivesQueue = append(archivesQueue, link)
			logger.Info("Enqueued archives: %s", link.Url)
		} else if mainPageRegex.MatchString(link.Curi.TrimmedPath) {
			seenCurisSet.add(link.Curi)
			mainPageQueue = append(mainPageQueue, link)
			logger.Info("Enqueued main page: %s", link.Url)
		} else {
			startPageOtherLinks = append(startPageOtherLinks, link)
		}
	}

	if feedGenerator == FeedGeneratorSubstack {
		archiveLink, _ := ToCanonicalLink("/archive", logger, startPage.FetchUri)
		if !seenCurisSet.contains(archiveLink.Curi) {
			logger.Info("Adding missing substack archives: %s", archiveLink.Url)
			archivesQueue = append(archivesQueue, archiveLink)
		}
	}

	logger.Info(
		"Start page and links: %d archives, %d main page, %d others",
		len(archivesQueue), len(mainPageQueue), len(startPageOtherLinks),
	)

	guidedCtx := guidedCrawlContext{
		SeenCurisSet:            seenCurisSet,
		ArchivesCategoriesState: &archivesCategoriesState,
		FeedEntryLinks:          feedEntryLinks,
		FeedEntryCurisTitlesMap: feedEntryCurisTitlesMap,
		FeedGenerator:           feedGenerator,
		CuriEqCfg:               curiEqCfg,
		AllowedHosts:            allowedHosts,
		HardcodedError:          nil,
	}

	result, resultOk := guidedCrawlFetchLoop(
		[]*guidedCrawlQueue{&archivesQueue, &mainPageQueue}, nil, 1, &guidedCtx, crawlCtx, logger,
	)
	if resultOk {
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
			progressLogger.LogAndSaveFetchedCount(nil)
		}
	}

	if feedEntryLinks.Length < 2 {
		return nil, oops.Newf("Too few entries in feed: %d", feedEntryLinks.Length)
	}

	feedEntryLinksSlice := feedEntryLinks.ToSlice()
	entry1Page, err := crawlHtmlPage(&feedEntryLinksSlice[0].Link, crawlCtx, logger)
	progressLogger.SaveStatus()
	if err != nil {
		return nil, oops.Wrap(err)
	}
	entry2Page, err := crawlHtmlPage(&feedEntryLinksSlice[1].Link, crawlCtx, logger)
	progressLogger.SaveStatus()
	if err != nil {
		return nil, oops.Wrap(err)
	}

	entry1Links := extractLinks(
		entry1Page.Document, entry1Page.FetchUri, allowedHosts, crawlCtx.Redirects, logger, includeXPathNone,
	)
	entry2Links := extractLinks(
		entry2Page.Document, entry2Page.FetchUri, allowedHosts, crawlCtx.Redirects, logger, includeXPathNone,
	)
	entry1Curis := ToCanonicalUris(entry1Links)
	entry1CurisSet := NewCanonicalUriSet(entry1Curis, curiEqCfg)

	var twoEntriesLinks []*Link
	entry2CurisSet := NewCanonicalUriSet(nil, curiEqCfg)
	for _, entry2Link := range entry2Links {
		if entry2CurisSet.Contains(entry2Link.Curi) {
			continue
		}

		entry2CurisSet.add(entry2Link.Curi)
		if entry1CurisSet.Contains(entry2Link.Curi) {
			twoEntriesLinks = append(twoEntriesLinks, &entry2Link.Link)
		}
	}

	var twoEntriesOtherLinks []*Link
	for _, link := range twoEntriesLinks {
		if seenCurisSet.contains(link.Curi) {
			continue
		}

		if archivesRegex.MatchString(link.Curi.TrimmedPath) {
			archivesQueue = append(archivesQueue, link)
		} else if mainPageRegex.MatchString(link.Curi.TrimmedPath) {
			mainPageQueue = append(mainPageQueue, link)
		} else {
			twoEntriesOtherLinks = append(twoEntriesOtherLinks, link)
		}
	}

	logger.Info(
		"Phase 2 (first two entries) start links: %d archives, %d main page, %d others",
		len(archivesQueue), len(mainPageQueue), len(twoEntriesOtherLinks),
	)

	result, resultOk = guidedCrawlFetchLoop(
		[]*guidedCrawlQueue{&archivesQueue, &mainPageQueue}, result, 2, &guidedCtx, crawlCtx, logger,
	)
	if resultOk {
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
			progressLogger.LogAndSaveFetchedCount(nil)
		}
	}

	if feedGenerator == FeedGeneratorMedium {
		logger.Info("Skipping phase 3 because Medium")
		if resultOk {
			return result, nil
		} else {
			return nil, ErrPatternNotDetected
		}
	}

	var othersQueue guidedCrawlQueue
	var filteredTwoEntriesOtherLinks []*Link
	for _, link := range twoEntriesOtherLinks {
		if !feedEntryCurisTitlesMap.Contains(link.Curi) {
			filteredTwoEntriesOtherLinks = append(filteredTwoEntriesOtherLinks, link)
		}
	}
	var twiceFilteredTwoEntriesOtherLinks []*Link
	if len(filteredTwoEntriesOtherLinks) > 10 {
		for _, link := range filteredTwoEntriesOtherLinks {
			if !likelyPostRegex.MatchString(link.Curi.TrimmedPath) {
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
		if seenCurisSet.contains(link.Curi) {
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
		feedEntryLinks.ToSlice(), func(entryLink *FeedEntryLink) bool {
			return strings.Count(entryLink.Curi.TrimmedPath, "/") <= 1
		},
	)
	if areAnyFeedEntriesTopLevel {
		logger.Info("Skipping phase 1 other links because some feed entries are top level")
	} else {
		phase1OthersCount := 0
		for _, link := range filteredStartPageOtherLinks {
			if seenCurisSet.contains(link.Curi) {
				continue
			}
			othersQueue = append(othersQueue, link)
			phase1OthersCount++
		}
		logger.Info("Phase 3 links from phase 1: %d", phase1OthersCount)
	}

	result, resultOk = guidedCrawlFetchLoop(
		[]*guidedCrawlQueue{&archivesQueue, &mainPageQueue, &othersQueue}, result, 3, &guidedCtx, crawlCtx,
		logger,
	)
	if resultOk {
		logger.Info("Phase 3 succeeded")
		return result, nil
	}

	return nil, ErrPatternNotDetected
}

type guidedCrawlContext struct {
	SeenCurisSet            guidedSeenCurisSet
	ArchivesCategoriesState *archivesCategoriesState
	FeedEntryLinks          *FeedEntryLinks
	FeedEntryCurisTitlesMap CanonicalUriMap[*LinkTitle]
	FeedGenerator           FeedGenerator
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

func guidedCrawlFetchLoop(
	queues []*guidedCrawlQueue, maybeInitialResult *postprocessedResult, phaseNumber int,
	guidedCtx *guidedCrawlContext, crawlCtx *CrawlContext, logger Logger,
) (*postprocessedResult, bool) {
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
		var link *Link
		var page *htmlPage
		switch lop := linkOrPage.(type) {
		case *Link:
			link = lop
			if crawlCtx.FetchedCuris.Contains(link.Curi) {
				continue
			}

			var err error
			page, err = crawlHtmlPage(link, crawlCtx, logger)
			progressLogger.SaveStatus()
			if err != nil {
				logger.Info("Couldn't fetch link: %v", err)
				continue
			}
		case *htmlPage:
			page = lop
			var ok bool
			link, ok = ToCanonicalLink(page.FetchUri.String(), logger, nil)
			if !ok {
				panic("Couldn't parse page fetch uri as a link")
			}
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
			progressLogger.SaveStatus()
		}

		pageAllLinks := extractLinks(
			puppeteerPage.Document, puppeteerPage.FetchUri, nil, crawlCtx.Redirects, logger,
			includeXPathAndClassXPath,
		)
		for _, pageLink := range pageAllLinks {
			if !guidedCtx.AllowedHosts[pageLink.Uri.Host] {
				continue
			}

			if guidedCtx.SeenCurisSet.contains(pageLink.Curi) {
				continue
			}

			if archivesRegex.MatchString(pageLink.Curi.TrimmedPath) {
				guidedCtx.SeenCurisSet.add(pageLink.Curi)
				*archivesQueue = append(*archivesQueue, &pageLink.Link)
				hadArchives = true
				archivesSeenCount++
				logger.Info("Enqueueing archives link: %s", pageLink.Curi)
			} else if mainPageRegex.MatchString(pageLink.Curi.TrimmedPath) {
				guidedCtx.SeenCurisSet.add(pageLink.Curi)
				*mainPageQueue = append(*mainPageQueue, &pageLink.Link)
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
		for _, pageResult := range pageResults {
			historicalMatchesCount++
			insertSortedResult(&sortedResults, pageResult)
		}

		if hadArchives && len(*archivesQueue) == 0 && len(sortedResults) > 0 {
			if ppResult, ok := postprocessResults(&sortedResults, guidedCtx, crawlCtx, logger); ok {
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
					return ppResult, true
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

	ppResult, ok := postprocessResults(&sortedResults, guidedCtx, crawlCtx, logger)
	logger.Info(
		"Processed/seen: archives %d/%d, main pages %d/%d",
		archivesProcessedCount, archivesSeenCount, mainPagesProcessedCount, mainPagesSeenCount,
	)
	logger.Info("Historical matches: %d", historicalMatchesCount)
	if ok {
		logger.Info(
			"Guided crawl loop finished (phase %d) with best result of %d links",
			phaseNumber, len(ppResult.Links),
		)
		return ppResult, true
	}

	logger.Info("Guided crawl loop finished (phase %d), no result", phaseNumber)
	return nil, false
}

type crawlHistoricalResult interface {
	mainLink() Link
	speculativeCount() int
}

func tryExtractHistorical(
	fetchLink *Link, page *htmlPage, pageLinks []*xpathLink, pageCurisSet *CanonicalUriSet,
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

func postprocessResults(
	sortedResults *[]crawlHistoricalResult, guidedCtx *guidedCrawlContext, crawlCtx *CrawlContext,
	logger Logger,
) (*postprocessedResult, bool) {
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
		var ppOk bool
		switch res := result.(type) {
		case *postprocessedResult:
			if res.MaybePartialPagedResult == nil {
				ppResult, ppOk = res, true
			} else {
				ppResult, ppOk = postprocessPartialPagedResult(
					res.MaybePartialPagedResult, guidedCtx, crawlCtx, logger,
				)
			}
		case *archivesSortedResult:
			ppResult, ppOk = postprocessArchivesSortedResult(res, guidedCtx, crawlCtx, logger)
		case *archivesShuffledResults:
			ppResult, ppOk = postprocessArchivesShuffledResults(res, guidedCtx, crawlCtx, logger)
		case *archivesMediumPinnedEntryResult:
			ppResult, ppOk = postprocessArchivesMediumPinnedEntryResult(res, guidedCtx, crawlCtx, logger)
		case *archivesLongFeedResult:
			ppResult, ppOk = postprocessArchivesLongFeedResult(res), true
		case *ArchivesCategoriesResult:
			ppResult, ppOk = postprocessArchviesCategoriesResult(res, guidedCtx, crawlCtx, logger)
		case *page1Result:
			// If page 1 result looks the best, check just the page 2 in case it was a scam
			ppResult, ppOk = postprocessPage1Result(res, guidedCtx, crawlCtx, logger)
		default:
			panic("Unknown result type")
		}

		if !ppOk {
			logger.Info("Postprocessing failed for %s, continuing", result.mainLink().Url)
			continue
		}

		if len(*sortedResults) == 0 ||
			speculativeCountBetterThan(ppResult, (*sortedResults)[0]) ||
			(ppResult.MaybePartialPagedResult == nil &&
				speculativeCountEqual(ppResult, (*sortedResults)[0])) {

			if ppResult.MaybePartialPagedResult != nil {
				var ok bool
				ppResult, ok = postprocessPartialPagedResult(
					ppResult.MaybePartialPagedResult, guidedCtx, crawlCtx, logger,
				)
				if !ok {
					logger.Info("Postprocessing failed for %s, continuing", result.mainLink().Url)
					continue
				}
			}

			logger.Info("Postprocessing succeeded")
			return ppResult, true
		}

		ppNotMatchingFeed := ""
		if !ppResult.IsMatchingFeed {
			ppNotMatchingFeed = ", not matching feed"
		}
		logger.Info("Inserting back postprocessed %v%s", printResult(ppResult), ppNotMatchingFeed)
		insertSortedResult(sortedResults, ppResult)
	}

	logger.Info("Postprocessing failed")
	return nil, false
}

func printResult(result crawlHistoricalResult) string {
	name := reflect.TypeOf(result).Elem().Name()
	return fmt.Sprintf("[%s, %s, %d]", name, result.mainLink().Url, result.speculativeCount())
}

func postprocessArchivesSortedResult(
	archivesSortedResult *archivesSortedResult, guidedCtx *guidedCrawlContext, crawlCtx *CrawlContext,
	logger Logger,
) (*postprocessedResult, bool) {
	progressLogger := crawlCtx.ProgressLogger
	if CanonicalUriEqual(archivesSortedResult.MainLnk.Curi, hardcodedBenKuhnArchives, guidedCtx.CuriEqCfg) {
		logger.Info("Postprocess archives sorted result start")
		logger.Info("Extra request for Ben Kuhn categories")
		titleCount := countLinkTitles(archivesSortedResult.Links)
		progressLogger.LogAndSaveFetchedCount(&titleCount)
		page, err := crawlHtmlPage(hardcodedBenKuhn, crawlCtx, logger)
		progressLogger.LogAndSavePostprocessing()
		if err != nil {
			logger.Info("Ben Kuhn categories fetch error: %v", err)
			guidedCtx.HardcodedError = oops.Wrapf(err, "Ben Kuhn categories fetch error")
		} else {
			postCategories, err := extractBenKuhnCategories(page, logger)
			if err != nil {
				logger.Info("Ben Kuhn categories extract error: %v", err)
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
				}, true
			}
		}
	}

	return &postprocessedResult{
		MainLnk:                 archivesSortedResult.MainLnk,
		Pattern:                 archivesSortedResult.Pattern,
		Links:                   archivesSortedResult.Links,
		IsMatchingFeed:          true,
		PostCategories:          archivesSortedResult.PostCategories,
		Extra:                   archivesSortedResult.Extra,
		MaybePartialPagedResult: nil,
	}, true
}

func postprocessArchivesShuffledResults(
	archivesShuffledResults *archivesShuffledResults, guidedCtx *guidedCrawlContext, crawlCtx *CrawlContext,
	logger Logger,
) (*postprocessedResult, bool) {
	logger.Info("Postprocess archives shuffled results start")
	sortedTentativeResults := slices.Clone(archivesShuffledResults.Results)
	sort.Slice(sortedTentativeResults, func(i, j int) bool {
		return len(sortedTentativeResults[i].Links) < len(sortedTentativeResults[j].Links)
	})
	archivesShuffledCounts := make([]int, len(sortedTentativeResults))
	for i, tentativeResult := range sortedTentativeResults {
		archivesShuffledCounts[i] = tentativeResult.SpeculativeCount()
	}
	logger.Info("Archives shuffled counts: %v", archivesShuffledCounts)

	var bestResult *postprocessedResult
	bestResultOk := false
	pagesByCanonicalUrl := make(map[string]*htmlPage)
	for _, tentativeResult := range sortedTentativeResults {
		logger.Info("Postprocessing archives shuffled result of %d", tentativeResult.SpeculativeCount())
		sortedLinks, ok := postprocessSortLinksMaybeDates(
			tentativeResult.Links, tentativeResult.MaybeDates, pagesByCanonicalUrl, guidedCtx, crawlCtx,
			logger,
		)
		if !ok {
			logger.Info("Postprocess archives shuffled results finish (iteration failed)")
			return bestResult, bestResultOk
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
		bestResultOk = true
	}

	logger.Info("Postprocess archives shuffled results finish")
	return bestResult, bestResultOk
}

func postprocessArchivesMediumPinnedEntryResult(
	mediumResult *archivesMediumPinnedEntryResult, guidedCtx *guidedCrawlContext, crawlCtx *CrawlContext,
	logger Logger,
) (*postprocessedResult, bool) {
	progressLogger := crawlCtx.ProgressLogger
	logger.Info("Postprocess archives medium pinned entry result start")
	pinnedEntryPage, err := crawlHtmlPage(&mediumResult.PinnedEntryLink.Link, crawlCtx, logger)
	progressLogger.LogAndSavePostprocessing()
	if err != nil {
		logger.Info(
			"Couldn't fetch first Medium link during result postprocess: %s (%v)",
			mediumResult.PinnedEntryLink.Curi, err,
		)
		return nil, false
	}

	allowedHosts := map[string]bool{pinnedEntryPage.FetchUri.Host: true}
	pinnedEntryPageLinks := extractLinks(
		pinnedEntryPage.Document, pinnedEntryPage.FetchUri, allowedHosts, crawlCtx.Redirects, logger,
		includeXPathNone,
	)

	sortedLinks, ok := historicalArchivesMediumSortFinish(
		&mediumResult.PinnedEntryLink, pinnedEntryPageLinks, mediumResult.OtherLinksDates,
		guidedCtx.CuriEqCfg, logger,
	)
	if !ok {
		logger.Info("Couldn't sort links during postprocess archives medium pinned entry result finish")
		return nil, false
	}

	if !compareWithFeed(sortedLinks, guidedCtx.FeedEntryLinks, guidedCtx.CuriEqCfg, logger) {
		logger.Info("Postprocess archives medium pinned entry result not matching feed")
		return nil, false
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
	}, true
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
) (*postprocessedResult, bool) {
	logger.Info("Postprocess archives categories results start")
	sortedLinks, ok := postprocessSortLinksMaybeDates(
		archivesCategoriesResult.Links, archivesCategoriesResult.MaybeDates, make(map[string]*htmlPage),
		guidedCtx, crawlCtx, logger,
	)
	if !ok {
		logger.Info("Postporcess archives categories results failed")
		return nil, false
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
	}, true
}

func postprocessPage1Result(
	page1Result *page1Result, guidedCtx *guidedCrawlContext, crawlCtx *CrawlContext, logger Logger,
) (*postprocessedResult, bool) {
	progressLogger := crawlCtx.ProgressLogger
	logger.Info("Postprocess page1 result start")
	page2, err := crawlHtmlPage(&page1Result.LinkToPage2, crawlCtx, logger)
	progressLogger.LogAndSavePostprocessing()
	if err != nil {
		logger.Info("Page 2 is not a page: %s (%v)", page1Result.LinkToPage2, err)
		return nil, false
	}

	pagedResult, ok := tryExtractPage2(page2, &page1Result.PagedState, guidedCtx, logger)
	if ok {
		titleCount := countLinkTitles(pagedResult.links())
		progressLogger.LogAndSaveFetchedCount(&titleCount)
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
			}, true
		case *fullPagedResult:
			return &postprocessedResult{
				MainLnk:                 r.MainLnk,
				Pattern:                 r.Pattern,
				Links:                   r.Lnks,
				IsMatchingFeed:          true,
				PostCategories:          r.PostCategories,
				Extra:                   r.Extra,
				MaybePartialPagedResult: nil,
			}, true
		default:
			panic("Unknown paged result type")
		}
	}

	logger.Info("Postprocess page1 result finish (failed)")
	return nil, false
}

func postprocessPartialPagedResult(
	partialResult *partialPagedResult, guidedCtx *guidedCrawlContext, crawlCtx *CrawlContext, logger Logger,
) (*postprocessedResult, bool) {
	progressLogger := crawlCtx.ProgressLogger
	logger.Info("Postprocess paged result start")
	var fullResult *fullPagedResult
	for {
		titleCount := countLinkTitles(partialResult.links())
		progressLogger.LogAndSaveFetchedCount(&titleCount)
		page, err := crawlHtmlPage(&partialResult.LinkToNextPage, crawlCtx, logger)
		progressLogger.LogAndSavePostprocessing()
		if err != nil {
			logger.Info(
				"Postprocess paged result failed, page %d is not a page: %s (%v)",
				partialResult.PagedState.PageNumber, partialResult.LinkToNextPage, err,
			)
			return nil, false
		}

		pagedResult, ok := tryExtractNextPage(page, partialResult, guidedCtx, logger)
		if !ok {
			logger.Info("Postprocess paged result failed")
			return nil, false
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
	}

	if CanonicalUriEqual(fullResult.MainLnk.Curi, hardcodedCaseyHandmer, guidedCtx.CuriEqCfg) {
		postCategories, err := crawlCaseyHandmerCategories(fullResult, guidedCtx, crawlCtx, logger)
		if err != nil {
			logger.Info("Couldn't fetch Casey Handmer categories: %v", err)
			guidedCtx.HardcodedError = err
		} else {
			logger.Info("Categories: %s", categoryCountsString(postCategories))
			fullResult.PostCategories = postCategories
		}
	}

	titleCount := countLinkTitles(fullResult.Lnks)
	progressLogger.LogAndSaveFetchedCount(&titleCount)
	logger.Info("Postprocess paged result finish")
	return &postprocessedResult{
		MainLnk:                 fullResult.MainLnk,
		Pattern:                 fullResult.Pattern,
		Links:                   fullResult.Lnks,
		IsMatchingFeed:          true,
		PostCategories:          fullResult.PostCategories,
		Extra:                   fullResult.Extra,
		MaybePartialPagedResult: nil,
	}, true
}

func crawlCaseyHandmerCategories(
	pagedResult *fullPagedResult, guidedCtx *guidedCrawlContext, crawlCtx *CrawlContext, logger Logger,
) ([]HistoricalBlogPostCategory, error) {
	progressLogger := crawlCtx.ProgressLogger
	logger.Info("Extra requests for Casey Handmer categories")
	titleCount := countLinkTitles(pagedResult.Lnks)
	progressLogger.LogAndSaveFetchedCount(&titleCount)

	spaceMisconceptions, err := crawlHtmlPage(hardcodedCaseyHandmerSpaceMisconceptions, crawlCtx, logger)
	progressLogger.LogAndSavePostprocessing()
	if err != nil {
		return nil, err
	}

	marsTrilogy, err := crawlHtmlPage(hardcodedCaseyHandmerMarsTrilogy, crawlCtx, logger)
	progressLogger.LogAndSavePostprocessing()
	if err != nil {
		return nil, err
	}

	futureOfEnergy, err := crawlHtmlPage(hardcodedCaseyHandmerFutureOfEnergy, crawlCtx, logger)
	progressLogger.LogAndSavePostprocessing()
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

type sortedLinks struct {
	Links           []*maybeTitledLink
	AreMatchingFeed bool
	DateXPath       string
	DateSource      dateSourceKind
}

func postprocessSortLinksMaybeDates(
	links []*maybeTitledLink, maybeDates []*date, pagesByCanonicalUrl map[string]*htmlPage,
	guidedCtx *guidedCrawlContext, crawlCtx *CrawlContext, logger Logger,
) (*sortedLinks, bool) {
	feedGenerator := guidedCtx.FeedGenerator
	progressLogger := crawlCtx.ProgressLogger
	logger.Info("Postprocess sort links, maybe dates start")

	var linksWithDates []linkDate
	var linksWithoutDates []*maybeTitledLink
	alreadyFetchedTitlesCount := 0
	for i, link := range links {
		date := maybeDates[i]
		if date != nil {
			linksWithDates = append(linksWithDates, linkDate{
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

	var crawledLinks []*maybeTitledLink
	var linksToCrawl []*maybeTitledLink
	for _, link := range linksWithoutDates {
		if pagesByCanonicalUrl[link.Curi.String()] != nil {
			crawledLinks = append(crawledLinks, link)
		} else {
			linksToCrawl = append(linksToCrawl, link)
		}
	}

	var sortState *sortState
	for _, link := range crawledLinks {
		page := pagesByCanonicalUrl[link.Curi.String()]
		var ok bool
		sortState, ok = historicalArchivesSortAdd(page, feedGenerator, sortState, logger)
		if !ok {
			logger.Info("Postprocess sort links, maybe dates failed during add already crawled")
			return nil, false
		}
	}

	for linkIdx, link := range linksToCrawl {
		page, err := crawlHtmlPage(&link.Link, crawlCtx, logger)
		if err != nil {
			progressLogger.LogAndSavePostprocessingResetCount()
			logger.Info("Couldn't fetch link during result postprocess: %s (%v)", link.Url, err)
			return nil, false
		}

		var ok bool
		sortState, ok = historicalArchivesSortAdd(page, feedGenerator, sortState, logger)
		if !ok {
			progressLogger.LogAndSavePostprocessingResetCount()
			logger.Info("Postprocess sort links, maybe dates failed during add")
			return nil, false
		}

		fetchedCount := alreadyFetchedTitlesCount + len(crawledLinks) + linkIdx + 1
		remainingCount := remainingTitlesCount + len(linksToCrawl) - linkIdx - 1
		progressLogger.LogAndSavePostprocessingCounts(fetchedCount, remainingCount)
		pagesByCanonicalUrl[page.Curi.String()] = page
	}

	resultLinks, dateSource, ok := historicalArchivesSortFinish(
		linksWithDates, linksWithoutDates, sortState, logger,
	)
	if !ok {
		logger.Info("Postprocess sort links, maybe dates failed during finish")
		return nil, false
	}

	areMatchingFeed := compareWithFeed(resultLinks, guidedCtx.FeedEntryLinks, guidedCtx.CuriEqCfg, logger)

	logger.Info("Postprocess sort links, maybe dates finish")
	return &sortedLinks{
		Links:           resultLinks,
		AreMatchingFeed: areMatchingFeed,
		DateXPath:       dateSource.XPath,
		DateSource:      dateSource.DateSource,
	}, true
}

func compareWithFeed(
	sortedLinks []*maybeTitledLink, feedEntryLinks *FeedEntryLinks, curiEqCfg *CanonicalEqualityConfig,
	logger Logger,
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
	feedEntryCurisTitlesMap *CanonicalUriMap[*LinkTitle], feedGenerator FeedGenerator,
	curiEqCfg *CanonicalEqualityConfig, crawlCtx *CrawlContext, logger Logger,
) []*titledLink {
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
		return titledLinks
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
			return titledLinks
		}
	} else {
		linksWithFeedTitles = links
		feedPresentTitlesCount = presentTitlesCount
		feedMissingTitlesCount = missingTitlesCount
	}

	progressLogger.LogAndSaveFetchedCount(&feedPresentTitlesCount)
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
			if err != nil {
				logger.Info("Couldn't fetch link title, going with url: %s (%v)", link.Link.Url, err)
				title = NewLinkTitle(link.Link.Url, LinkTitleSourceUrl, nil)
			} else {
				pageTitle := getPageTitle(page, feedGenerator, logger)
				pageTitles = append(pageTitles, pageTitle)
				pageTitleLinks = append(pageTitleLinks, &link.Link)
				title = NewLinkTitle(pageTitle, LinkTitleSourcePageTitle, nil)
			}

			fetchedTitlesCount++
			progressLogger.LogAndSavePostprocessingCounts(
				feedPresentTitlesCount+fetchedTitlesCount, feedMissingTitlesCount-fetchedTitlesCount,
			)
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
		return titledLinks
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
			progressLogger.LogAndSavePostprocessingCounts(feedPresentTitlesCount+fetchedTitlesCount+1, 1)
			page, err := crawlHtmlPage(&testLink, crawlCtx, logger)
			progressLogger.LogAndSavePostprocessingCounts(feedPresentTitlesCount+fetchedTitlesCount+1, 0)
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
	return titledLinks
}

func countLinkTitles(links []*maybeTitledLink) int {
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
	sort.Slice(sourceCounts, func(i, j int) bool {
		return sourceCounts[i].Count > sourceCounts[j].Count
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
	feedEntryCurisTitlesMap := NewCanonicalUriMap[*LinkTitle](curiEqCfg)
	for _, entryLink := range parsedFeed.EntryLinks.ToSlice() {
		feedEntryCurisTitlesMap.Add(entryLink.Link, entryLink.MaybeTitle)
	}
	guidedCtx := guidedCrawlContext{
		SeenCurisSet:            newGuidedSeenCurisSet(curiEqCfg),
		ArchivesCategoriesState: nil,
		FeedEntryLinks:          &parsedFeed.EntryLinks,
		FeedEntryCurisTitlesMap: feedEntryCurisTitlesMap,
		FeedGenerator:           parsedFeed.Generator,
		CuriEqCfg:               curiEqCfg,
		AllowedHosts:            nil,
		HardcodedError:          nil,
	}

	archivesUrl := rootUrl + "/archive"
	archivesLink, ok := ToCanonicalLink(archivesUrl, logger, nil)
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
			if pageCurisSet.Contains(link.Link.Curi) {
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
		archivesLink, archivesHtmlPage, pageAllLinks, &pageCurisSet, extractionsByStarCount,
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
