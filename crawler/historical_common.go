package crawler

import (
	"feedrewind/oops"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/antchfx/htmlquery"
	om "github.com/wk8/go-ordered-map/v2"
	"golang.org/x/net/html"
)

type starCountExtractions struct {
	StarCount   int
	Extractions []maskedXPathExtraction
}

type maskedXPathExtraction struct {
	MaskedXPath                 string
	LinksExtraction             linksExtraction
	UnfilteredLinks             []*maybeTitledHtmlLink
	LogLines                    []string
	MarkupDatesExtraction       sortedDatesExtraction
	MediumMarkupDatesExtraction datesExtraction
	AlmostMarkupDatesExtraction sortedDatesExtraction
	SomeMarkupDatesExtraction   datesExtraction
	MaybeUrlDatesExtraction     maybeDatesExtraction
	TitleRelativeXPaths         []titleRelativeXPath
	XPathName                   string
	DistanceToTopParent         int
}

type linksExtraction struct {
	Links         []*maybeTitledHtmlLink
	Curis         []CanonicalUri
	CurisSet      CanonicalUriSet
	HasDuplicates bool
}

type sortedDatesExtraction struct {
	MaybeDates       []date
	AreSorted        sortedStatus
	AreReverseSorted sortedStatus
	LogLines         []string
}

type sortedStatus int

const (
	sortedStatusUnknown sortedStatus = iota
	sortedStatusNo
	sortedStatusYes
)

type datesExtraction struct {
	MaybeDates []date
	LogLines   []string
}

type maybeDatesExtraction struct {
	MaybeDates []*date
	LogLines   []string
}

type titleRelativeXPath struct {
	XPath string
	Kind  titleRelativeXPathKind
}

func getExtractionsByStarCount(
	pageLinks []*xpathLink, feedGenerator FeedGenerator, feedEntryLinks *FeedEntryLinks,
	feedEntryCurisTitlesMap *CanonicalUriMap[*LinkTitle], curiEqCfg *CanonicalEqualityConfig,
	almostMatchThreshold int, logger Logger,
) []starCountExtractions {
	var extractionsByStarCount []starCountExtractions
	for starCount := 1; starCount <= 3; starCount++ {
		maskedXPathLinkGroupings := groupLinksByMaskedXPath(
			pageLinks, feedEntryCurisTitlesMap, curiEqCfg, starCount,
		)
		logger.Info("Masked xpaths with %d stars: %d", starCount, len(maskedXPathLinkGroupings))

		var maskedXPathExtractions []maskedXPathExtraction
		for _, linksGrouping := range maskedXPathLinkGroupings {
			extraction := getMaskedXPathExtraction(
				linksGrouping, starCount, feedGenerator, feedEntryLinks, feedEntryCurisTitlesMap, curiEqCfg,
				almostMatchThreshold,
			)
			maskedXPathExtractions = append(maskedXPathExtractions, extraction)
		}

		extractionsByStarCount = append(extractionsByStarCount, starCountExtractions{
			StarCount:   starCount,
			Extractions: maskedXPathExtractions,
		})
	}
	return extractionsByStarCount
}

type xpathTreeNode struct {
	XPathSegments []xpathSegment
	Children      *om.OrderedMap[xpathSegment, *xpathTreeNode]
	MaybeParent   *xpathTreeNode
	IsLink        bool
	IsFeedLink    bool
}

type xpathSegment struct {
	Tag   string
	Index xpathSegmentIndex
}

type xpathSegmentIndex int

const xpathSegmentIndexStar xpathSegmentIndex = -1

type maskedXPathLinksGrouping struct {
	MaskedXPath              string
	Links                    []*maybeTitledHtmlLink
	TitleRelativeXPaths      []titleRelativeXPath
	XPathName                string
	DistanceToTopParent      int
	RelativeXPathToTopParent string
	LogLines                 []string
}

func (x titleRelativeXPath) String() string {
	var kind string
	switch x.Kind {
	case titleRelativeXPathKindSelf:
		kind = "self"
	case titleRelativeXPathKindChild:
		kind = "child"
	case titleRelativeXPathKindNeighbor:
		kind = "neighbor"
	default:
		panic(oops.Newf("Unknown kind: %v", x.Kind))
	}
	return fmt.Sprintf("[%q, %s]", x.XPath, kind)
}

type titleRelativeXPathKind int

const (
	titleRelativeXPathKindSelf titleRelativeXPathKind = iota
	titleRelativeXPathKindChild
	titleRelativeXPathKindNeighbor
)

var xpathSegmentRegex *regexp.Regexp

func init() {
	xpathSegmentRegex = regexp.MustCompile(`^([^\[]+)\[(\d+)\]$`)
}

func groupLinksByMaskedXPath(
	pageLinks []*xpathLink, feedEntryCurisTitlesMap *CanonicalUriMap[*LinkTitle],
	curiEqCfg *CanonicalEqualityConfig, starCount int,
) []maskedXPathLinksGrouping {
	xpathToSegments := func(xpath string) []xpathSegment {
		var xpathSegments []xpathSegment
		for _, token := range strings.Split(xpath, "/")[1:] {
			match := xpathSegmentRegex.FindStringSubmatch(token)
			index, _ := strconv.ParseInt(match[2], 10, 64)
			xpathSegments = append(xpathSegments, xpathSegment{
				Tag:   match[1],
				Index: xpathSegmentIndex(index),
			})
		}
		return xpathSegments
	}

	type XPathSegmentsIsFeedLink struct {
		Segments   []xpathSegment
		IsFeedLink bool
	}

	var xpathSegmentsIsFeedLinks []XPathSegmentsIsFeedLink
	for _, pageLink := range pageLinks {
		xpath := pageLink.XPath
		if useClassXPath(starCount) {
			xpath = pageLink.ClassXPath
		}
		xpathSegmentsIsFeedLinks = append(xpathSegmentsIsFeedLinks, XPathSegmentsIsFeedLink{
			Segments:   xpathToSegments(xpath),
			IsFeedLink: feedEntryCurisTitlesMap.Contains(pageLink.Curi),
		})
	}

	buildXPathTree := func(xpathSegmentsIsFeedLinks []XPathSegmentsIsFeedLink) *xpathTreeNode {
		xpathTree := &xpathTreeNode{
			XPathSegments: nil,
			Children:      om.New[xpathSegment, *xpathTreeNode](),
			MaybeParent:   nil,
			IsLink:        false,
			IsFeedLink:    false,
		}

		for _, xpathSegmentsIsFeedLink := range xpathSegmentsIsFeedLinks {
			currentNode := xpathTree
			for segIdx, segment := range xpathSegmentsIsFeedLink.Segments {
				isLink := segIdx == len(xpathSegmentsIsFeedLink.Segments)-1
				isFeedLink := isLink && xpathSegmentsIsFeedLink.IsFeedLink
				if child, ok := currentNode.Children.Get(segment); ok {
					child.IsLink = child.IsLink || isLink
					child.IsFeedLink = child.IsFeedLink || isFeedLink
				} else {
					childXPathSegments := slices.Clone(currentNode.XPathSegments)
					childXPathSegments = append(childXPathSegments, segment)
					currentNode.Children.Set(segment, &xpathTreeNode{
						XPathSegments: childXPathSegments,
						Children:      om.New[xpathSegment, *xpathTreeNode](),
						MaybeParent:   currentNode,
						IsLink:        isLink,
						IsFeedLink:    isFeedLink,
					})
				}
				currentNode, _ = currentNode.Children.Get(segment)
			}
		}
		return xpathTree
	}

	xpathTree := buildXPathTree(xpathSegmentsIsFeedLinks)

	maskedXPathFromSegments := func(xpathSegments []xpathSegment) string {
		var builder strings.Builder
		for _, segment := range xpathSegments {
			builder.WriteRune('/')
			builder.WriteString(segment.Tag)
			builder.WriteRune('[')
			if segment.Index == xpathSegmentIndexStar {
				builder.WriteRune('*')
			} else {
				fmt.Fprint(&builder, segment.Index)
			}
			builder.WriteRune(']')
		}
		return builder.String()
	}

	var addMaskedXPathsSegments func(
		startNode *xpathTreeNode, startXPathSegmentsSuffix []xpathSegment, starsRemaining int,
		pageFeedMaskedXPathsSegments *om.OrderedMap[string, []xpathSegment],
	)
	addMaskedXPathsSegments = func(
		startNode *xpathTreeNode, startXPathSegmentsSuffix []xpathSegment, starsRemaining int,
		pageFeedMaskedXPathsSegments *om.OrderedMap[string, []xpathSegment],
	) {
		maybeAncestorNode := startNode.MaybeParent
		xpathSegmentsSuffix := startXPathSegmentsSuffix
		for maybeAncestorNode != nil {
			ancestorNode := maybeAncestorNode
			childSegment := startNode.XPathSegments[len(ancestorNode.XPathSegments)]
			maskedXPathSegments := slices.Clone(ancestorNode.XPathSegments)
			maskedXPathSegments = append(maskedXPathSegments, xpathSegment{
				Tag:   childSegment.Tag,
				Index: xpathSegmentIndexStar,
			})
			maskedXPathSegments = append(maskedXPathSegments, xpathSegmentsSuffix...)
			maskedXPath := maskedXPathFromSegments(maskedXPathSegments)
			if _, ok := pageFeedMaskedXPathsSegments.Get(maskedXPath); starsRemaining > 1 || !ok {
				foundAnotherLink := false
				for acPair := ancestorNode.Children.Oldest(); acPair != nil; acPair = acPair.Next() {
					ancestorChildSegment, ancestorChild := acPair.Key, acPair.Value
					if !(ancestorChildSegment.Tag == childSegment.Tag &&
						ancestorChildSegment.Index != childSegment.Index) {
						continue
					}

					xpathSegmentsRemaining := xpathSegmentsSuffix
					currentChild := ancestorChild
					for {
						if len(xpathSegmentsRemaining) == 0 {
							if currentChild.IsLink {
								foundAnotherLink = true
							}
							break
						}

						xpathKey := xpathSegmentsRemaining[0]
						for gcPair := currentChild.Children.Oldest(); gcPair != nil; gcPair = gcPair.Next() {
							grandchildSegment, currentGrandchild := gcPair.Key, gcPair.Value
							if grandchildSegment.Tag == xpathKey.Tag &&
								(grandchildSegment.Index == xpathKey.Index ||
									xpathKey.Index == xpathSegmentIndexStar) {

								currentChild = currentGrandchild
								break
							}
						}
						if currentChild == nil {
							break
						}

						xpathSegmentsRemaining = xpathSegmentsRemaining[1:]
					}

					if foundAnotherLink {
						break
					}
				}

				if foundAnotherLink {
					if starsRemaining == 1 {
						pageFeedMaskedXPathsSegments.Set(maskedXPath, maskedXPathSegments)
					} else {
						nextXPathSegmentsSuffix := append([]xpathSegment{{
							Tag:   childSegment.Tag,
							Index: xpathSegmentIndexStar,
						}}, xpathSegmentsSuffix...)
						addMaskedXPathsSegments(
							ancestorNode, nextXPathSegmentsSuffix, starsRemaining-1,
							pageFeedMaskedXPathsSegments,
						)
					}
				}
			}

			xpathSegmentsSuffix = append([]xpathSegment{
				startNode.XPathSegments[len(ancestorNode.XPathSegments)],
			}, xpathSegmentsSuffix...)
			maybeAncestorNode = ancestorNode.MaybeParent
		}
	}

	var traverseXPathTreeFeedLinks func(xpathTreeNodeVal *xpathTreeNode, visitor func(*xpathTreeNode))
	traverseXPathTreeFeedLinks = func(xpathTreeNodeVal *xpathTreeNode, visitor func(*xpathTreeNode)) {
		for pair := xpathTreeNodeVal.Children.Oldest(); pair != nil; pair = pair.Next() {
			childNode := pair.Value
			if childNode.IsFeedLink {
				visitor(childNode)
			}
			traverseXPathTreeFeedLinks(childNode, visitor)
		}
	}

	pageFeedMaskedXPathsSegments := om.New[string, []xpathSegment]()
	traverseXPathTreeFeedLinks(xpathTree, func(linkNode *xpathTreeNode) {
		addMaskedXPathsSegments(linkNode, nil, starCount, pageFeedMaskedXPathsSegments)
	})

	var maskedXPathSegmentsIsFeedLinks []XPathSegmentsIsFeedLink
	for pair := pageFeedMaskedXPathsSegments.Oldest(); pair != nil; pair = pair.Next() {
		maskedXPathSegmentsIsFeedLinks = append(maskedXPathSegmentsIsFeedLinks, XPathSegmentsIsFeedLink{
			Segments:   pair.Value,
			IsFeedLink: false,
		})
	}
	maskedXPathTree := buildXPathTree(maskedXPathSegmentsIsFeedLinks)

	var addLinksMatchingSubtree func(
		currentNode *xpathTreeNode, link *xpathLink, remainingLinkXPathSegments []xpathSegment,
		linksByMaskedXPath *om.OrderedMap[string, []*xpathLink],
	)
	addLinksMatchingSubtree = func(
		currentNode *xpathTreeNode, link *xpathLink, remainingLinkXPathSegments []xpathSegment,
		linksByMaskedXPath *om.OrderedMap[string, []*xpathLink],
	) {
		if len(remainingLinkXPathSegments) == 0 {
			if currentNode.IsLink {
				maskedXPath := maskedXPathFromSegments(currentNode.XPathSegments)
				links, _ := linksByMaskedXPath.Get(maskedXPath)
				linksByMaskedXPath.Set(maskedXPath, append(links, link))
			}
			return
		}

		nextSegment := remainingLinkXPathSegments[0]
		for pair := currentNode.Children.Oldest(); pair != nil; pair = pair.Next() {
			childSegment, child := pair.Key, pair.Value
			if !(childSegment.Tag == nextSegment.Tag &&
				(childSegment.Index == nextSegment.Index || childSegment.Index == xpathSegmentIndexStar)) {
				continue
			}

			addLinksMatchingSubtree(child, link, remainingLinkXPathSegments[1:], linksByMaskedXPath)
		}
	}

	linksByMaskedXPath := om.New[string, []*xpathLink]()
	for linkIdx, pageLink := range pageLinks {
		addLinksMatchingSubtree(
			maskedXPathTree, pageLink, xpathSegmentsIsFeedLinks[linkIdx].Segments, linksByMaskedXPath,
		)
	}

	filteredLinksByMaskedXPath := om.New[string, []*xpathLink]()
	for pair := linksByMaskedXPath.Oldest(); pair != nil; pair = pair.Next() {
		maskedXPath, links := pair.Key, pair.Value
		var curis []CanonicalUri
		for _, link := range links {
			curis = append(curis, link.Curi)
		}
		curiSet := NewCanonicalUriSet(curis, curiEqCfg)
		if curiSet.Length > 1 {
			filteredLinksByMaskedXPath.Set(maskedXPath, links)
		}
	}

	xpathName := "xpath"
	if useClassXPath(starCount) {
		xpathName = "class_xpath"
	}
	var maskedXPathLinkGroupings []maskedXPathLinksGrouping
	for pair := filteredLinksByMaskedXPath.Oldest(); pair != nil; pair = pair.Next() {
		maskedXPath, links := pair.Key, pair.Value
		distanceToTopParent, relativeXPathToTopParent := getTopParentDistanceRelativeXPath(maskedXPath)
		var linksMatchingFeed []*xpathLink
		for _, link := range links {
			if feedEntryCurisTitlesMap.Contains(link.Curi) {
				linksMatchingFeed = append(linksMatchingFeed, link)
			}
		}
		var logLines []string
		titleRelativeXPaths := extractTitleRelativeXPaths(
			linksMatchingFeed, feedEntryCurisTitlesMap, curiEqCfg, distanceToTopParent,
			relativeXPathToTopParent, &logLines,
		)
		var titledLinks []*maybeTitledHtmlLink
		for _, link := range links {
			titledLink := populateLinkTitle(&link.Link, link.Element, titleRelativeXPaths)
			titledLinks = append(titledLinks, titledLink)
		}
		maskedXPathLinkGroupings = append(maskedXPathLinkGroupings, maskedXPathLinksGrouping{
			MaskedXPath:              maskedXPath,
			Links:                    titledLinks,
			TitleRelativeXPaths:      titleRelativeXPaths,
			XPathName:                xpathName,
			DistanceToTopParent:      distanceToTopParent,
			RelativeXPathToTopParent: relativeXPathToTopParent,
			LogLines:                 logLines,
		})
	}

	// Prioritize xpaths with maximum number of original link titles matching feed, then discovered link
	// titles matching feed
	sortKey := func(linksGrouping maskedXPathLinksGrouping) int64 {
		originalTitleMatchingCount := int32(0)
		discoveredTitleMatchingCount := int32(0)
		for _, link := range linksGrouping.Links {
			if feedEntryTitle, ok := feedEntryCurisTitlesMap.Get(link.Curi); ok {
				linkEqualizedTitle := equalizeTitle(getElementTitle(link.Element))
				feedEqualizedTitle := feedEntryTitle.EqualizedValue
				if linkEqualizedTitle == feedEqualizedTitle {
					originalTitleMatchingCount++
				}

				if link.MaybeTitle != nil && link.MaybeTitle.EqualizedValue == feedEqualizedTitle {
					discoveredTitleMatchingCount++
				}
			}
		}
		key := math.MinInt32*int64(originalTitleMatchingCount) - int64(discoveredTitleMatchingCount)
		return key
	}
	slices.SortStableFunc(maskedXPathLinkGroupings, func(a, b maskedXPathLinksGrouping) int {
		diff := sortKey(a) - sortKey(b)
		switch {
		case diff < 0:
			return -1
		case diff > 0:
			return 1
		default:
			return 0
		}
	})

	return maskedXPathLinkGroupings
}

func useClassXPath(starCount int) bool {
	return starCount >= 2
}

func extractTitleRelativeXPaths(
	linksMatchingFeed []*xpathLink, feedEntryCurisTitlesMap *CanonicalUriMap[*LinkTitle],
	curiEqCfg *CanonicalEqualityConfig, distanceToTopParent int, relativeXPathToTopParent string,
	logLines *[]string,
) []titleRelativeXPath {
	var eqLinkTitleValues []string
	var feedTitles []*LinkTitle
	var curis []CanonicalUri
	for _, link := range linksMatchingFeed {
		titleValue := getElementTitle(link.Element)
		eqTitleValue := equalizeTitle(titleValue)
		eqLinkTitleValues = append(eqLinkTitleValues, eqTitleValue)
		feedTitle, _ := feedEntryCurisTitlesMap.Get(link.Curi)
		feedTitles = append(feedTitles, feedTitle)
		curis = append(curis, link.Curi)
	}

	// See if the link inner text just matches
	var curisNotExactlyMatching []CanonicalUri
	for linkIdx, link := range linksMatchingFeed {
		if feedTitles[linkIdx] == nil || eqLinkTitleValues[linkIdx] != feedTitles[linkIdx].EqualizedValue {
			curisNotExactlyMatching = append(curisNotExactlyMatching, link.Curi)
		}
	}

	getAllowedMismatchCount := func(linksCount int) int {
		switch {
		case linksCount <= 8:
			return 0
		case linksCount <= 52:
			return 2
		default:
			return 3
		}
	}

	// Title relative xpaths are discovered before collapsing links. If a link has multiple parts, the title
	// of each part won't match feed, but we should count it as one mismatch and not multiple.
	uniqueMismatchCount := NewCanonicalUriSet(curisNotExactlyMatching, curiEqCfg).Length
	uniqueLinksCount := NewCanonicalUriSet(curis, curiEqCfg).Length

	allowedMismatchCount := getAllowedMismatchCount(uniqueLinksCount)
	if uniqueMismatchCount <= allowedMismatchCount {
		almostLog := "almost"
		if uniqueMismatchCount == 0 {
			almostLog = "exactly"
		}
		*logLines = append(*logLines, fmt.Sprintf("titles %s matching", almostLog))
		return []titleRelativeXPath{{
			XPath: "",
			Kind:  titleRelativeXPathKindSelf,
		}}
	}

	var findTitleMatch func(element *html.Node, feedTitle *LinkTitle, childXPathToSkip string) string
	findTitleMatch = func(element *html.Node, feedTitle *LinkTitle, childXPathToSkip string) string {
		if feedTitle == nil {
			return ""
		}

		tagCounts := make(map[string]int)
		for child := element.FirstChild; child != nil; child = child.NextSibling {
			tag, ok := getXPathTag(child)
			if !ok {
				continue
			}
			tagCounts[tag]++

			childTitleValue := getElementTitle(child)
			if childTitleValue == "" {
				continue
			}

			childXPath := fmt.Sprintf("/%s[%d]", tag, tagCounts[tag])
			if childXPath == childXPathToSkip {
				continue
			}

			eqChildTitleValue := equalizeTitle(childTitleValue)
			if eqChildTitleValue == feedTitle.EqualizedValue {
				return childXPath
			}

			if strings.Contains(eqChildTitleValue, feedTitle.EqualizedValue) {
				grandchildXPath := findTitleMatch(child, feedTitle, "")
				if grandchildXPath != "" {
					return fmt.Sprintf("%s%s", childXPath, grandchildXPath)
				}
			}
		}

		return ""
	}

	// See if there is a child element that matches
	var curisWithoutChildMatch []CanonicalUri
	for linkIdx, link := range linksMatchingFeed {
		if feedTitles[linkIdx] == nil ||
			!strings.Contains(eqLinkTitleValues[linkIdx], feedTitles[linkIdx].EqualizedValue) {

			curisWithoutChildMatch = append(curisWithoutChildMatch, link.Curi)
		}
	}

	uniqueChildMismatchCount := NewCanonicalUriSet(curisWithoutChildMatch, curiEqCfg).Length
	if uniqueChildMismatchCount <= allowedMismatchCount {
		childXPaths := om.New[string, bool]()
		for linkIdx, link := range linksMatchingFeed {
			childXPath := findTitleMatch(link.Element, feedTitles[linkIdx], "")
			childXPaths.Set(childXPath, true)
		}

		for pair := childXPaths.Oldest(); pair != nil; pair = pair.Next() {
			childXPath := pair.Key
			if childXPath == "" {
				continue
			}

			childXPathTitleMismatchCount := 0
			for linkIdx, link := range linksMatchingFeed {
				childElement := htmlquery.FindOne(link.Element, childXPath)
				if childElement == nil ||
					equalizeTitle(getElementTitle(childElement)) != feedTitles[linkIdx].EqualizedValue {

					childXPathTitleMismatchCount++
				}
			}

			if childXPathTitleMismatchCount <= allowedMismatchCount {
				almostLog := "almost"
				if childXPathTitleMismatchCount == 0 {
					almostLog = "exactly"
				}
				logLine := fmt.Sprintf("child titles %s matching at %s", almostLog, childXPath)
				*logLines = append(*logLines, logLine)
				return []titleRelativeXPath{{
					XPath: childXPath,
					Kind:  titleRelativeXPathKindChild,
				}}
			}
		}
	}

	// See if there is a neighbor element that matches
	neighborXPaths := om.New[string, bool]()
	topParentRegex := regexp.MustCompile(fmt.Sprintf(`(/[^/]+){%d}$`, distanceToTopParent))
	for linkIdx, link := range linksMatchingFeed {
		linkXPathRelativeToTopParent := topParentRegex.FindString(link.XPath)
		linkTopParent := link.Element
		for i := 0; i < distanceToTopParent; i++ {
			linkTopParent = linkTopParent.Parent
		}
		titleXPathRelativeToTopParent := findTitleMatch(
			linkTopParent, feedTitles[linkIdx], linkXPathRelativeToTopParent,
		)
		if titleXPathRelativeToTopParent == "" {
			continue
		}

		neighborXPath := fmt.Sprintf("%s%s", relativeXPathToTopParent, titleXPathRelativeToTopParent)
		neighborXPaths.Set(neighborXPath, true)
	}

	var matchingNeighborXPaths []titleRelativeXPath
	for pair := neighborXPaths.Oldest(); pair != nil; pair = pair.Next() {
		neighborXPath := pair.Key
		allTitlesMatching := true
		for linkIdx, link := range linksMatchingFeed {
			neighborElement := htmlquery.FindOne(link.Element, neighborXPath)
			if neighborElement == nil {
				continue
			}

			eqNeighborTitleValue := equalizeTitle(getElementTitle(neighborElement))
			if feedTitles[linkIdx] == nil || eqNeighborTitleValue != feedTitles[linkIdx].EqualizedValue {
				allTitlesMatching = false
				break
			}
		}

		if allTitlesMatching {
			matchingNeighborXPaths = append(matchingNeighborXPaths, titleRelativeXPath{
				XPath: neighborXPath,
				Kind:  titleRelativeXPathKindNeighbor,
			})
		}
	}

	if len(matchingNeighborXPaths) > 0 {
		logLine := fmt.Sprintf("neighbor titles matching at %v", matchingNeighborXPaths)
		*logLines = append(*logLines, logLine)
		return matchingNeighborXPaths
	}

	*logLines = append(*logLines, "titles not matching")
	return nil
}

var urlDateRegex *regexp.Regexp

func init() {
	urlDateRegex = regexp.MustCompile(`/(\d{4})/(\d{2})/(\d{2})/`)
}

func getMaskedXPathExtraction(
	linksGrouping maskedXPathLinksGrouping, starCount int, feedGenerator FeedGenerator,
	feedEntryLinks *FeedEntryLinks, feedEntryCurisTitlesMap *CanonicalUriMap[*LinkTitle],
	curiEqCfg *CanonicalEqualityConfig, almostMatchThreshold int,
) maskedXPathExtraction {
	links := linksGrouping.Links
	logLines := slices.Clone(linksGrouping.LogLines)

	var collapsedLinks []*maybeTitledHtmlLink
	for linkIdx := range links {
		if linkIdx == 0 || !CanonicalUriEqual(links[linkIdx].Curi, links[linkIdx-1].Curi, curiEqCfg) {
			collapsedLinks = append(collapsedLinks, links[linkIdx])
		} else {
			// Merge titles if multiple equal links in a row
			lastLink := collapsedLinks[len(collapsedLinks)-1]
			link := links[linkIdx]
			var newMaybeTitle *LinkTitle
			switch {
			case lastLink.MaybeTitle != nil && link.MaybeTitle != nil:
				newTitleValue := lastLink.MaybeTitle.Value + link.MaybeTitle.Value
				var newTitleSource linkTitleSource
				if lastLink.MaybeTitle.Source == link.MaybeTitle.Source {
					newTitleSource = lastLink.MaybeTitle.Source
				} else {
					newTitleSource = LinkTitleSourceCollapsed
				}
				newTitle := NewLinkTitle(newTitleValue, newTitleSource, nil)
				newMaybeTitle = &newTitle
			case lastLink.MaybeTitle != nil:
				newMaybeTitle = lastLink.MaybeTitle
			default:
				newMaybeTitle = link.MaybeTitle
			}

			collapsedLinks[len(collapsedLinks)-1] = &maybeTitledHtmlLink{
				maybeTitledLink: maybeTitledLink{
					Link:       lastLink.Link,
					MaybeTitle: newMaybeTitle,
				},
				Element: lastLink.Element,
			}
		}
	}

	if len(collapsedLinks) != len(links) {
		appendLogLinef(&logLines, "collapsed %d -> %d links", len(links), len(collapsedLinks))
	} else {
		appendLogLinef(&logLines, "%d links", len(links))
	}

	filteredLinks := collapsedLinks
	markupDatesExtraction := sortedDatesExtraction{
		MaybeDates:       nil,
		AreSorted:        sortedStatusUnknown,
		AreReverseSorted: sortedStatusUnknown,
		LogLines:         []string{"Ø markup dates"},
	}
	mediumMarkupDatesExtraction := datesExtraction{
		MaybeDates: nil,
		LogLines:   []string{"Ø Medium markup dates"},
	}
	almostMarkupDatesExtraction := sortedDatesExtraction{
		MaybeDates:       nil,
		AreSorted:        sortedStatusUnknown,
		AreReverseSorted: sortedStatusUnknown,
		LogLines:         []string{"Ø almost markup dates"},
	}
	someMarkupDatesExtraction := datesExtraction{
		MaybeDates: nil,
		LogLines:   []string{"Ø spme markup dates"},
	}

	var linksMatchingFeed []*maybeTitledHtmlLink
	for _, link := range collapsedLinks {
		if feedEntryCurisTitlesMap.Contains(link.Curi) {
			linksMatchingFeed = append(linksMatchingFeed, link)
		}
	}
	curisMatchingFeed := ToCanonicalUris(linksMatchingFeed)
	uniqueLinksMatchingFeedCount := NewCanonicalUriSet(curisMatchingFeed, curiEqCfg).Length

	matchingMaybeMarkupDates, maybeMarkupDatesLogLines := extractMaybeMarkupDates(
		collapsedLinks, linksMatchingFeed, linksGrouping.DistanceToTopParent,
		linksGrouping.RelativeXPathToTopParent, false,
	)

	if matchingMaybeMarkupDates != nil {
		var matchingMarkupDates []date
		if uniqueLinksMatchingFeedCount > almostMatchThreshold {
			filteredLinks = []*maybeTitledHtmlLink{}
			for linkIdx := 0; linkIdx < len(collapsedLinks); linkIdx++ {
				maybeDate := matchingMaybeMarkupDates[linkIdx]
				if maybeDate != nil {
					filteredLinks = append(filteredLinks, collapsedLinks[linkIdx])
					matchingMarkupDates = append(matchingMarkupDates, *maybeDate)
				}
			}
			if len(filteredLinks) != len(collapsedLinks) {
				appendLogLinef(
					&logLines, "filtered by dates %d -> %d", len(collapsedLinks), len(filteredLinks),
				)
			}
		} else if !slices.Contains(matchingMaybeMarkupDates, nil) {
			matchingMarkupDates = make([]date, len(matchingMaybeMarkupDates))
			for i, maybeDate := range matchingMaybeMarkupDates {
				matchingMarkupDates[i] = *maybeDate
			}
		}

		if matchingMarkupDates != nil {
			matchingAreSorted := sortedStatusUnknown
			matchingAreReverseSorted := sortedStatusUnknown
			if starCount >= 2 {
				matchingAreSorted = sortedStatusYes
				for i := 0; i < len(matchingMarkupDates)-1; i++ {
					if matchingMarkupDates[i].Compare(matchingMarkupDates[i+1]) == -1 {
						matchingAreSorted = sortedStatusNo
						break
					}
				}
				matchingAreReverseSorted = sortedStatusYes
				for i := 0; i < len(matchingMarkupDates)-1; i++ {
					if matchingMarkupDates[i].Compare(matchingMarkupDates[i+1]) == 1 {
						matchingAreReverseSorted = sortedStatusNo
						break
					}
				}
			}
			// Otherwise, trust the ordering of links more than dates

			if uniqueLinksMatchingFeedCount == feedEntryLinks.Length {
				markupDatesLogLines := slices.Clone(maybeMarkupDatesLogLines)
				appendLogLinef(&markupDatesLogLines, "%d markup dates", len(matchingMarkupDates))
				markupDatesExtraction = sortedDatesExtraction{
					MaybeDates:       matchingMarkupDates,
					AreSorted:        matchingAreSorted,
					AreReverseSorted: matchingAreReverseSorted,
					LogLines:         markupDatesLogLines,
				}
			} else if uniqueLinksMatchingFeedCount >= almostMatchThreshold {
				almostMarkupDatesLogLines := slices.Clone(maybeMarkupDatesLogLines)
				appendLogLinef(&almostMarkupDatesLogLines, "%d almost markup dates", len(matchingMarkupDates))
				almostMarkupDatesExtraction = sortedDatesExtraction{
					MaybeDates:       matchingMarkupDates,
					AreSorted:        matchingAreSorted,
					AreReverseSorted: matchingAreReverseSorted,
					LogLines:         almostMarkupDatesLogLines,
				}
			}

			someMarkupDatesLogLines := slices.Clone(maybeMarkupDatesLogLines)
			appendLogLinef(&someMarkupDatesLogLines, "%d some markup dates", len(matchingMarkupDates))
			someMarkupDatesExtraction = datesExtraction{
				MaybeDates: matchingMarkupDates,
				LogLines:   someMarkupDatesLogLines,
			}
		}
	}

	if feedGenerator == FeedGeneratorMedium && uniqueLinksMatchingFeedCount == feedEntryLinks.Length-1 {
		maybeMediumMarkupDates, maybeMediumMarkupDatesLogLines := extractMaybeMarkupDates(
			collapsedLinks, linksMatchingFeed, linksGrouping.DistanceToTopParent,
			linksGrouping.RelativeXPathToTopParent, true,
		)

		if maybeMediumMarkupDates != nil && !slices.Contains(maybeMediumMarkupDates, nil) {
			mediumMarkupDates := make([]date, len(maybeMediumMarkupDates))
			for i, maybeDate := range maybeMediumMarkupDates {
				mediumMarkupDates[i] = *maybeDate
			}
			appendLogLinef(&maybeMediumMarkupDatesLogLines, "%d Medium markup dates", len(mediumMarkupDates))
			mediumMarkupDatesExtraction = datesExtraction{
				MaybeDates: mediumMarkupDates,
				LogLines:   maybeMediumMarkupDatesLogLines,
			}
		}
	}

	filteredCuris := ToCanonicalUris(filteredLinks)
	filteredCurisSet := NewCanonicalUriSet(filteredCuris, curiEqCfg)
	hasDuplicates := filteredCurisSet.Length != len(filteredCuris)
	linksExtraction := linksExtraction{
		Links:         filteredLinks,
		Curis:         filteredCuris,
		CurisSet:      filteredCurisSet,
		HasDuplicates: hasDuplicates,
	}

	maybeUrlDates := make([]*date, len(filteredLinks))
	urlDatesCount := 0
	for i, link := range filteredLinks {
		if link.Curi.Path == "" {
			continue
		}

		dateMatch := urlDateRegex.FindStringSubmatch(link.Curi.Path)
		if dateMatch == nil {
			continue
		}

		year, _ := strconv.Atoi(dateMatch[1])
		month, _ := strconv.Atoi(dateMatch[2])
		if month < 1 || month > 12 {
			continue
		}
		day, _ := strconv.Atoi(dateMatch[3])
		if day < 1 || day > 31 {
			continue
		}

		maybeUrlDates[i] = &date{
			Year:  year,
			Month: time.Month(month),
			Day:   day,
		}
		urlDatesCount++
	}

	maybeUrlDatesExtraction := maybeDatesExtraction{
		MaybeDates: maybeUrlDates,
		LogLines: []string{
			fmt.Sprintf("%d/%d url dates", urlDatesCount, len(maybeUrlDates)),
		},
	}

	return maskedXPathExtraction{
		MaskedXPath:                 linksGrouping.MaskedXPath,
		LinksExtraction:             linksExtraction,
		UnfilteredLinks:             collapsedLinks,
		LogLines:                    logLines,
		MarkupDatesExtraction:       markupDatesExtraction,
		MediumMarkupDatesExtraction: mediumMarkupDatesExtraction,
		AlmostMarkupDatesExtraction: almostMarkupDatesExtraction,
		SomeMarkupDatesExtraction:   someMarkupDatesExtraction,
		MaybeUrlDatesExtraction:     maybeUrlDatesExtraction,
		TitleRelativeXPaths:         linksGrouping.TitleRelativeXPaths,
		XPathName:                   linksGrouping.XPathName,
		DistanceToTopParent:         linksGrouping.DistanceToTopParent,
	}
}

func getTopParentDistanceRelativeXPath(maskedXPath string) (int, string) {
	lastStarIndex := strings.LastIndex(maskedXPath, "*")
	distanceToTopParent := strings.Count(maskedXPath[lastStarIndex:], "/")
	relativeXPathToTopParent := ""
	if distanceToTopParent > 0 {
		var builder strings.Builder
		for i := 0; i < distanceToTopParent; i++ {
			if i > 0 {
				builder.WriteString("/")
			}
			builder.WriteString("..")
		}
		relativeXPathToTopParent = builder.String()
	}
	return distanceToTopParent, relativeXPathToTopParent
}

func extractMaybeMarkupDates(
	links []*maybeTitledHtmlLink, linksMatchingFeed []*maybeTitledHtmlLink, distanceToTopParent int,
	relativeXPathToTopParent string, guessYear bool,
) ([]*date, []string) {
	type DateXPath struct {
		RelativeXPath string
		Kind          dateSourceKind
	}
	var dateXPaths []DateXPath
	for linkIdx, link := range linksMatchingFeed {
		linkTopParent := link.Element
		for i := 0; i < distanceToTopParent; i++ {
			linkTopParent = linkTopParent.Parent
		}
		var linkDateXPaths []DateXPath

		var collectNeighborDates func(element *html.Node, relativeXPathFromTopParent string)
		collectNeighborDates = func(element *html.Node, relativeXPathFromTopParent string) {
			ds := tryExtractElementDate(element, guessYear)
			if ds != nil {
				dateRelativeXPath := relativeXPathToTopParent + relativeXPathFromTopParent
				linkDateXPaths = append(linkDateXPaths, DateXPath{
					RelativeXPath: dateRelativeXPath,
					Kind:          ds.SourceKind,
				})
			}

			tagCounts := make(map[string]int)
			for child := element.FirstChild; child != nil; child = child.NextSibling {
				tag, ok := getXPathTag(child)
				if !ok {
					continue
				}
				tagCounts[tag]++

				childXPath := fmt.Sprintf(
					"%s/%s[%d]", relativeXPathFromTopParent, tag, tagCounts[tag],
				)
				collectNeighborDates(child, childXPath)
			}
		}

		collectNeighborDates(linkTopParent, "")

		if linkIdx == 0 {
			dateXPaths = linkDateXPaths
		} else {
			var newDateXPaths []DateXPath
			for _, dateXPath := range dateXPaths {
				found := false
				for _, linkDateXPath := range linkDateXPaths {
					if linkDateXPath == dateXPath {
						found = true
						break
					}
				}
				if found {
					newDateXPaths = append(newDateXPaths, dateXPath)
				}
			}
			dateXPaths = newDateXPaths
		}
	}

	var datesXPathsFromTime []string
	for _, dateXPath := range dateXPaths {
		if dateXPath.Kind == dateSourceKindTime {
			datesXPathsFromTime = append(datesXPathsFromTime, dateXPath.RelativeXPath)
		}
	}
	var dateRelativeXPath string
	var dateRelativeXPathFound bool
	var logLine string
	switch {
	case len(dateXPaths) == 1:
		dateRelativeXPath = dateXPaths[0].RelativeXPath
		dateRelativeXPathFound = true
		logLine = fmt.Sprintf("single date XPath: %s", dateRelativeXPath)
	case len(datesXPathsFromTime) == 1:
		dateRelativeXPath = datesXPathsFromTime[0]
		dateRelativeXPathFound = true
		logLine = fmt.Sprintf(
			"multiple date XPaths (%d), one from time: %s", len(dateXPaths), dateRelativeXPath,
		)
	case len(dateXPaths) > 0:
		dateRelativeXPathFound = false
		logLine = fmt.Sprintf("multiple date XPaths (%d), no way to resolve", len(dateXPaths))
	default:
		dateRelativeXPathFound = false
		logLine = "no date XPath"
	}
	logLines := []string{logLine}
	if !dateRelativeXPathFound {
		return nil, logLines
	}

	var maybeDates []*date
	for _, link := range links {
		dateElement := htmlquery.FindOne(link.Element, dateRelativeXPath)
		ds := tryExtractElementDate(dateElement, guessYear)
		var maybeDate *date
		if ds != nil {
			maybeDate = &ds.Date
		}
		maybeDates = append(maybeDates, maybeDate)
	}

	return maybeDates, logLines
}

func populateLinkTitle(
	link *Link, element *html.Node, titleRelativeXPaths []titleRelativeXPath,
) *maybeTitledHtmlLink {
	if len(titleRelativeXPaths) == 0 {
		return &maybeTitledHtmlLink{
			maybeTitledLink: maybeTitledLink{
				Link:       *link,
				MaybeTitle: nil,
			},
			Element: element,
		}
	}

	var titleValue string
	var source linkTitleSource
	alternativeValuesBySource := make(map[linkTitleSource]string)
	for _, titleRelativeXPath := range titleRelativeXPaths {
		var titleElement *html.Node
		if titleRelativeXPath.Kind == titleRelativeXPathKindSelf {
			titleElement = element
		} else {
			titleElement = htmlquery.FindOne(element, titleRelativeXPath.XPath)
		}

		if titleElement == nil {
			continue
		}

		sourceXPath := linkTitleSource(titleRelativeXPath.XPath)
		if titleValue != "" {
			alternativeValuesBySource[sourceXPath] = getElementTitle(titleElement)
		} else {
			titleValue = getElementTitle(titleElement)
			source = sourceXPath
		}
	}

	if titleValue == "" {
		return &maybeTitledHtmlLink{
			maybeTitledLink: maybeTitledLink{
				Link:       *link,
				MaybeTitle: nil,
			},
			Element: element,
		}
	}

	title := NewLinkTitle(titleValue, source, alternativeValuesBySource)
	return &maybeTitledHtmlLink{
		maybeTitledLink: maybeTitledLink{
			Link:       *link,
			MaybeTitle: &title,
		},
		Element: element,
	}
}

func joinLogLines(logLines []string) string {
	if len(logLines) == 0 {
		return ""
	} else {
		return fmt.Sprintf(" (%s)", strings.Join(logLines, ", "))
	}
}

func appendLogLinef(logLines *[]string, format string, args ...any) {
	logLine := fmt.Sprintf(format, args...)
	*logLines = append(*logLines, logLine)
}
