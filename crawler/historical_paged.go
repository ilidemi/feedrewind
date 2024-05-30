package crawler

import (
	"feedrewind/util"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/antchfx/htmlquery"
	om "github.com/wk8/go-ordered-map/v2"
	"golang.org/x/net/html"
)

// Paged extraction supports xpaths and class xpaths but actually is only using xpaths
// Paging with class xpaths came out of one weird blog but keeping it just in case

type page1Result struct {
	MainLnk      Link
	LinkToPage2  Link
	MaxPage1Size int
	PagedState   page2State
}

func (r *page1Result) mainLink() Link {
	return r.MainLnk
}

func (r *page1Result) speculativeCount() int {
	return 2*r.MaxPage1Size + 1
}

type page2State struct {
	IsCertain        bool
	PagingPattern    pagingPattern
	Page1            *htmlPage
	Page1Links       []*xpathLink
	Page1Extractions []page1Extraction
	MainLnk          Link
}

type page1Extraction struct {
	MaskedXPath         string
	Links               []*maybeTitledLink
	XPathName           string
	LogLines            []string
	TitleRelativeXPaths []titleRelativeXPath
	PageSize            int
	MaybeExtraFirstLink *maybeTitledLink
	PostCategories      postCategoriesMap
	PostTags            postCategoriesMap
}

var blogspotPostsByDateRegex *regexp.Regexp

func init() {
	blogspotPostsByDateRegex = regexp.MustCompile(`(\(date-outer\)\[)\d+(.+\(post-outer\)\[)\d+`)
}

func tryExtractPage1(
	page1Link *Link, page1 *htmlPage, page1Links []*xpathLink, page1CurisSet *CanonicalUriSet,
	extractionsByStarCount []starCountExtractions, guidedCtx *guidedCrawlContext, logger Logger,
) (crawlHistoricalResult, bool) {
	feedEntryLinks := guidedCtx.FeedEntryLinks
	curiEqCfg := guidedCtx.CuriEqCfg

	linkPatternToPage2, ok := findLinkToPage2(
		page1Links, page1, guidedCtx.FeedGenerator, guidedCtx.CuriEqCfg, logger,
	)
	if !ok {
		return nil, false
	}

	linkToPage2 := linkPatternToPage2.Link
	isPage2Certain := linkPatternToPage2.IsCertain
	pagingPattern := linkPatternToPage2.PagingPattern

	pageOverlappingLinksCount := feedEntryLinks.includedPrefixLength(page1CurisSet)
	logger.Info(
		"Possible page 1: %s (paging pattern: %#v, %d overlaps)",
		page1.Curi, pagingPattern, pageOverlappingLinksCount,
	)

	type PagedMaskedXPathExtraction struct {
		MaskedXPath         string
		UnfilteredLinks     []*maybeTitledHtmlLink
		LogLines            []string
		TitleRelativeXPaths []titleRelativeXPath
		XPathName           string
	}

	var maskedXPathExtractions []PagedMaskedXPathExtraction

	// Blogger has a known pattern with posts grouped by date
	if _, ok := pagingPattern.(*pagingPatternBlogger); ok {
		var page1SameHostLinks []*xpathLink
		for _, link := range page1Links {
			if link.Uri.Host == page1.FetchUri.Host {
				page1SameHostLinks = append(page1SameHostLinks, link)
			}
		}

		var page1LinksGroupedByDate []*xpathLink
		for _, link := range page1SameHostLinks {
			if blogspotPostsByDateRegex.MatchString(link.ClassXPath) {
				page1LinksGroupedByDate = append(page1LinksGroupedByDate, link)
			}
		}

		var page1FeedLinksGroupedByDate []*xpathLink
		for _, link := range page1LinksGroupedByDate {
			if guidedCtx.FeedEntryCurisTitlesMap.Contains(link.Curi) {
				page1FeedLinksGroupedByDate = append(page1FeedLinksGroupedByDate, link)
			}
		}

		if len(page1FeedLinksGroupedByDate) > 0 {
			extractionsByMaskedXPath := om.New[string, *PagedMaskedXPathExtraction]()
			titleXPath := titleRelativeXPath{
				XPath: "",
				Kind:  titleRelativeXPathKindSelf,
			}
			for _, pageFeedLink := range page1FeedLinksGroupedByDate {
				maskedClassXpath := blogspotPostsByDateRegex.ReplaceAllString(
					pageFeedLink.ClassXPath, "$1*$2*",
				)
				extractionsByMaskedXPath.Set(maskedClassXpath, &PagedMaskedXPathExtraction{
					MaskedXPath:         maskedClassXpath,
					UnfilteredLinks:     nil,
					XPathName:           "class_xpath",
					LogLines:            nil,
					TitleRelativeXPaths: []titleRelativeXPath{titleXPath},
				})
			}
			for _, pageLink := range page1LinksGroupedByDate {
				maskedClassXpath := blogspotPostsByDateRegex.ReplaceAllString(
					pageLink.ClassXPath, `$1*$2*`,
				)
				if extraction, ok := extractionsByMaskedXPath.Get(maskedClassXpath); ok {
					titleValue := getElementTitle(pageLink.Element)
					title := NewLinkTitle(titleValue, LinkTitleSourceInnerText, nil)
					unfilteredLinks := &extraction.UnfilteredLinks
					*unfilteredLinks = append(*unfilteredLinks, &maybeTitledHtmlLink{
						maybeTitledLink: maybeTitledLink{
							Link:       pageLink.Link,
							MaybeTitle: &title,
						},
						Element: pageLink.Element,
					})
				}
			}
			for pair := extractionsByMaskedXPath.Oldest(); pair != nil; pair = pair.Next() {
				maskedXPathExtractions = append(maskedXPathExtractions, *pair.Value)
			}
		}
	}

	// For all others, just extract all masked xpaths
	if len(maskedXPathExtractions) == 0 {
		maskedXPathExtractions =
			make([]PagedMaskedXPathExtraction, len(extractionsByStarCount[0].Extractions))
		for i := range extractionsByStarCount[0].Extractions {
			extraction := &extractionsByStarCount[0].Extractions[i]
			maskedXPathExtractions[i] = PagedMaskedXPathExtraction{
				MaskedXPath:         extraction.MaskedXPath,
				UnfilteredLinks:     extraction.UnfilteredLinks,
				LogLines:            extraction.LogLines,
				TitleRelativeXPaths: extraction.TitleRelativeXPaths,
				XPathName:           extraction.XPathName,
			}
		}
	}

	var page1Extractions []page1Extraction
	for _, extraction := range maskedXPathExtractions {
		links := extraction.UnfilteredLinks

		if CanonicalUriEqual(page1Link.Curi, hardcodedCaseyHandmer, curiEqCfg) {
			for i, link := range links {
				if CanonicalUriEqual(link.Link.Curi, feedEntryLinks.LinkBuckets[0][0].Curi, curiEqCfg) {
					links = links[i:]
					break
				}
			}
		}

		curis := ToCanonicalUris(links)
		page2LinkIndex := slices.IndexFunc(curis, func(curi CanonicalUri) bool {
			return CanonicalUriEqual(curi, linkToPage2.Curi, curiEqCfg)
		})
		if page2LinkIndex != -1 {
			continue
		}

		var maybeExtraFirstLink *maybeTitledLink
		_, isOverlapMatching := feedEntryLinks.sequenceMatch(curis, curiEqCfg)
		if isOverlapMatching {
			maybeExtraFirstLink = nil
		} else {
			var isOverlapMinusOneMatching bool
			isOverlapMinusOneMatching, maybeExtraFirstLink =
				feedEntryLinks.sequenceMatchExceptFirst(curis, curiEqCfg)
			if !isOverlapMinusOneMatching {
				continue
			}
		}

		curisSet := NewCanonicalUriSet(curis, curiEqCfg)
		if curisSet.Length != len(curis) {
			logger.Info("Masked XPath %s has duplicates: %v", extraction.MaskedXPath, curis)
			continue
		}

		pageSize := len(links)
		if maybeExtraFirstLink != nil {
			pageSize++
		}
		postCategories, postTags := getPostCategoriesTags(links, extraction.MaskedXPath)
		maybeTitledLinks := dropHtml(links)
		page1Extractions = append(page1Extractions, page1Extraction{
			MaskedXPath:         extraction.MaskedXPath,
			Links:               maybeTitledLinks,
			XPathName:           extraction.XPathName,
			LogLines:            extraction.LogLines,
			TitleRelativeXPaths: extraction.TitleRelativeXPaths,
			PageSize:            pageSize,
			MaybeExtraFirstLink: maybeExtraFirstLink,
			PostCategories:      postCategories,
			PostTags:            postTags,
		})
	}

	if len(page1Extractions) == 0 {
		logger.Info("No good overlap with feed prefix")
		return nil, false
	}

	slices.SortStableFunc(page1Extractions, func(a, b page1Extraction) int {
		return b.PageSize - a.PageSize // descending
	})

	maxPage1Size := page1Extractions[0].PageSize
	logger.Info("Max prefix: %d", maxPage1Size)

	return &page1Result{
		MainLnk:      *page1Link,
		LinkToPage2:  linkToPage2,
		MaxPage1Size: maxPage1Size,
		PagedState: page2State{
			IsCertain:        isPage2Certain,
			PagingPattern:    pagingPattern,
			Page1:            page1,
			Page1Links:       page1Links,
			Page1Extractions: page1Extractions,
			MainLnk:          *page1Link,
		},
	}, true
}

const uncategorized = "Uncategorized"

type pagedResult interface {
	links() []*maybeTitledLink
	pagedResultTag()
}

type fullPagedResult struct {
	MainLnk        Link
	Pattern        string
	Lnks           []*maybeTitledLink
	PostCategories []HistoricalBlogPostCategory
	Extra          []string
}

func (r *fullPagedResult) MainLink() Link {
	return r.MainLnk
}

func (r *fullPagedResult) SpeculativeCount() int {
	return len(r.Lnks)
}

func (r *fullPagedResult) links() []*maybeTitledLink {
	return r.Lnks
}

func (r *fullPagedResult) pagedResultTag() {}

type partialPagedResult struct {
	MainLnk        Link
	LinkToNextPage Link
	NextPageNumber int
	Lnks           []*maybeTitledLink
	PagedState     nextPageState
}

func (r *partialPagedResult) MainLink() Link {
	return r.MainLnk
}

func (r *partialPagedResult) SpeculativeCount() int {
	return len(r.Lnks) + 1
}

func (r *partialPagedResult) links() []*maybeTitledLink {
	return r.Lnks
}

func (r *partialPagedResult) pagedResultTag() {}

type nextPageState struct {
	PagingPattern       pagingPattern
	PageNumber          int
	KnownEntryCurisSet  CanonicalUriSet
	MaskedXPath         string
	TitleRelativeXPaths []titleRelativeXPath
	XPathExtra          []string
	PageSizes           []int
	Page1               *htmlPage
	Page1Links          []*xpathLink
	MainLnk             Link
	PostCategories      postCategoriesMap
	PostTags            postCategoriesMap
}

func tryExtractPage2(
	page2 *htmlPage, page2State *page2State, guidedCtx *guidedCrawlContext, logger Logger,
) (pagedResult, bool) {
	isPage2Certain := page2State.IsCertain
	pagingPattern := page2State.PagingPattern
	page1Link := page2State.MainLnk
	feedEntryLinks := guidedCtx.FeedEntryLinks
	curiEqCfg := guidedCtx.CuriEqCfg

	logger.Info("Possible page 2: %s", page2.Curi)

	var page1EntryLinks []*maybeTitledLink
	var page2EntryLinks []*maybeTitledLink
	var bestMaskedXPath string
	var bestTitleRelativeXPaths []titleRelativeXPath
	var bestXPathExtra []string
	var pageSizes []int
	var page1PostCategories postCategoriesMap
	var page1PostTags postCategoriesMap
	var page2PostCategories postCategoriesMap
	var page2PostTags postCategoriesMap

	allowedHosts := map[string]bool{
		page2.FetchUri.Host: true,
	}
	page2Links := extractLinks(
		page2.Document, page2.FetchUri, allowedHosts, map[string]*Link{}, logger, includeXPathOnly,
	)
	linksToPage3 := pagingPattern.FindLinksToNextPage(page2, page2Links, 3)
	curisToPage3 := ToCanonicalUris(linksToPage3)
	curisToPage3Set := NewCanonicalUriSet(curisToPage3, curiEqCfg)
	if curisToPage3Set.Length > 1 {
		page3Curis := ToCanonicalUris(linksToPage3)
		logger.Info("Multiple links to page 3: %v", page3Curis)
		return nil, false
	}

	neighborPageLinks := []Link{page1Link}
	if curisToPage3Set.Length == 1 {
		neighborPageLinks = append(neighborPageLinks, *linksToPage3[0])
	}

	// Look for matches on page 2 that overlap with feed
	for _, page1Extraction := range page2State.Page1Extractions {
		page1XPathLinks := page1Extraction.Links
		page1MaskedXPath := page1Extraction.MaskedXPath
		page1Size := page1Extraction.PageSize

		var page2XPathLinks []*maybeTitledHtmlLink
		var traverse func(element *html.Node, maskedXPathSuffix string)
		traverse = func(element *html.Node, maskedXPathSuffix string) {
			expectedTag, maybeExpectedClassesStr, expectedIndex, segmentLength :=
				parseFirstXPathSegment(maskedXPathSuffix)
			tagCount := 0
			for child := element.FirstChild; child != nil; child = child.NextSibling {
				tag, ok := getXPathTag(child)
				if !ok || tag != expectedTag {
					continue
				}

				tagCount++

				if expectedIndex == xpathSegmentIndexStar || xpathSegmentIndex(tagCount) == expectedIndex {
					if maybeExpectedClassesStr == nil || *maybeExpectedClassesStr == getClassesStr(child) {
						if len(maskedXPathSuffix) == segmentLength {
							if link, ok := linkFromElement(
								child, page2.FetchUri, page1Extraction.TitleRelativeXPaths, logger,
							); ok {
								page2XPathLinks = append(page2XPathLinks, link)
							}
						} else {
							traverse(child, maskedXPathSuffix[segmentLength:])
						}
					}
				}
			}
		}
		traverse(page2.Document, page1MaskedXPath)

		if len(page2XPathLinks) == 0 {
			continue
		}

		page2XPathCuris := ToCanonicalUris(page2XPathLinks)
		page2XPathCurisSet := NewCanonicalUriSet(page2XPathCuris, curiEqCfg)
		neighborOverlapIndex := slices.IndexFunc(neighborPageLinks, func(link Link) bool {
			return page2XPathCurisSet.Contains(link.Curi)
		})
		if neighborOverlapIndex != -1 {
			continue
		}

		page1OverlapIndex := slices.IndexFunc(page1XPathLinks, func(link *maybeTitledLink) bool {
			return page2XPathCurisSet.Contains(link.Curi)
		})
		if page1OverlapIndex != -1 {
			continue
		}

		if _, ok := feedEntryLinks.subsequenceMatch(page2XPathCuris, page1Size, curiEqCfg); !ok {
			continue
		}

		logLines := slices.Clone(page1Extraction.LogLines)
		var possiblePage1EntryLinks []*maybeTitledLink
		if page1Extraction.MaybeExtraFirstLink == nil {
			possiblePage1EntryLinks = page1XPathLinks
		} else {
			appendLogLinef(&logLines, "the newest post is decorated")
			possiblePage1EntryLinks = make([]*maybeTitledLink, 1, 1+len(page1XPathLinks))
			possiblePage1EntryLinks[0] = page1Extraction.MaybeExtraFirstLink
			possiblePage1EntryLinks = append(possiblePage1EntryLinks, page1XPathLinks...)
		}
		if len(possiblePage1EntryLinks)+len(page2XPathLinks) <=
			len(page1EntryLinks)+len(page2EntryLinks) {
			continue
		}

		page1EntryLinks = possiblePage1EntryLinks
		page2EntryLinks = dropHtml(page2XPathLinks)
		bestMaskedXPath = page1MaskedXPath
		bestTitleRelativeXPaths = page1Extraction.TitleRelativeXPaths
		appendLogLinef(&logLines, "%d page 2 links", len(page2EntryLinks))
		logStr := joinLogLines(logLines)
		bestXPathExtra = []string{
			fmt.Sprintf("xpath: %s%s", page1MaskedXPath, logStr),
		}
		pageSizes = []int{page1Size, len(page2XPathLinks)}
		page1PostCategories = page1Extraction.PostCategories
		page1PostTags = page1Extraction.PostTags
		page2PostCategories, page2PostTags = getPostCategoriesTags(page2XPathLinks, page1MaskedXPath)
		logger.Info("XPath from page 1 looks good for page 2: %s%s", page1MaskedXPath, logStr)
	}

	// See if the first page had some sort of decoration, and links of the seconf page moved under another
	// parent but retained the inner structure
	if _, ok := pagingPattern.(*pagingPatternBlogger); page2EntryLinks == nil && isPage2Certain && !ok {
		for _, page1Extraction := range page2State.Page1Extractions {
			page1XPathLinks := page1Extraction.Links
			page1MaskedXPath := page1Extraction.MaskedXPath
			page1Size := page1Extraction.PageSize

			page2LogLines := []string{"first page is decorated"}

			// For example, start with /div(x)[1]/article(y)[*]/a(z)[1]
			maskedXPathStarIndex := strings.Index(page1MaskedXPath, "*")
			maskedXPathSuffixStart := strings.LastIndex(page1MaskedXPath[:maskedXPathStarIndex], "/")
			maskedXPathSuffix := page1MaskedXPath[maskedXPathSuffixStart:] // /article(y)[*]/a(z)[1]

			var appendMatches func(
				matchingLinks []*maybeTitledHtmlLink, element *html.Node, maskedXPathSuffix string,
			) []*maybeTitledHtmlLink
			appendMatches = func(
				matchingLinks []*maybeTitledHtmlLink, element *html.Node, maskedXPathSuffix string,
			) []*maybeTitledHtmlLink {
				expectedTag, maybeExpectedClassesStr, expectedIndex, segmentLength :=
					parseFirstXPathSegment(maskedXPathSuffix)
				tagCount := 0
				for child := element.FirstChild; child != nil; child = child.NextSibling {
					tag, ok := getXPathTag(child)
					if !ok || tag != expectedTag {
						continue
					}

					tagCount++

					if expectedIndex == xpathSegmentIndexStar ||
						xpathSegmentIndex(tagCount) == expectedIndex {

						if maybeExpectedClassesStr == nil ||
							*maybeExpectedClassesStr == getClassesStr(child) {

							if len(maskedXPathSuffix) == segmentLength {
								if link, ok := linkFromElement(
									child, page2.FetchUri, page1Extraction.TitleRelativeXPaths, logger,
								); ok {
									matchingLinks = append(matchingLinks, link)
								}
							} else {
								matchingLinks = appendMatches(
									matchingLinks, child, maskedXPathSuffix[segmentLength:],
								)
							}
						}
					}
				}

				return matchingLinks
			}

			suffixFirstTag, maybeSuffixFirstClassesStr, _, suffixFirstSegmentLength :=
				parseFirstXPathSegment(maskedXPathSuffix)
			page2MatchingLinksByXPathPrefix := make(map[string][]*maybeTitledHtmlLink)

			var traverse func(element *html.Node, xpathSegments []xpathSegment)
			traverse = func(element *html.Node, xpathSegments []xpathSegment) {
				tagCounts := make(map[string]int)
				for child := element.FirstChild; child != nil; child = child.NextSibling {
					tag, ok := getXPathTag(child)
					if !ok {
						continue
					}
					tagCounts[tag]++

					if tag == suffixFirstTag {
						if maybeSuffixFirstClassesStr == nil ||
							*maybeSuffixFirstClassesStr == getClassesStr(child) {

							var sb strings.Builder
							for _, segment := range xpathSegments {
								sb.WriteRune('/')
								sb.WriteString(segment.Tag)
								sb.WriteRune('[')
								fmt.Fprint(&sb, segment.Index)
								sb.WriteRune(']')
							}
							xpathPrefix := sb.String()
							page2MatchingLinksByXPathPrefix[xpathPrefix] = appendMatches(
								page2MatchingLinksByXPathPrefix[xpathPrefix], child,
								maskedXPathSuffix[suffixFirstSegmentLength:],
							)
						}
					}

					childXPathSegments := slices.Clone(xpathSegments)
					childXPathSegments = append(childXPathSegments, xpathSegment{
						Tag:   tag,
						Index: xpathSegmentIndex(tagCounts[tag]),
					})
					traverse(child, childXPathSegments)
				}
			}

			traverse(page2.Document, nil)

			if len(page2MatchingLinksByXPathPrefix) != 1 {
				continue
			}

			var page2XPathPrefix string // For example, /div(x)[2]
			var page2XPathLinks []*maybeTitledHtmlLink
			for page2XPathPrefix, page2XPathLinks = range page2MatchingLinksByXPathPrefix {
			}
			if len(page2XPathLinks) == 0 {
				continue
			}
			page2MaskedXPath := page2XPathPrefix + maskedXPathSuffix // /div(x)[2]/article(y)[*]/a(z)[1]

			page1XPathCuris := ToCanonicalUris(page1XPathLinks)
			page1XPathCurisSet := NewCanonicalUriSet(page1XPathCuris, curiEqCfg)
			overlapIndex := slices.IndexFunc(page2XPathLinks, func(link *maybeTitledHtmlLink) bool {
				return page1XPathCurisSet.Contains(link.Curi)
			})
			if overlapIndex != -1 {
				continue
			}

			page2XPathCuris := ToCanonicalUris(page2XPathLinks)
			if _, ok := feedEntryLinks.subsequenceMatch(page2XPathCuris, page1Size, curiEqCfg); !ok {
				continue
			}

			page1EntryLinks = page1XPathLinks
			page2EntryLinks = dropHtml(page2XPathLinks)
			bestMaskedXPath = page2MaskedXPath
			bestTitleRelativeXPaths = page1Extraction.TitleRelativeXPaths
			page1LogStr := joinLogLines(page1Extraction.LogLines)
			appendLogLinef(&page2LogLines, "%d links", len(page2EntryLinks))
			page2LogStr := joinLogLines(page2LogLines)
			bestXPathExtra = []string{
				fmt.Sprintf("page1_xpath: %s%s", page1MaskedXPath, page1LogStr),
				fmt.Sprintf("page2_xpath: %s%s", page2MaskedXPath, page2LogStr),
			}
			pageSizes = []int{page1Size, len(page2EntryLinks)}
			page1PostCategories = page1Extraction.PostCategories
			page1PostTags = page1Extraction.PostTags
			page2PostCategories, page2PostTags = getPostCategoriesTags(page2XPathLinks, page2MaskedXPath)
			logger.Info("XPath looks good for page 1: %s%s", page1MaskedXPath, page1LogStr)
			logger.Info("XPath looks good for page 2: %s%s", page2MaskedXPath, page2LogStr)
			break
		}
	}

	if page2EntryLinks == nil {
		logger.Info("Couldn't find an xpath maching page 1 and page 2")
		return nil, false
	}

	entryLinks := append(slices.Clone(page1EntryLinks), page2EntryLinks...)
	postCategoriesMap := page1PostCategories.merge(page2PostCategories)
	postTagsMap := page1PostTags.merge(page2PostTags)

	if len(linksToPage3) == 0 {
		logger.Info("Best count: %d with 2 pages of %v", len(entryLinks), pageSizes)
		pageSizeCountStr := countPageSizesStr(pageSizes)

		var postCategoriesOrTags []HistoricalBlogPostCategory
		postCategories := postCategoriesMap.flatten()
		postTags := postTagsMap.flatten()
		var postCategoriesExtra []string
		if !(len(postCategories) == 1 && postCategories[0].Name == uncategorized) {
			postCategoriesOrTags = postCategories
			postCategoriesStr := categoryCountsString(postCategories)
			logger.Info("Categories: %s", postCategoriesStr)
			appendLogLinef(&postCategoriesExtra, "categories: %s", postCategoriesStr)
		} else if !(len(postTags) == 1 && postTags[0].Name == uncategorized) {
			postCategoriesOrTags = postTags
			postCategoriesStr := categoryCountsString(postTags)
			logger.Info("Categories from tags: %s", postCategoriesStr)
			appendLogLinef(&postCategoriesExtra, "categories from tags: %s", postCategoriesStr)
		} else {
			logger.Info("No categories")
		}

		var extra []string
		appendLogLinef(&extra, "page_count: 2")
		appendLogLinef(&extra, "page_sizes: %s", pageSizeCountStr)
		extra = append(extra, bestXPathExtra...)
		appendLogLinef(&extra, `last_page: <a href="%s">%s</a>`, page2.FetchUri, page2.Curi)
		appendLogLinef(&extra, "paging_pattern: %v", pagingPattern)
		extra = append(extra, postCategoriesExtra...)

		return &fullPagedResult{
			MainLnk:        page2State.MainLnk,
			Pattern:        "paged_last",
			Lnks:           entryLinks,
			PostCategories: postCategoriesOrTags,
			Extra:          extra,
		}, true
	}

	entryCuris := ToCanonicalUris(entryLinks)
	knownEntryCurisSet := NewCanonicalUriSet(entryCuris, curiEqCfg)

	return &partialPagedResult{
		MainLnk:        page2State.MainLnk,
		LinkToNextPage: *linksToPage3[0],
		NextPageNumber: 3,
		Lnks:           entryLinks,
		PagedState: nextPageState{
			PagingPattern:       pagingPattern,
			PageNumber:          3,
			KnownEntryCurisSet:  knownEntryCurisSet,
			MaskedXPath:         bestMaskedXPath,
			TitleRelativeXPaths: bestTitleRelativeXPaths,
			XPathExtra:          bestXPathExtra,
			PageSizes:           pageSizes,
			Page1:               page2State.Page1,
			Page1Links:          page2State.Page1Links,
			MainLnk:             page2State.MainLnk,
			PostCategories:      postCategoriesMap,
			PostTags:            postTagsMap,
		},
	}, true
}

func tryExtractNextPage(
	page *htmlPage, pagedResult *partialPagedResult, guidedCtx *guidedCrawlContext, logger Logger,
) (pagedResult, bool) {
	entryLinks := pagedResult.Lnks
	pagedState := pagedResult.PagedState
	pagingPattern := pagedState.PagingPattern
	pageNumber := pagedState.PageNumber
	knownEntryCurisSet := pagedState.KnownEntryCurisSet
	maskedXPath := pagedState.MaskedXPath
	titleRelativeXPaths := pagedState.TitleRelativeXPaths
	feedEntryLinks := guidedCtx.FeedEntryLinks
	curiEqCfg := guidedCtx.CuriEqCfg

	logger.Info("Possible page %d: %s", pageNumber, page.Curi)

	var pageXPathLinks []*maybeTitledHtmlLink
	var traverse func(element *html.Node, maskedXPathSuffix string)
	traverse = func(element *html.Node, maskedXPathSuffix string) {
		expectedTag, maybeExpectedClassesStr, expectedIndex, segmentLength :=
			parseFirstXPathSegment(maskedXPathSuffix)
		tagCount := 0
		for child := element.FirstChild; child != nil; child = child.NextSibling {
			tag, ok := getXPathTag(child)
			if !ok || tag != expectedTag {
				continue
			}

			tagCount++

			if expectedIndex == xpathSegmentIndexStar || xpathSegmentIndex(tagCount) == expectedIndex {
				if maybeExpectedClassesStr == nil || *maybeExpectedClassesStr == getClassesStr(child) {
					if len(maskedXPathSuffix) == segmentLength {
						if link, ok := linkFromElement(
							child, page.FetchUri, titleRelativeXPaths, logger,
						); ok {
							pageXPathLinks = append(pageXPathLinks, link)
						}
					} else {
						traverse(child, maskedXPathSuffix[segmentLength:])
					}
				}
			}
		}
	}
	traverse(page.Document, maskedXPath)

	if len(pageXPathLinks) == 0 {
		logger.Info("XPath doesn't work for page %d: %s", pageNumber, maskedXPath)
		return nil, false
	}

	var pageKnownCuris []CanonicalUri
	for _, link := range pageXPathLinks {
		if knownEntryCurisSet.Contains(link.Curi) {
			pageKnownCuris = append(pageKnownCuris, link.Curi)
		}
	}
	if len(pageKnownCuris) != 0 {
		logger.Info("Page %d has known links: %v", pageNumber, pageKnownCuris)
	}

	pageEntryCuris := ToCanonicalUris(pageXPathLinks)
	if _, ok := feedEntryLinks.subsequenceMatch(pageEntryCuris, len(entryLinks), curiEqCfg); !ok {
		logger.Info("Page %d doesn't overlap with feed", pageNumber)
		logger.Info("Page urls: %v", pageEntryCuris)
		logger.Info("Feed urls (offset %d): %v", len(entryLinks), feedEntryLinks)
		return nil, false
	}

	allowedHosts := map[string]bool{
		page.FetchUri.Host: true,
	}
	pageLinks := extractLinks(
		page.Document, page.FetchUri, allowedHosts, map[string]*Link{}, logger, includeXPathAndClassXPath,
	)
	nextPageNumber := pageNumber + 1
	linksToNextPage := pagingPattern.FindLinksToNextPage(page, pageLinks, nextPageNumber)
	curisToNextPage := ToCanonicalUris(linksToNextPage)
	curisToNextPageSet := NewCanonicalUriSet(curisToNextPage, curiEqCfg)
	if curisToNextPageSet.Length > 1 {
		curisToNextPage := ToCanonicalUris(linksToNextPage)
		logger.Info("Found multiple links to the next page, can't decide: %v", curisToNextPage)
		return nil, false
	}

	pagePostCategories, pagePostTags := getPostCategoriesTags(pageXPathLinks, maskedXPath)
	postCategoriesMap := pagedState.PostCategories.merge(pagePostCategories)
	postTagsMap := pagedState.PostTags.merge(pagePostTags)

	nextEntryLinks := append(slices.Clone(entryLinks), dropHtml(pageXPathLinks)...)
	nextKnownEntryCurisSet := knownEntryCurisSet.clone()
	nextKnownEntryCurisSet.addMany(pageEntryCuris)
	pageSizes := append(slices.Clone(pagedState.PageSizes), len(pageXPathLinks))

	if curisToNextPageSet.Length == 1 {
		return &partialPagedResult{
			MainLnk:        pagedState.MainLnk,
			LinkToNextPage: *linksToNextPage[0],
			NextPageNumber: nextPageNumber,
			Lnks:           nextEntryLinks,
			PagedState: nextPageState{
				PagingPattern:       pagingPattern,
				PageNumber:          nextPageNumber,
				KnownEntryCurisSet:  nextKnownEntryCurisSet,
				MaskedXPath:         maskedXPath,
				TitleRelativeXPaths: titleRelativeXPaths,
				XPathExtra:          pagedState.XPathExtra,
				PageSizes:           pageSizes,
				Page1:               pagedState.Page1,
				Page1Links:          pagedState.Page1Links,
				MainLnk:             pagedState.MainLnk,
				PostCategories:      postCategoriesMap,
				PostTags:            postTagsMap,
			},
		}, true
	}

	pageCount := pageNumber
	var firstPageLinksToLastPage bool
	if _, ok := pagingPattern.(*pagingPatternBlogger); ok {
		firstPageLinksToLastPage = false
	} else {
		linksToLastPage := pagingPattern.FindLinksToNextPage(
			pagedState.Page1, pagedState.Page1Links, pageCount,
		)
		firstPageLinksToLastPage = len(linksToLastPage) > 0
	}

	logger.Info("Best count: %d with %d pages of %v", len(nextEntryLinks), pageCount, pageSizes)
	pageSizeCountsStr := countPageSizesStr(pageSizes)

	var postCategoriesOrTags []HistoricalBlogPostCategory
	var postCategoriesSource string
	switch {
	case CanonicalUriEqual(pagedState.MainLnk.Curi, hardcodedMrMoneyMustache, curiEqCfg):
		postCategoriesOrTags = hardcodedMrMoneyMustacheCategories
		postCategoriesSource = " hardcoded"
	case CanonicalUriEqual(pagedState.MainLnk.Curi, hardcodedFactorio, curiEqCfg):
		factorioCategories, err := extractFactorioCategories(nextEntryLinks)
		if err != nil {
			logger.Info("Couldn't extract Factorio categories: %v", err)
			guidedCtx.HardcodedError = err
		} else {
			postCategoriesOrTags = factorioCategories
			postCategoriesSource = " hardcoded"
		}
	case CanonicalUriEqual(pagedState.MainLnk.Curi, hardcodedACOUP, curiEqCfg):
		acoupCategoires, err := extractACOUPCategories(nextEntryLinks)
		if err != nil {
			logger.Info("Couldn't extract ACOUP categories: %v", err)
			guidedCtx.HardcodedError = err
		} else {
			postCategoriesOrTags = acoupCategoires
			postCategoriesSource = " hardcoded +"
		}
		postCategoriesOrTags = append(postCategoriesOrTags, postTagsMap.flatten()...)
		postCategoriesSource += " from tags"
	case CanonicalUriEqual(pagedState.MainLnk.Curi, hardcodedCryptographyEngineering, curiEqCfg):
		postCategoriesOrTags = append(
			slices.Clone(hardcodedCryptographyEngineeringCategories),
			postTagsMap.flatten()...,
		)
		postCategoriesSource = " hardcoded + from tags"
	default:
		postCategories := postCategoriesMap.flatten()
		postTags := postTagsMap.flatten()
		if !(len(postCategories) == 1 && postCategories[0].Name == uncategorized) {
			postCategoriesOrTags = postCategories
		} else if !(len(postTags) == 1 && postTags[0].Name == uncategorized) {
			postCategoriesOrTags = postTags
			postCategoriesSource = " from tags"
		}
	}

	var postCategoriesExtra []string
	if len(postCategoriesOrTags) > 0 {
		postCategoriesStr := categoryCountsString(postCategoriesOrTags)
		logger.Info("Categories: %s", postCategoriesStr)
		postCategoriesExtra = []string{
			fmt.Sprintf("categories%s: %s", postCategoriesSource, postCategoriesStr),
		}
	} else {
		logger.Info("No categories")
	}

	var pattern string
	if firstPageLinksToLastPage {
		pattern = "paged_last"
	} else {
		pattern = "paged_next"
	}

	var extra []string
	appendLogLinef(&extra, "page_count: %d", pageCount)
	appendLogLinef(&extra, "page_sizes: %s", pageSizeCountsStr)
	extra = append(extra, pagedState.XPathExtra...)
	appendLogLinef(&extra, `last_page: <a href="%s">%s</a>`, page.FetchUri, page.Curi)
	appendLogLinef(&extra, "paging_pattern: %s", pagingPattern)
	extra = append(extra, postCategoriesExtra...)

	return &fullPagedResult{
		MainLnk:        pagedState.MainLnk,
		Pattern:        pattern,
		Lnks:           nextEntryLinks,
		PostCategories: postCategoriesOrTags,
		Extra:          extra,
	}, true
}

var firstClassXPathTextSegmentRegex *regexp.Regexp
var firstClassXPathSegmentRegex *regexp.Regexp
var firstXPathSegmentRegex *regexp.Regexp

func init() {
	firstClassXPathTextSegmentRegex = regexp.MustCompile(`^/` +
		/**/ `(text\(\))` +
		/**/ `(())` +
		/**/ `\[([^/]+)\]`,
	)
	firstClassXPathSegmentRegex = regexp.MustCompile(`^/` +
		/**/ `([^/]+)` +
		/**/ `(\(` +
		/*  */ `([^)]*)` +
		/**/ `\))` +
		/**/ `\[([^/]+)\]`,
	)
	firstXPathSegmentRegex = regexp.MustCompile(`^/` +
		/**/ `([^/]+)` +
		/**/ `(())` +
		/**/ `\[([^/]+)\]`,
	)
}

func parseFirstXPathSegment(
	maskedXPath string,
) (tag string, maybeClassesStr *string, index xpathSegmentIndex, length int) {
	segmentMatch := firstClassXPathTextSegmentRegex.FindStringSubmatch(maskedXPath)
	if segmentMatch == nil {
		segmentMatch = firstClassXPathSegmentRegex.FindStringSubmatch(maskedXPath)
	}
	if segmentMatch == nil {
		segmentMatch = firstXPathSegmentRegex.FindStringSubmatch(maskedXPath)
	}
	tag = segmentMatch[1]
	if len(segmentMatch[2]) > 0 {
		maybeClassesStr = &segmentMatch[3]
	}
	if segmentMatch[4] == "*" {
		index = xpathSegmentIndexStar
	} else {
		indexInt, _ := strconv.Atoi(segmentMatch[4])
		index = xpathSegmentIndex(indexInt)
	}
	return tag, maybeClassesStr, index, len(segmentMatch[0])
}

type linkPatternToPage2 struct {
	Link          Link
	IsCertain     bool
	PagingPattern pagingPattern
}

var linkToPage2PathRegex *regexp.Regexp
var probableLinkToPage2PathRegex *regexp.Regexp

func init() {
	linkToPage2PathRegex = regexp.MustCompile(`/(?:index-?2|page/?2)[^/\d]*$`)
	probableLinkToPage2PathRegex = regexp.MustCompile(`/2$`)
}

func findLinkToPage2(
	page1Links []*xpathLink, page1 *htmlPage, feedGenerator FeedGenerator, curiEqCfg *CanonicalEqualityConfig,
	logger Logger,
) (*linkPatternToPage2, bool) {
	var bloggerNextPageLinks []*xpathLink
	for _, link := range page1Links {
		if link.Curi.TrimmedPath == "/search" && len(link.Uri.Query()["updated-max"]) == 1 {
			bloggerNextPageLinks = append(bloggerNextPageLinks, link)
		}
	}

	if feedGenerator == FeedGeneratorBlogger && len(bloggerNextPageLinks) > 0 {
		linksToPage2 := bloggerNextPageLinks
		linksToPage2Curis := ToCanonicalUris(linksToPage2)
		linksToPage2CurisSet := NewCanonicalUriSet(linksToPage2Curis, curiEqCfg)
		if linksToPage2CurisSet.Length > 1 {
			logger.Info("Page %s has multiple page 2 links: %v", page1.Curi, linksToPage2Curis)
			return nil, false
		}

		return &linkPatternToPage2{
			Link:          linksToPage2[0].Link,
			IsCertain:     true,
			PagingPattern: &pagingPatternBlogger{},
		}, true
	}

	isCertain := true
	isPathMatch := true
	var linksToPage2 []*xpathLink
	for _, link := range page1Links {
		if link.Curi.Host == page1.Curi.Host && linkToPage2PathRegex.MatchString(link.Curi.TrimmedPath) {
			linksToPage2 = append(linksToPage2, link)
		}
	}

	if len(linksToPage2) == 0 {
		isPathMatch = false
		for _, link := range page1Links {
			if link.Curi.Host == page1.Curi.Host {
				if pageValues, ok := link.Uri.Query()["page"]; ok && len(pageValues) == 1 {
					if pageValues[0] == "2" {
						linksToPage2 = append(linksToPage2, link)
					}
				}
			}
		}
	}

	if len(linksToPage2) == 0 {
		isCertain = false
		isPathMatch = true
		for _, link := range page1Links {
			if link.Curi.Host == page1.Curi.Host &&
				probableLinkToPage2PathRegex.MatchString(link.Curi.TrimmedPath) {

				linksToPage2 = append(linksToPage2, link)
			}
		}
	}

	if len(linksToPage2) == 0 {
		return nil, false
	}

	linksToPage2Curis := ToCanonicalUris(linksToPage2)

	if !isCertain {
		logger.Info(
			"Did not find certain links from %s to page 2 to but found some probable ones: %v",
			page1.Curi, linksToPage2Curis,
		)
	}

	linksToPage2CurisSet := NewCanonicalUriSet(linksToPage2Curis, curiEqCfg)
	if linksToPage2CurisSet.Length > 1 {
		logger.Info("Page %s has multiple page 2 links: %v", page1.Curi, linksToPage2Curis)
		return nil, false
	}

	link := linksToPage2[0]
	if isPathMatch {
		trimmedPath := link.Curi.TrimmedPath
		pageNumberIndex := strings.LastIndex(trimmedPath, "2")
		return &linkPatternToPage2{
			Link:      link.Link,
			IsCertain: isCertain,
			PagingPattern: &pagingPatternPathTemplate{
				Host:       link.Curi.Host,
				PathPrefix: trimmedPath[:pageNumberIndex],
				PathSuffix: trimmedPath[pageNumberIndex+1:],
				IsCertain:  isCertain,
			},
		}, true
	} else {
		return &linkPatternToPage2{
			Link:      link.Link,
			IsCertain: isCertain,
			PagingPattern: &pagingPatternQueryTemplate{
				Host:        link.Curi.Host,
				TrimmedPath: link.Curi.TrimmedPath,
				IsCertain:   isCertain,
			},
		}, true
	}
}

type pagingPattern interface {
	String() string
	FindLinksToNextPage(currentPage *htmlPage, currentPageLinks []*xpathLink, nextPageNumber int) []*Link
}

type pagingPatternBlogger struct{}

func (*pagingPatternBlogger) String() string {
	return ":blogger"
}

func (*pagingPatternBlogger) FindLinksToNextPage(
	currentPage *htmlPage, currentPageLinks []*xpathLink, nextPageNumber int,
) []*Link {
	prevUpdatedMaxValues := currentPage.FetchUri.Query()["updated-max"]
	if currentPage.Curi.TrimmedPath != "/search" || len(prevUpdatedMaxValues) != 1 {
		return nil
	}

	var matchingLinks []*Link
	for _, link := range currentPageLinks {
		if !strings.HasPrefix(link.XPath, "/html[1]/head[1]") && link.Curi.TrimmedPath == "/search" {
			if updatedMaxValues, ok := link.Uri.Query()["updated-max"]; ok && len(updatedMaxValues) == 1 {
				if updatedMaxValues[0] < prevUpdatedMaxValues[0] {
					matchingLinks = append(matchingLinks, &link.Link)
				}
			}
		}
	}
	return matchingLinks
}

type pagingPatternPathTemplate struct {
	Host       string
	PathPrefix string
	PathSuffix string
	IsCertain  bool
}

func (p *pagingPatternPathTemplate) String() string {
	return fmt.Sprintf(
		"{host: %q, path_prefix: %q, path_suffix: %q, is_certain: %t}",
		p.Host, p.PathPrefix, p.PathSuffix, p.IsCertain,
	)
}

func (p *pagingPatternPathTemplate) FindLinksToNextPage(
	_ *htmlPage, currentPageLinks []*xpathLink, nextPageNumber int,
) []*Link {
	var matchingLinks []*Link
	expectedPath := fmt.Sprintf("%s%d%s", p.PathPrefix, nextPageNumber, p.PathSuffix)
	for _, link := range currentPageLinks {
		if link.Curi.Host == p.Host && link.Curi.TrimmedPath == expectedPath {
			matchingLinks = append(matchingLinks, &link.Link)
		}
	}
	return matchingLinks
}

type pagingPatternQueryTemplate struct {
	Host        string
	TrimmedPath string
	IsCertain   bool
}

func (p *pagingPatternQueryTemplate) String() string {
	return fmt.Sprintf("{host: %q, trimmed_path: %q, is_certain: %t}", p.Host, p.TrimmedPath, p.IsCertain)
}

func (p *pagingPatternQueryTemplate) FindLinksToNextPage(
	_ *htmlPage, currentPageLinks []*xpathLink, nextPageNumber int,
) []*Link {
	var matchingLinks []*Link
	expectedQueryValue := fmt.Sprint(nextPageNumber)
	for _, link := range currentPageLinks {
		if link.Curi.Host == p.Host && link.Curi.TrimmedPath == p.TrimmedPath {
			if pageValues, ok := link.Uri.Query()["page"]; ok && len(pageValues) == 1 {
				if pageValues[0] == expectedQueryValue {
					matchingLinks = append(matchingLinks, &link.Link)
				}
			}
		}
	}
	return matchingLinks
}

var categoryRegex *regexp.Regexp
var tagRegex *regexp.Regexp

func init() {
	categoryRegex = regexp.MustCompile(`\bcategory\b`)
	tagRegex = regexp.MustCompile(`\btag\b`)
}

type postCategoriesMap struct {
	CategoriesByLowercaseName map[string]*HistoricalBlogPostCategory
}

func NewPostCategoriesMap() postCategoriesMap {
	return postCategoriesMap{
		CategoriesByLowercaseName: make(map[string]*HistoricalBlogPostCategory),
	}
}

func (c *postCategoriesMap) add(categoryName string, links ...Link) {
	lowercaseName := strings.ToLower(categoryName)
	if category, ok := c.CategoriesByLowercaseName[lowercaseName]; ok {
		category.PostLinks = append(category.PostLinks, links...)
	} else {
		c.CategoriesByLowercaseName[lowercaseName] = &HistoricalBlogPostCategory{
			Name:      categoryName,
			IsTop:     false,
			PostLinks: links,
		}
	}
}

func (c postCategoriesMap) merge(other postCategoriesMap) postCategoriesMap {
	result := NewPostCategoriesMap()
	for lowercaseName, category := range c.CategoriesByLowercaseName {
		result.CategoriesByLowercaseName[lowercaseName] = &HistoricalBlogPostCategory{
			Name:      category.Name,
			IsTop:     category.IsTop,
			PostLinks: slices.Clone(category.PostLinks),
		}
	}
	for lowercaseName, category := range other.CategoriesByLowercaseName {
		if existingCategory, ok := result.CategoriesByLowercaseName[lowercaseName]; ok {
			existingCategory.PostLinks = append(existingCategory.PostLinks, category.PostLinks...)
		} else {
			result.CategoriesByLowercaseName[lowercaseName] = category
		}
	}
	return result
}

func (c postCategoriesMap) flatten() []HistoricalBlogPostCategory {
	categories := make([]HistoricalBlogPostCategory, 0, len(c.CategoriesByLowercaseName))
	for _, category := range c.CategoriesByLowercaseName {
		categories = append(categories, *category)
	}
	slices.SortFunc(categories, func(a, b HistoricalBlogPostCategory) int {
		return len(b.PostLinks) - len(a.PostLinks) // descending
	})
	return categories
}

func getPostCategoriesTags(
	links []*maybeTitledHtmlLink, maskedXPath string,
) (categories, tags postCategoriesMap) {
	lastStarIndex := strings.LastIndex(maskedXPath, "*")
	levelsAtOrAfterStar := strings.Count(maskedXPath[lastStarIndex:], "/")
	categories = NewPostCategoriesMap()
	tags = NewPostCategoriesMap()

	var linksWithoutCategory []Link
	var linksWithoutTag []Link
	for _, link := range links {
		linkTopParent := link.Element
		for i := 0; i < levelsAtOrAfterStar; i++ {
			linkTopParent = linkTopParent.Parent
		}

		hasCategory := false
		hasTag := false
		siblingLinkElements := htmlquery.Find(linkTopParent, "//a")
		for _, linkElement := range siblingLinkElements {
			rel := findAttr(linkElement, "rel")
			if rel == "" {
				continue
			}

			if categoryRegex.MatchString(rel) {
				hasCategory = true
				categoryName := innerText(linkElement)
				categories.add(categoryName, link.Link)
			}
			if tagRegex.MatchString(rel) {
				hasTag = true
				categoryName := innerText(linkElement)
				tags.add(categoryName, link.Link)
			}
		}

		if !hasCategory {
			linksWithoutCategory = append(linksWithoutCategory, link.Link)
		}
		if !hasTag {
			linksWithoutTag = append(linksWithoutTag, link.Link)
		}
	}

	if len(linksWithoutCategory) > 0 {
		categories.add(uncategorized, linksWithoutCategory...)
	}
	if len(linksWithoutTag) > 0 {
		tags.add(uncategorized, linksWithoutTag...)
	}

	return categories, tags
}

func countPageSizesStr(pageSizes []int) string {
	pageSizeCounts := make(map[int]int)
	for _, pageSize := range pageSizes {
		pageSizeCounts[pageSize]++
	}
	sortedPageSizes := util.Keys(pageSizeCounts)
	slices.SortStableFunc(sortedPageSizes, func(a, b int) int {
		return pageSizeCounts[b] - pageSizeCounts[a] // descending
	})
	var sb strings.Builder
	sb.WriteRune('{')
	for i, pageSize := range sortedPageSizes {
		if i > 0 {
			sb.WriteString(", ")
		}
		fmt.Fprintf(&sb, "%d: %d", pageSize, pageSizeCounts[pageSize])
	}
	sb.WriteRune('}')
	return sb.String()
}
