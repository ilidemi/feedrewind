package crawler

import (
	"fmt"
	"slices"
	"strings"

	"feedrewind.com/util"

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

type dateXPathSource struct {
	XPath      string
	Date       date
	DateSource dateSourceKind
}

type sortablePage struct {
	Url                string
	Title              string
	DatesXPathsSources []dateXPathSource
}

func historicalArchivesToSortablePage(
	page *htmlPage, feedGenerator FeedGenerator, logger Logger,
) sortablePage {
	var datesXPathsSources []dateXPathSource
	var traverse func(element *html.Node, xpathSegments []string)
	traverse = func(element *html.Node, xpathSegments []string) {
		tagCounts := make(map[string]int)
		for child := element.FirstChild; child != nil; child = child.NextSibling {
			tag, ok := getXPathTag(child)
			if !ok {
				continue
			}
			tagCounts[tag]++

			if tag == "meta" && findAttr(child, "property") == "article:published_time" {
				if content := findAttr(child, "content"); content != "" {
					if metaDate := tryExtractTextDate(content, false); metaDate != nil {
						lastSegment := "/meta[@property='article:published_time']"
						datesXPathsSources = append(datesXPathsSources, dateXPathSource{
							XPath:      strings.Join(xpathSegments, "") + lastSegment,
							Date:       *metaDate,
							DateSource: dateSourceKindMeta,
						})
					}
				}
			}

			gwernDate := false
			if tag == "meta" && findAttr(child, "name") == "dc.date.issued" {
				if content := findAttr(child, "content"); content != "" {
					if metaDate := tryExtractTextDate(content, false); metaDate != nil {
						gwernDate = true
						lastSegment := "/meta[@name='dc.date.issued']"
						datesXPathsSources = append(datesXPathsSources, dateXPathSource{
							XPath:      strings.Join(xpathSegments, "") + lastSegment,
							Date:       *metaDate,
							DateSource: dateSourceKindMeta,
						})
					}
				}
			}
			if !gwernDate {
				_ = 1
			}

			childXPathSegment := fmt.Sprintf("/%s[%d]", tag, tagCounts[tag])
			childXPathSegments := slices.Clone(xpathSegments)
			childXPathSegments = append(childXPathSegments, childXPathSegment)

			if dateSource := tryExtractElementDate(child, false); dateSource != nil {
				datesXPathsSources = append(datesXPathsSources, dateXPathSource{
					XPath:      strings.Join(childXPathSegments, ""),
					Date:       dateSource.Date,
					DateSource: dateSource.SourceKind,
				})
			}

			traverse(child, childXPathSegments)
		}
	}

	traverse(page.Document, nil)

	pageTitle := strings.Clone(getPageTitle(page, feedGenerator, logger))

	// The resulting memory is not referencing the html
	return sortablePage{
		Url:                page.FetchUri.String(),
		Title:              pageTitle,
		DatesXPathsSources: datesXPathsSources,
	}
}

func historicalArchivesSortAdd(
	page sortablePage, maybeSortState *sortState, logger Logger,
) (*sortState, bool) {
	logger.Info("Archives sort add start")

	var newSortState sortState
	if maybeSortState != nil {
		pageDatesByXPathSource := make(map[xpathDateSource]date)
		for _, xds := range page.DatesXPathsSources {
			xs := xpathDateSource{XPath: xds.XPath, DateSource: xds.DateSource}
			pageDatesByXPathSource[xs] = xds.Date
		}
		pageTitles := slices.Clone(maybeSortState.PageTitles)
		pageTitles = append(pageTitles, page.Title)
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
		for _, xds := range page.DatesXPathsSources {
			xs := xpathDateSource{XPath: xds.XPath, DateSource: xds.DateSource}
			dates := []date{xds.Date}
			datesByXPathSource[xs] = &dates
		}
		newSortState = sortState{
			DatesByXPathSource: datesByXPathSource,
			PageTitles:         []string{page.Title},
		}
	}

	keys := util.Keys(newSortState.DatesByXPathSource)
	logger.Info("Sort state after %s: %v (%d total)", page.Url, keys, len(newSortState.PageTitles))

	if len(newSortState.DatesByXPathSource) == 0 {
		if maybeSortState != nil {
			logger.Info("Pages don't have a common date path after %s:", page.Url)
			for xs, dates := range maybeSortState.DatesByXPathSource {
				logger.Info("%s -> %v", xs, *dates)
			}
		} else {
			logger.Info("Page doesn't have a date at %s", page.Url)
		}
		return nil, false
	}

	logger.Info("Archives sort add finish")
	return &newSortState, true
}

func historicalArchivesSortFinish(
	linksWithKnownDates []linkDate[pristineMaybeTitledLink], links []*pristineMaybeTitledLink,
	maybeSortState *sortState, logger Logger,
) (sortedLinks []*pristineMaybeTitledLink, dateSource *xpathDateSource, ok bool) {
	logger.Info("Archives sort finish start")

	var linksDates []linkDate[pristineMaybeTitledLink]
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
		switch {
		case len(maybeSortState.DatesByXPathSource) == 1:
			for xs, dates := range maybeSortState.DatesByXPathSource {
				xs := xs
				dateSource = &xs
				resultDates = *dates
				logger.Info("Good shuffled date xpath_source: %s", xs)
			}
		case len(datesByXPathFromMeta) == 1:
			for xpath, dates := range datesByXPathFromMeta {
				dateSource = &xpathDateSource{
					XPath:      xpath,
					DateSource: dateSourceKindMeta,
				}
				resultDates = *dates
				logger.Info("Good shuffled date xpath from meta: %s", xpath)
			}
		case len(datesByXPathFromTime) == 1:
			for xpath, dates := range datesByXPathFromTime {
				dateSource = &xpathDateSource{
					XPath:      xpath,
					DateSource: dateSourceKindTime,
				}
				resultDates = *dates
				logger.Info("Good shuffled date xpath from time: %s", xpath)
			}
		default:
			logger.Info("Couldn't sort links: %v", *maybeSortState)
			return nil, nil, false
		}

		titleCount := 0
		titledLinks := make([]*pristineMaybeTitledLink, len(links))
		for i, link := range links {
			if link.MaybeTitle != nil {
				titledLinks[i] = link
				continue
			}

			title := NewPristineLinkTitle(NewLinkTitle(
				maybeSortState.PageTitles[i], LinkTitleSourcePageTitle, nil,
			))
			titleCount++
			titledLinks[i] = &pristineMaybeTitledLink{
				Link:       link.Link,
				MaybeTitle: &title,
			}
		}
		logger.Info("Set %d link titles from page titles", titleCount)

		linksDates = slices.Clone(linksWithKnownDates)
		for i, link := range links {
			linksDates = append(linksDates, linkDate[pristineMaybeTitledLink]{
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
	sortedLinks = make([]*pristineMaybeTitledLink, len(sortedLinksDates))
	for i := range sortedLinksDates {
		sortedLinks[i] = &sortedLinksDates[i].Link
	}

	logger.Info("Archives sort finish finish")
	return sortedLinks, dateSource, true
}

func historicalArchivesMediumSortFinish(
	pinnedEntryLink *pristineMaybeTitledLink, pinnedEntryPageLinks []*xpathLink,
	otherLinksDates []linkDate[pristineMaybeTitledLink], curiEqCfg *CanonicalEqualityConfig, logger Logger,
) ([]*pristineMaybeTitledLink, bool) {
	logger.Info("Archives medium sort finish start")
	var pinnedDate *date
	for _, link := range pinnedEntryPageLinks {
		if !CanonicalUriEqual(pinnedEntryLink.Link.Curi(), link.Curi, curiEqCfg) {
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

	pinnedLinkDate := linkDate[pristineMaybeTitledLink]{
		Link: *pinnedEntryLink,
		Date: *pinnedDate,
	}
	linksDates := append(slices.Clone(otherLinksDates), pinnedLinkDate)
	sortedLinksDates := sortLinksDates(linksDates)
	sortedLinks := make([]*pristineMaybeTitledLink, len(sortedLinksDates))
	for i, linkDate := range sortedLinksDates {
		sortedLinks[i] = &linkDate.Link
	}

	logger.Info("Archives medium sort finish finish")
	return sortedLinks, true
}

type linkDate[Link any] struct {
	Link Link
	Date date
}

// Sort newest to oldest, preserve link order within the same date
func sortLinksDates[Link any](linksDates []linkDate[Link]) []linkDate[Link] {
	sortedLinksDates := slices.Clone(linksDates)
	slices.SortStableFunc(sortedLinksDates, func(a, b linkDate[Link]) int {
		return b.Date.Compare(a.Date)
	})
	return sortedLinksDates
}
