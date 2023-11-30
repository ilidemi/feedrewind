package crawler

import (
	"fmt"
	"sort"
	"strings"

	"golang.org/x/exp/slices"
	"golang.org/x/net/html"
)

type xpathDateSource struct {
	XPath      string
	DateSource dateSourceKind
}

type sortState struct {
	DatesByXPathSource map[xpathDateSource]*[]date
	PageTitles         []string
}

func (s sortState) String() string {
	var sb strings.Builder
	fmt.Fprint(&sb, "{DatesByXPathSource: {")
	isFirst := true
	for xds, dates := range s.DatesByXPathSource {
		if isFirst {
			isFirst = false
		} else {
			fmt.Fprint(&sb, ", ")
		}
		fmt.Fprintf(&sb, "%v: %v", xds, *dates)
	}
	fmt.Fprint(&sb, "}, PageTitles: [")
	for i, title := range s.PageTitles {
		if i != 0 {
			fmt.Fprint(&sb, ", ")
		}
		fmt.Fprintf(&sb, "%q", title)
	}
	fmt.Fprint(&sb, "]}")
	return sb.String()
}

func historicalArchivesSortAdd(
	page *htmlPage, feedGenerator FeedGenerator, maybeSortState *sortState, logger Logger,
) (*sortState, bool) {
	logger.Info("Archives sort add start")

	type DateXPathSource struct {
		XPath      string
		Date       date
		DateSource dateSourceKind
	}
	var pageDatesXPathsSources []DateXPathSource

	var traverse func(element *html.Node, xpathSegments []string)
	traverse = func(element *html.Node, xpathSegments []string) {
		tagCounts := make(map[string]int)
		for child := element.FirstChild; child != nil; child = child.NextSibling {
			tag, ok := getXPathTag(child)
			if !ok {
				continue
			}
			tagCounts[tag]++

			childXPathSegment := fmt.Sprintf("/%s[%d]", tag, tagCounts[tag])
			childXPathSegments := append(xpathSegments, childXPathSegment) // OK to overwrite memory here

			if tag == "meta" && findAttr(child, "property") == "article:published_time" {
				if content := findAttr(child, "content"); content != "" {
					if metaDate := tryExtractTextDate(content, false); metaDate != nil {
						pageDatesXPathsSources = append(pageDatesXPathsSources, DateXPathSource{
							XPath:      strings.Join(childXPathSegments, ""),
							Date:       *metaDate,
							DateSource: dateSourceKindMeta,
						})
					}
				}
			}

			if dateSource := tryExtractElementDate(child, false); dateSource != nil {
				pageDatesXPathsSources = append(pageDatesXPathsSources, DateXPathSource{
					XPath:      strings.Join(childXPathSegments, ""),
					Date:       dateSource.Date,
					DateSource: dateSource.SourceKind,
				})
			}

			traverse(child, childXPathSegments)
		}
	}

	traverse(page.Document, nil)

	pageTitle := getPageTitle(page, feedGenerator, logger)

	var newSortState sortState
	if maybeSortState != nil {
		pageDatesByXPathSource := make(map[xpathDateSource]date)
		for _, xds := range pageDatesXPathsSources {
			xs := xpathDateSource{XPath: xds.XPath, DateSource: xds.DateSource}
			pageDatesByXPathSource[xs] = xds.Date
		}
		pageTitles := slices.Clone(maybeSortState.PageTitles)
		pageTitles = append(pageTitles, pageTitle)
		newSortState = sortState{
			DatesByXPathSource: make(map[xpathDateSource]*[]date),
			PageTitles:         pageTitles,
		}
		for xs, dates := range maybeSortState.DatesByXPathSource {
			if date, ok := pageDatesByXPathSource[xs]; ok {
				newDates := slices.Clone(*dates)
				newDates = append(newDates, date)
				newSortState.DatesByXPathSource[xs] = &newDates
			}
		}
	} else {
		datesByXPathSource := make(map[xpathDateSource]*[]date)
		for _, xds := range pageDatesXPathsSources {
			xs := xpathDateSource{XPath: xds.XPath, DateSource: xds.DateSource}
			dates := []date{xds.Date}
			datesByXPathSource[xs] = &dates
		}
		newSortState = sortState{
			DatesByXPathSource: datesByXPathSource,
			PageTitles:         []string{pageTitle},
		}
	}

	keys := make([]xpathDateSource, 0, len(newSortState.DatesByXPathSource))
	for key := range newSortState.DatesByXPathSource {
		keys = append(keys, key)
	}
	logger.Info("Sort state after %s: %v (%d total)", page.FetchUri, keys, len(newSortState.PageTitles))

	if len(newSortState.DatesByXPathSource) == 0 {
		if maybeSortState != nil {
			logger.Info("Pages don't have a common date path after %s:", page.FetchUri)
			for xs, dates := range maybeSortState.DatesByXPathSource {
				logger.Info("%s -> %v", xs, *dates)
			}
		} else {
			logger.Info("Page doesn't have a date at %s", page.FetchUri)
		}
		return nil, false
	}

	logger.Info("Archives sort add finish")
	return &newSortState, true
}

func historicalArchivesSortFinish(
	linksWithKnownDates []linkDate, links []*maybeTitledLink, maybeSortState *sortState, logger Logger,
) (sortedLinks []*maybeTitledLink, dateSource *xpathDateSource, ok bool) {
	logger.Info("Archives sort finish start")

	var linksDates []linkDate
	if maybeSortState != nil {
		datesByXPathFromMeta := make(map[string]*[]date)
		datesByXPathFromTime := make(map[string]*[]date)
		for xs, dates := range maybeSortState.DatesByXPathSource {
			switch xs.DateSource {
			case dateSourceKindMeta:
				datesByXPathFromMeta[xs.XPath] = dates
			case dateSourceKindTime:
				datesByXPathFromTime[xs.XPath] = dates
			}
		}
		var resultDates []date
		if len(maybeSortState.DatesByXPathSource) == 1 {
			for xs, dates := range maybeSortState.DatesByXPathSource {
				xs := xs
				dateSource = &xs
				resultDates = *dates
				logger.Info("Good shuffled date xpath_source: %s", xs)
			}
		} else if len(datesByXPathFromMeta) == 1 {
			for xpath, dates := range datesByXPathFromMeta {
				dateSource = &xpathDateSource{
					XPath:      xpath,
					DateSource: dateSourceKindMeta,
				}
				resultDates = *dates
				logger.Info("Good shuffled date xpath from meta: %s", xpath)
			}
		} else if len(datesByXPathFromTime) == 1 {
			for xpath, dates := range datesByXPathFromTime {
				dateSource = &xpathDateSource{
					XPath:      xpath,
					DateSource: dateSourceKindTime,
				}
				resultDates = *dates
				logger.Info("Good shuffled date xpath from time: %s", xpath)
			}
		} else {
			logger.Info("Couldn't sort links: %v", *maybeSortState)
			return nil, nil, false
		}

		titleCount := 0
		titledLinks := make([]*maybeTitledLink, len(links))
		for i, link := range links {
			if link.MaybeTitle != nil {
				titledLinks[i] = link
				continue
			}

			title := NewLinkTitle(maybeSortState.PageTitles[i], LinkTitleSourcePageTitle, nil)
			titleCount++
			titledLinks[i] = &maybeTitledLink{
				Link:       link.Link,
				MaybeTitle: &title,
			}
		}
		logger.Info("Set %d link titles from page titles", titleCount)

		linksDates = slices.Clone(linksWithKnownDates)
		for i, link := range links {
			linksDates = append(linksDates, linkDate{
				Link: *link,
				Date: resultDates[i],
			})
		}
	} else {
		linksDates = linksWithKnownDates
		dateSource = &xpathDateSource{
			XPath:      "",
			DateSource: dateSourceKindUnknown,
		}
	}

	sortedLinksDates := sortLinksDates(linksDates)
	sortedLinks = make([]*maybeTitledLink, len(sortedLinksDates))
	for i := range sortedLinksDates {
		sortedLinks[i] = &sortedLinksDates[i].Link
	}

	logger.Info("Archives sort finish finish")
	return sortedLinks, dateSource, true
}

func historicalArchivesMediumSortFinish(
	pinnedEntryLink *maybeTitledLink, pinnedEntryPageLinks []*xpathLink, otherLinksDates []linkDate,
	curiEqCfg *CanonicalEqualityConfig, logger Logger,
) ([]*maybeTitledLink, bool) {
	logger.Info("Archives medium sort finish start")
	var pinnedDate *date
	for _, link := range pinnedEntryPageLinks {
		if !CanonicalUriEqual(pinnedEntryLink.Curi, link.Curi, curiEqCfg) {
			continue
		}

		elementText := innerText(link.Element)
		date := tryExtractTextDate(elementText, true)
		if date == nil {
			continue
		}

		pinnedDate = date
		break
	}

	if pinnedDate == nil {
		logger.Info("Archives medium sort finish finish (failed)")
		return nil, false
	}

	pinnedLinkDate := linkDate{
		Link: *pinnedEntryLink,
		Date: *pinnedDate,
	}
	linksDates := append(slices.Clone(otherLinksDates), pinnedLinkDate)
	sortedLinksDates := sortLinksDates(linksDates)
	sortedLinks := make([]*maybeTitledLink, len(sortedLinksDates))
	for i, linkDate := range sortedLinksDates {
		sortedLinks[i] = &linkDate.Link
	}

	logger.Info("Archives medium sort finish finish")
	return sortedLinks, true
}

type linkDate struct {
	Link maybeTitledLink
	Date date
}

// Sort newest to oldest, preserve link order within the same date
func sortLinksDates(linksDates []linkDate) []linkDate {
	sortedLinksDates := slices.Clone(linksDates)
	sort.SliceStable(sortedLinksDates, func(i, j int) bool {
		return dateCompare(sortedLinksDates[i].Date, sortedLinksDates[j].Date) > 0
	})
	return sortedLinksDates
}
