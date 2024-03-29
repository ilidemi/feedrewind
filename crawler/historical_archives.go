package crawler

import (
	"fmt"
	"strings"

	"golang.org/x/exp/slices"
	"golang.org/x/net/html"
)

type archivesSortedResult struct {
	MainLnk        Link
	Pattern        string
	Links          []*maybeTitledLink
	HasDates       bool
	PostCategories []HistoricalBlogPostCategory
	Extra          []string
}

func (r *archivesSortedResult) mainLink() Link {
	return r.MainLnk
}

func (r *archivesSortedResult) speculativeCount() int {
	return len(r.Links)
}

type archivesMediumPinnedEntryResult struct {
	MainLnk         Link
	Pattern         string
	PinnedEntryLink maybeTitledLink
	OtherLinksDates []linkDate
	Extra           []string
}

func (r *archivesMediumPinnedEntryResult) mainLink() Link {
	return r.MainLnk
}

func (r *archivesMediumPinnedEntryResult) speculativeCount() int {
	return len(r.OtherLinksDates) + 1
}

type archivesShuffledResult struct {
	MainLnk        Link
	Pattern        string
	Links          []*maybeTitledLink
	MaybeDates     []*date
	PostCategories []HistoricalBlogPostCategory
	Extra          []string
}

func (r *archivesShuffledResult) MainLink() Link {
	return r.MainLnk
}

func (r *archivesShuffledResult) SpeculativeCount() int {
	return len(r.Links)
}

type archivesShuffledResults struct {
	MainLnk        Link
	Results        []*archivesShuffledResult
	SpeculativeCnt int
}

func (r *archivesShuffledResults) mainLink() Link {
	return r.MainLnk
}

func (r *archivesShuffledResults) speculativeCount() int {
	return r.SpeculativeCnt
}

type archivesLongFeedResult struct {
	MainLnk Link
	Pattern string
	Links   []*maybeTitledLink
	Extra   []string
}

func (r *archivesLongFeedResult) mainLink() Link {
	return r.MainLnk
}

func (r *archivesLongFeedResult) speculativeCount() int {
	return len(r.Links)
}

func getArchivesAlmostMatchThreshold(feedLength int) int {
	if feedLength <= 3 {
		return feedLength
	} else if feedLength <= 7 {
		return feedLength - 1
	} else if feedLength <= 25 {
		return feedLength - 2
	} else if feedLength <= 62 {
		return feedLength - 3
	} else {
		return feedLength - 7
	}
}

func tryExtractArchives(
	fetchLink *Link, page *htmlPage, pageLinks []*xpathLink, pageCurisSet *CanonicalUriSet,
	extractionsByStarCount []starCountExtractions, almostMatchThreshold int,
	guidedCtx *guidedCrawlContext, logger Logger,
) []crawlHistoricalResult {
	if guidedCtx.FeedEntryLinks.countIncluded(pageCurisSet) < almostMatchThreshold &&
		!CanonicalUriEqual(fetchLink.Curi, hardcodedDanLuu, guidedCtx.CuriEqCfg) {
		return nil
	}

	if CanonicalUriEqual(fetchLink.Curi, hardcodedCryptographyEngineeringAll, guidedCtx.CuriEqCfg) {
		logger.Info("Skipping archives for Cryptography Engineering to pick up categories from paged")
		return nil
	}

	logger.Info("Possible archives page: %s", page.Curi)

	var mainResult crawlHistoricalResult
	minLinksCount := 1

	if CanonicalUriEqual(fetchLink.Curi, hardcodedDanLuu, guidedCtx.CuriEqCfg) {
		logger.Info("Extracting archives for Dan Luu")
		links := extractionsByStarCount[0].Extractions[0].LinksExtraction.Links
		firstIdx := 0
		for !(links[firstIdx].Curi.Host == "danluu.com" && links[firstIdx].Curi.TrimmedPath != "") {
			firstIdx++
		}
		lastIdx := 0
		for !(links[lastIdx].Curi.Host == "danluu.com" &&
			links[lastIdx].Curi.TrimmedPath == "/why-hardware-development-is-hard") {
			lastIdx++
		}
		postLinks := links[firstIdx : lastIdx+1]
		return []crawlHistoricalResult{
			&archivesSortedResult{
				MainLnk:        *fetchLink,
				Pattern:        "archives_sorted",
				Links:          dropHtml(postLinks),
				HasDates:       false,
				PostCategories: nil,
				Extra:          nil,
			},
		}
	}

	if CanonicalUriEqual(fetchLink.Curi, hardcodedJuliaEvans, guidedCtx.CuriEqCfg) {
		logger.Info("Extracting archives for Julia Evans")
		starCountExtractions := extractionsByStarCount[1] // 2 stars
		if shuffledResult, ok := tryExtractShuffled(
			page, starCountExtractions, -1, minLinksCount, fetchLink, guidedCtx, logger,
		); ok {
			return []crawlHistoricalResult{
				&archivesShuffledResults{
					MainLnk:        *fetchLink,
					Results:        []*archivesShuffledResult{shuffledResult},
					SpeculativeCnt: shuffledResult.SpeculativeCount(),
				},
			}
		} else {
			logger.Error("Couldn't extract archives for Julia Evans")
		}
	}

	var sortedFewerStarsCuris []CanonicalUri
	var sortedFewerStarsHaveDates bool
	for _, starCountExtractions := range extractionsByStarCount {
		if sortedResult, ok := tryExtractSorted(
			starCountExtractions, -1, sortedFewerStarsCuris, sortedFewerStarsHaveDates, 0, fetchLink,
			guidedCtx, logger,
		); ok {
			mainResult = sortedResult
			minLinksCount = len(sortedResult.Links) + 1
			sortedFewerStarsCuris = ToCanonicalUris(sortedResult.Links)
			sortedFewerStarsHaveDates = sortedResult.HasDates
		}
	}

	if guidedCtx.FeedEntryLinks.Length < 3 {
		logger.Info(
			"Skipping sorted match with highlighted first link because the feed is small (%d)",
			guidedCtx.FeedEntryLinks.Length,
		)
	} else {
		var sortedHighlightFewerStarsCuris []CanonicalUri
		for _, starCountExtractions := range extractionsByStarCount {
			if sortedHighlightResult, ok := tryExtractSortedHighlightFirstLink(
				starCountExtractions, pageCurisSet, sortedHighlightFewerStarsCuris, minLinksCount,
				fetchLink, guidedCtx, logger,
			); ok {
				mainResult = sortedHighlightResult
				minLinksCount = len(sortedHighlightResult.Links) + 1
				sortedHighlightFewerStarsCuris = ToCanonicalUris(sortedHighlightResult.Links)
			}
		}
	}

	if mediumPinnedEntryResult, ok := tryExtractMediumPinnedEntry(
		extractionsByStarCount[0].Extractions, pageLinks, minLinksCount, fetchLink, guidedCtx, logger,
	); ok {
		// Medium with pinned entry is a very specific match
		return []crawlHistoricalResult{mediumPinnedEntryResult}
	}

	var sorted2XPathsFewerStarsCuris []CanonicalUri
	oneStarExtractions := extractionsByStarCount[0].Extractions
	for _, starCountExtractions := range extractionsByStarCount {
		if sorted2XPathsResult, ok := tryExtractSorted2XPaths(
			oneStarExtractions, starCountExtractions, sorted2XPathsFewerStarsCuris, minLinksCount,
			fetchLink, guidedCtx, logger,
		); ok {
			mainResult = sorted2XPathsResult
			minLinksCount = len(sorted2XPathsResult.Links) + 1
			sorted2XPathsFewerStarsCuris = ToCanonicalUris(sorted2XPathsResult.Links)
		}
	}

	if guidedCtx.FeedEntryLinks.Length < minLinksCount {
		logger.Info(
			"Skipping almost feed match because min links count is already greater (%d > %d)",
			minLinksCount, guidedCtx.FeedEntryLinks.Length,
		)
	} else {
		var almostFeedFewerStarsCuris []CanonicalUri
		for _, starCountExtractions := range extractionsByStarCount {
			if almostFeedResult, ok := tryExtractAlmostMatchingFeed(
				starCountExtractions, almostMatchThreshold, almostFeedFewerStarsCuris, fetchLink, guidedCtx,
				logger,
			); ok {
				mainResult = almostFeedResult
				minLinksCount = len(almostFeedResult.Links) + 1
				almostFeedFewerStarsCuris = ToCanonicalUris(almostFeedResult.Links)
			}
		}
	}

	var tentativeBetterResults []*archivesShuffledResult

	if sortedResult, ok := mainResult.(*archivesSortedResult); ok && sortedResult.HasDates {
		logger.Info("Skipping shuffled match because there's already a sorted result with dates")
	} else {
		for _, starCountExtractions := range extractionsByStarCount {
			if shuffledResult, ok := tryExtractShuffled(
				page, starCountExtractions, -1, minLinksCount, fetchLink, guidedCtx, logger,
			); ok {
				tentativeBetterResults = append(tentativeBetterResults, shuffledResult)
				minLinksCount = len(shuffledResult.Links) + 1
			}
		}
	}

	// Haven't ported try_extract_shuffled_2xpaths because only one blog needs it

	var sortedAlmostFewerStarsCuris []CanonicalUri
	var sortedAlmostFewerStarsHaveDates bool
	for _, starCountExtractions := range extractionsByStarCount {
		if sortedAlmostResult, ok := tryExtractSorted(
			starCountExtractions, almostMatchThreshold, sortedAlmostFewerStarsCuris,
			sortedAlmostFewerStarsHaveDates, minLinksCount, fetchLink, guidedCtx, logger,
		); ok {
			mainResult = sortedAlmostResult
			minLinksCount = len(sortedAlmostResult.Links) + 1
			sortedAlmostFewerStarsCuris = ToCanonicalUris(sortedAlmostResult.Links)
			sortedAlmostFewerStarsHaveDates = sortedAlmostResult.HasDates
		}
	}

	if sortedResult, ok := mainResult.(*archivesSortedResult); ok && sortedResult.HasDates {
		logger.Info("Skipping shuffled_almost match because there's already a sorted match with dates")
	} else {
		for _, starCountExtractions := range extractionsByStarCount {
			if shuffledAlmostResult, ok := tryExtractShuffled(
				page, starCountExtractions, almostMatchThreshold, minLinksCount, fetchLink, guidedCtx,
				logger,
			); ok {
				tentativeBetterResults = append(tentativeBetterResults, shuffledAlmostResult)
				minLinksCount = len(shuffledAlmostResult.Links) + 1
			}
		}
	}

	if longFeedResult, ok := tryExtractLongFeed(
		guidedCtx.FeedEntryLinks, pageCurisSet, minLinksCount, fetchLink, logger,
	); ok {
		mainResult = longFeedResult
		minLinksCount = len(longFeedResult.Links) + 1 //nolint:ineffassign,staticcheck
	}

	var results []crawlHistoricalResult
	if mainResult != nil {
		results = append(results, mainResult)
	}
	if len(tentativeBetterResults) > 0 {
		speculativeCount := 0
		for _, result := range tentativeBetterResults {
			if len(result.Links) > speculativeCount {
				speculativeCount = len(result.Links)
			}
		}
		results = append(results, &archivesShuffledResults{
			MainLnk:        *fetchLink,
			Results:        tentativeBetterResults,
			SpeculativeCnt: speculativeCount,
		})
	}
	return results
}

func tryExtractSorted(
	starCountExtractions starCountExtractions, almostMatchThreshold int, fewerStarsCuris []CanonicalUri,
	fewerStarsHaveDates bool, minLinksCount int, mainLink *Link, guidedCtx *guidedCrawlContext, logger Logger,
) (*archivesSortedResult, bool) {
	isAlmost := almostMatchThreshold != -1
	almostSuffix := ""
	if isAlmost {
		almostSuffix = "_almost"
	}
	feedEntryLinks := guidedCtx.FeedEntryLinks
	curiEqCfg := guidedCtx.CuriEqCfg
	logger.Info("Trying sorted%s match with %d stars", almostSuffix, starCountExtractions.StarCount)

	var bestXPath string
	var bestLinks []*maybeTitledLink
	var bestHtmlLinks []*maybeTitledHtmlLink
	var bestDistanceToTopParent int
	var bestHasDates bool
	var bestPattern string
	var bestLogStr string
	for _, extraction := range starCountExtractions.Extractions {
		links := extraction.LinksExtraction.Links
		curis := extraction.LinksExtraction.Curis
		curisSet := extraction.LinksExtraction.CurisSet
		hasDuplicates := extraction.LinksExtraction.HasDuplicates
		var markupDatesExtraction *sortedDatesExtraction
		if isAlmost {
			markupDatesExtraction = &extraction.AlmostMarkupDatesExtraction
		} else {
			markupDatesExtraction = &extraction.MarkupDatesExtraction
		}
		maybeDates := markupDatesExtraction.MaybeDates
		datesLogLines := markupDatesExtraction.LogLines

		if len(bestLinks) >= len(links) {
			continue
		}
		if fewerStarsHaveDates && maybeDates == nil {
			continue
		}
		if len(links) < feedEntryLinks.Length {
			continue
		}
		if len(links) < minLinksCount {
			continue
		}

		logLines := slices.Clone(extraction.LogLines)
		logLines = append(logLines, datesLogLines...)

		var targetFeedEntryLinks *FeedEntryLinks
		if isAlmost {
			filteredFeedEntryLinks := feedEntryLinks.filterIncluded(&curisSet)
			if filteredFeedEntryLinks.Length == feedEntryLinks.Length {
				continue
			}
			if filteredFeedEntryLinks.Length < almostMatchThreshold {
				continue
			}
			appendLogLinef(
				&logLines, "almost feed match %d/%d", filteredFeedEntryLinks.Length, feedEntryLinks.Length,
			)
			targetFeedEntryLinks = filteredFeedEntryLinks
		} else {
			if !feedEntryLinks.allIncluded(&curisSet) {
				continue
			}
			targetFeedEntryLinks = feedEntryLinks
		}

		// In 1+*(*(*)) sorted links are deduped to pick the oldest occurrence of each, haven't had a real
		// example in just sorted

		_, isMatchingFeed := targetFeedEntryLinks.sequenceMatch(curis, curiEqCfg)
		isMatchingFewerStarsLinks := true
		if fewerStarsCuris != nil {
			if len(curis) < len(fewerStarsCuris) {
				isMatchingFewerStarsLinks = false
			} else {
				for i := range fewerStarsCuris {
					if !CanonicalUriEqual(curis[i], fewerStarsCuris[i], curiEqCfg) {
						isMatchingFewerStarsLinks = false
						break
					}
				}
			}
		}
		if markupDatesExtraction.AreSorted != sortedStatusNo &&
			isMatchingFeed &&
			!hasDuplicates &&
			isMatchingFewerStarsLinks {

			bestXPath = extraction.MaskedXPath
			bestLinks = dropHtml(links)
			bestHtmlLinks = links
			bestDistanceToTopParent = extraction.DistanceToTopParent
			bestHasDates = maybeDates != nil
			bestPattern = fmt.Sprintf("archives%s", almostSuffix)
			bestLogStr = joinLogLines(logLines)
			logger.Info("Masked xpath is good: %s%s", bestXPath, bestLogStr)
			continue
		}

		reversedLinks := slices.Clone(links)
		slices.Reverse(reversedLinks)
		reversedCuris := slices.Clone(curis)
		slices.Reverse(reversedCuris)
		isReversedMatchingFewerStarsLinksPrefix := true
		isReversedMatchingFewerStarsLinksSuffix := true
		if fewerStarsCuris != nil {
			if len(curis) < len(fewerStarsCuris) {
				isReversedMatchingFewerStarsLinksPrefix = false
				isReversedMatchingFewerStarsLinksSuffix = false
			} else {
				for i := range fewerStarsCuris {
					if !CanonicalUriEqual(reversedCuris[i], fewerStarsCuris[i], curiEqCfg) {
						isReversedMatchingFewerStarsLinksPrefix = false
					}
					reversedCuri := reversedCuris[len(reversedCuris)-len(fewerStarsCuris)+i]
					if !CanonicalUriEqual(reversedCuri, fewerStarsCuris[i], curiEqCfg) {
						isReversedMatchingFewerStarsLinksSuffix = false
					}
					if !isReversedMatchingFewerStarsLinksPrefix && !isReversedMatchingFewerStarsLinksSuffix {
						break
					}
				}
			}
		}

		_, isReversedMatchingFeed := targetFeedEntryLinks.sequenceMatch(reversedCuris, curiEqCfg)
		if markupDatesExtraction.AreReverseSorted != sortedStatusNo &&
			isReversedMatchingFeed &&
			!hasDuplicates &&
			(isReversedMatchingFewerStarsLinksPrefix || isReversedMatchingFewerStarsLinksSuffix) {

			bestXPath = extraction.MaskedXPath
			bestLinks = dropHtml(reversedLinks)
			bestHtmlLinks = reversedLinks
			bestDistanceToTopParent = extraction.DistanceToTopParent
			bestHasDates = maybeDates != nil
			bestPattern = fmt.Sprintf("archives%s", almostSuffix)
			bestLogStr = joinLogLines(logLines)
			logger.Info("Masked xpath is good in reverse order: %s%s", bestXPath, bestLogStr)
			continue
		}

		if maybeDates != nil {
			var uniqueLinksDates []linkDate
			curisSetByDate := make(map[date]*CanonicalUriSet)
			for i, link := range links {
				date := maybeDates[i]
				if _, ok := curisSetByDate[date]; !ok {
					curiSet := NewCanonicalUriSet(nil, curiEqCfg)
					curisSetByDate[date] = &curiSet
				}

				if !curisSetByDate[date].Contains(link.Curi) {
					uniqueLinksDates = append(uniqueLinksDates, linkDate{
						Link: link.maybeTitledLink,
						Date: date,
					})
					curisSetByDate[date].add(link.Curi)
				}
			}

			if len(uniqueLinksDates) != curisSet.Length {
				var sb strings.Builder
				for i, curi := range curis {
					if i > 0 {
						fmt.Fprint(&sb, ", ")
					}
					date := maybeDates[i]
					fmt.Fprintf(&sb, "[%q, %s]", curi, date)
				}
				logger.Info(
					"Masked xpath %s has all links with dates but also duplicates with conflicting dates: %s",
					sb.String(),
				)
				continue
			}

			sortedLinksDates := sortLinksDates(uniqueLinksDates)
			sortedLinks := make([]*maybeTitledLink, len(sortedLinksDates))
			for i := range sortedLinksDates {
				sortedLinks[i] = &sortedLinksDates[i].Link
			}
			sortedCuris := ToCanonicalUris(sortedLinks)
			_, isSortedMatchingFeed := targetFeedEntryLinks.sequenceMatch(sortedCuris, curiEqCfg)
			if !isSortedMatchingFeed {
				logger.Info(
					"Masked xpath %s has all links with dates but doesn't match feed after sorting",
					extraction.MaskedXPath,
				)
				var sb strings.Builder
				for i, linkDate := range sortedLinksDates {
					if i > 0 {
						fmt.Fprint(&sb, ", ")
					}
					fmt.Fprintf(&sb, "[%q, %s]", linkDate.Link.Curi, linkDate.Date)
				}
				logger.Info("Masked xpath links with dates: %s", sb.String())
				logger.Info("Feed links: %s", targetFeedEntryLinks)
				continue
			}

			// Don't compare with fewer stars canonical urls
			// If it's two stars by category, categories are interspersed and have dates, but one category
			// matches feed, dates are still a good signal to merge the categories
			// (jvns)

			bestXPath = extraction.MaskedXPath
			bestLinks = sortedLinks
			bestHtmlLinks = nil
			bestDistanceToTopParent = extraction.DistanceToTopParent
			bestHasDates = true
			bestPattern = fmt.Sprintf("archives_shuffled%s", almostSuffix)

			if len(links) > len(uniqueLinksDates) {
				appendLogLinef(&logLines, "dedup %d -> %d", len(links), len(uniqueLinksDates))
			}
			newestDate := sortedLinksDates[0].Date
			oldestDate := sortedLinksDates[len(sortedLinksDates)-1].Date
			appendLogLinef(&logLines, "from %s to %s", oldestDate, newestDate)
			bestLogStr := joinLogLines(logLines)
			logger.Info("Masked xpath is good sorted by date: %s%s", extraction.MaskedXPath, bestLogStr)
			continue
		}
	}

	if bestLinks != nil {
		extra := []string{fmt.Sprintf("xpath: %s%s", bestXPath, bestLogStr)}

		var postCategories []HistoricalBlogPostCategory
		if CanonicalUriEqual(mainLink.Curi, hardcodedKalzumeus, curiEqCfg) {
			postCategories = hardcodedKalzumeusCategories
		} else if guidedCtx.FeedGenerator == FeedGeneratorSubstack && bestHtmlLinks != nil {
			postCategories = extractSubstackCategories(bestHtmlLinks, bestDistanceToTopParent)
		}
		if len(postCategories) > 0 {
			postCategoriesStr := categoryCountsString(postCategories)
			logger.Info("Categories: %s", postCategoriesStr)
			appendLogLinef(&extra, "categories: %s", postCategoriesStr)
		}

		return &archivesSortedResult{
			MainLnk:        *mainLink,
			Pattern:        bestPattern,
			Links:          bestLinks,
			HasDates:       bestHasDates,
			PostCategories: postCategories,
			Extra:          extra,
		}, true
	} else {
		logger.Info("No sorted match with %d stars", starCountExtractions.StarCount)
		return nil, false
	}
}

func tryExtractSortedHighlightFirstLink(
	starCountExtractions starCountExtractions, pageCurisSet *CanonicalUriSet,
	fewerStarsCuris []CanonicalUri, minLinksCount int, mainLink *Link, guidedCtx *guidedCrawlContext,
	logger Logger,
) (*archivesSortedResult, bool) {
	feedEntryLinks := guidedCtx.FeedEntryLinks
	curiEqCfg := guidedCtx.CuriEqCfg
	logger.Info(
		"Trying sorted match with highlighted first link and %d stars", starCountExtractions.StarCount,
	)

	var bestXPath string
	var bestFirstLink *maybeTitledLink
	var bestLinks []*maybeTitledLink
	var bestLogStr string
	for _, extraction := range starCountExtractions.Extractions {
		links := extraction.LinksExtraction.Links
		curis := extraction.LinksExtraction.Curis
		curisSet := extraction.LinksExtraction.CurisSet
		logLines := slices.Clone(extraction.LogLines)

		var dedupOtherLinks []*maybeTitledHtmlLink
		var dedupLastCuris []CanonicalUri
		if feedEntryLinks.Length == guidedCtx.FeedEntryCurisTitlesMap.Length {
			dedupOtherCurisSet := NewCanonicalUriSet(nil, curiEqCfg)
			for i := len(links) - 1; i >= 0; i-- {
				link := links[i]
				if dedupOtherCurisSet.Contains(link.Curi) {
					continue
				}

				dedupOtherLinks = append(dedupOtherLinks, link)
				dedupOtherCurisSet.add(link.Curi)
			}
			slices.Reverse(dedupOtherLinks)
			for _, link := range dedupOtherLinks {
				dedupLastCuris = append(dedupLastCuris, link.Curi)
			}

			if len(dedupOtherLinks) != len(links) {
				appendLogLinef(&logLines, "dedup %d -> %d", len(links), len(dedupOtherLinks))
			}
		} else {
			dedupOtherLinks = links
			dedupLastCuris = curis
		}

		if len(bestLinks) >= len(dedupOtherLinks) {
			continue
		}
		if len(dedupOtherLinks) < feedEntryLinks.Length-1 {
			continue
		}
		if len(dedupOtherLinks) < minLinksCount-1 {
			continue
		}

		isMatchingFeed, firstLink := feedEntryLinks.sequenceMatchExceptFirst(dedupLastCuris, curiEqCfg)

		isMatchingFewerStarsLinks := true
		if fewerStarsCuris != nil {
			if len(dedupLastCuris) < len(fewerStarsCuris) {
				isMatchingFewerStarsLinks = false
			} else {
				for i, curi := range dedupLastCuris[:len(fewerStarsCuris)-1] {
					fewerStarsCuri := fewerStarsCuris[i+1]
					if !CanonicalUriEqual(curi, fewerStarsCuri, curiEqCfg) {
						isMatchingFewerStarsLinks = false
						break
					}
				}
			}
		}

		if isMatchingFeed &&
			pageCurisSet.Contains(firstLink.Curi) &&
			!curisSet.Contains(firstLink.Curi) &&
			isMatchingFewerStarsLinks {

			bestXPath = extraction.MaskedXPath
			bestFirstLink = firstLink
			bestLinks = dropHtml(dedupOtherLinks)
			bestLogStr = joinLogLines(logLines)
			logger.Info("Masked xpath is good: %s%s", bestXPath, bestLogStr)
			continue
		}
	}

	if bestLinks != nil && bestFirstLink != nil {
		return &archivesSortedResult{
			MainLnk:        *mainLink,
			Pattern:        "archives_2xpaths",
			Links:          append([]*maybeTitledLink{bestFirstLink}, bestLinks...),
			HasDates:       false,
			PostCategories: nil,
			Extra: []string{
				fmt.Sprintf("counts: 1 + %d", len(bestLinks)),
				fmt.Sprintf("suffix_xpath: %s%s", bestXPath, bestLogStr),
			},
		}, true
	} else {
		logger.Info(
			"No sorted match with highlighted first link and %d stars", starCountExtractions.StarCount,
		)
		return nil, false
	}
}

func tryExtractMediumPinnedEntry(
	extractions []maskedXPathExtraction, pageLinks []*xpathLink, minLinksCount int, fetchLink *Link,
	guidedCtx *guidedCrawlContext, logger Logger,
) (*archivesMediumPinnedEntryResult, bool) {
	feedEntryLinks := guidedCtx.FeedEntryLinks
	curiEqCfg := guidedCtx.CuriEqCfg
	logger.Info("Trying Medium match with pinned entry")

	if guidedCtx.FeedGenerator != FeedGeneratorMedium {
		logger.Info("Feed generator is not Medium")
		return nil, false
	}

	for _, extraction := range extractions {
		links := dropHtml(extraction.LinksExtraction.Links)
		curisSet := extraction.LinksExtraction.CurisSet
		mediumMarkupDatesExtraction := extraction.MediumMarkupDatesExtraction

		if len(links) < feedEntryLinks.Length-1 {
			continue
		}
		if len(links) < minLinksCount-1 {
			continue
		}
		if mediumMarkupDatesExtraction.MaybeDates == nil {
			continue
		}

		var feedLinksNotMatching []*FeedEntryLink
		for _, link := range feedEntryLinks.ToSlice() {
			if !curisSet.Contains(link.Curi) {
				feedLinksNotMatching = append(feedLinksNotMatching, link)
			}
		}
		if len(feedLinksNotMatching) != 1 {
			continue
		}

		pinnedEntryLinkIdx := slices.IndexFunc(pageLinks, func(pageLink *xpathLink) bool {
			return CanonicalUriEqual(pageLink.Curi, feedLinksNotMatching[0].Curi, curiEqCfg)
		})
		if pinnedEntryLinkIdx == -1 {
			continue
		}
		pinnedEntryLink := pageLinks[pinnedEntryLinkIdx]

		otherLinksDates := make([]linkDate, len(links))
		for i, link := range links {
			date := mediumMarkupDatesExtraction.MaybeDates[i]
			otherLinksDates = append(otherLinksDates, linkDate{
				Link: *link,
				Date: date,
			})
		}

		logLines := slices.Clone(extraction.LogLines)
		logLines = append(logLines, mediumMarkupDatesExtraction.LogLines...)
		appendLogLinef(&logLines, "1 + %d links", len(otherLinksDates))
		logStr := joinLogLines(logLines)
		logger.Info("Masked XPath is good with Medium pinned entry: %s%s", extraction.MaskedXPath, logStr)
		pinnedTitleValue := getElementTitle(pinnedEntryLink.Element)
		pinnedTitle := NewLinkTitle(pinnedTitleValue, LinkTitleSourceInnerText, nil)
		return &archivesMediumPinnedEntryResult{
			MainLnk: *fetchLink,
			Pattern: "archives_shuffled_2xpaths",
			PinnedEntryLink: maybeTitledLink{
				Link:       pinnedEntryLink.Link,
				MaybeTitle: &pinnedTitle,
			},
			OtherLinksDates: otherLinksDates,
			Extra: []string{
				fmt.Sprintf("counts: 1 + %d", len(otherLinksDates)),
				fmt.Sprintf("pinned_link_xpath: %s", pinnedEntryLink.XPath),
				fmt.Sprintf("suffix_xpath: %s%s", extraction.MaskedXPath, logStr),
			},
		}, true
	}

	logger.Info("No Medium match with pinned entry")
	return nil, false
}

func tryExtractSorted2XPaths(
	oneStarExtractions []maskedXPathExtraction, starCountExtractions starCountExtractions,
	fewerStarsCuris []CanonicalUri, minLinksCount int, mainLink *Link, guidedCtx *guidedCrawlContext,
	logger Logger,
) (*archivesSortedResult, bool) {
	feedEntryLinks := guidedCtx.FeedEntryLinks
	curiEqCfg := guidedCtx.CuriEqCfg
	logger.Info("Trying sorted match with 1+%d stars", starCountExtractions.StarCount)

	prefixExtractionsByLength := make(map[int]maskedXPathExtraction)
	for _, prefixExtraction := range oneStarExtractions {
		links := prefixExtraction.LinksExtraction.Links
		curis := prefixExtraction.LinksExtraction.Curis
		if len(links) >= feedEntryLinks.Length {
			continue
		}
		if _, ok := feedEntryLinks.sequenceMatch(curis, curiEqCfg); !ok {
			continue
		}

		if _, ok := prefixExtractionsByLength[len(links)]; !ok {
			prefixExtractionsByLength[len(links)] = prefixExtraction
		}
	}

	var bestLinks []*maybeTitledLink
	var bestPrefixXPath string
	var bestSuffixXPath string
	var bestPrefixCount int
	var bestSuffixCount int
	var bestPrefixLogStr string
	var bestSuffixLogStr string

	for _, suffixExtraction := range starCountExtractions.Extractions {
		suffixLinks := suffixExtraction.LinksExtraction.Links
		suffixCuris := suffixExtraction.LinksExtraction.Curis

		// In 1+*(*(*)) sorted links are deduped to pick the oldest occurrence of each, haven't had a real
		// example here

		suffixMatchingLinks, targetPrefixLength := feedEntryLinks.sequenceSuffixMatch(suffixCuris, curiEqCfg)
		if suffixMatchingLinks == nil {
			continue
		}
		if targetPrefixLength+len(suffixLinks) < feedEntryLinks.Length {
			continue
		}
		prefixExtraction, prefixOk := prefixExtractionsByLength[targetPrefixLength]
		if !prefixOk {
			continue
		}
		totalLength := targetPrefixLength + len(suffixLinks)
		if totalLength < minLinksCount {
			continue
		}
		if len(bestLinks) >= totalLength {
			continue
		}

		prefixLinks := prefixExtraction.LinksExtraction.Links

		// Ensure the first suffix link appears on the page after the last prefix link
		// Find the lowest common parent and see if prefix parent comes before suffix parent
		lastPrefixLink := prefixLinks[len(prefixLinks)-1]
		firstSuffixLink := suffixLinks[0]
		// Link can't be a parent of another link. Not actually expecting that but just in case
		if lastPrefixLink.Element == firstSuffixLink.Element.Parent ||
			firstSuffixLink.Element == lastPrefixLink.Element.Parent {
			continue
		}
		prefixAncestorByParent := make(map[*html.Node]*html.Node)
		for element := lastPrefixLink.Element; element != nil; element = element.Parent {
			prefixAncestorByParent[element.Parent] = element
		}
		topSuffixElement := firstSuffixLink.Element
		for topSuffixElement != nil && prefixAncestorByParent[topSuffixElement.Parent] == nil {
			topSuffixElement = topSuffixElement.Parent
		}
		commonParent := topSuffixElement.Parent
		topPrefixElement := prefixAncestorByParent[topSuffixElement.Parent]
		isLastPrefixBeforeLastSuffix := false
		for child := commonParent.FirstChild; child != nil; child = child.NextSibling {
			if child == topPrefixElement {
				isLastPrefixBeforeLastSuffix = true
				break
			}
			if child == topSuffixElement {
				isLastPrefixBeforeLastSuffix = false
				break
			}
		}
		if !isLastPrefixBeforeLastSuffix {
			continue
		}

		logger.Info("Found partition with two xpaths: %d + %d", targetPrefixLength, len(suffixLinks))
		prefixLogStr := joinLogLines(prefixExtraction.LogLines)
		suffixLogStr := joinLogLines(suffixExtraction.LogLines)
		logger.Info("Prefix XPath: %s%s", prefixExtraction.MaskedXPath, prefixLogStr)
		logger.Info("Suffix XPath: %s%s", suffixExtraction.MaskedXPath, suffixLogStr)

		combinedLinks := append(slices.Clone(prefixLinks), suffixLinks...)
		combinedCuris := ToCanonicalUris(combinedLinks)
		combinedCurisSet := NewCanonicalUriSet(combinedCuris, curiEqCfg)
		if len(combinedCuris) != combinedCurisSet.Length {
			logger.Info("Combination has all feed links but also duplicates: %s", combinedCuris)
			continue
		}

		isMatchingFewerStarsLinks := true
		if fewerStarsCuris != nil {
			if len(combinedCuris) < len(fewerStarsCuris) {
				isMatchingFewerStarsLinks = false
			} else {
				for i, curi := range combinedCuris[:len(fewerStarsCuris)] {
					fewerStarsCuri := fewerStarsCuris[i]
					if !CanonicalUriEqual(curi, fewerStarsCuri, curiEqCfg) {
						isMatchingFewerStarsLinks = false
						break
					}
				}
			}
		}

		if !isMatchingFewerStarsLinks {
			logger.Info("Combination doesn't match fewer stars links")
			continue
		}

		bestLinks = dropHtml(combinedLinks)
		bestPrefixXPath = prefixExtraction.MaskedXPath
		bestSuffixXPath = suffixExtraction.MaskedXPath
		bestPrefixCount = len(prefixLinks)
		bestSuffixCount = len(suffixLinks)
		bestPrefixLogStr = prefixLogStr
		bestSuffixLogStr = suffixLogStr
		logger.Info("Combination is good (%d links)", len(combinedLinks))
	}

	if bestLinks != nil {
		return &archivesSortedResult{
			MainLnk:        *mainLink,
			Pattern:        "archives_2xpaths",
			Links:          bestLinks,
			HasDates:       false,
			PostCategories: nil,
			Extra: []string{
				fmt.Sprintf("star_count: 1 + %d", starCountExtractions.StarCount),
				fmt.Sprintf("counts: %d + %d", bestPrefixCount, bestSuffixCount),
				fmt.Sprintf("prefix_xpath: %s%s", bestPrefixXPath, bestPrefixLogStr),
				fmt.Sprintf("suffix_xpath: %s%s", bestSuffixXPath, bestSuffixLogStr),
			},
		}, true
	}

	logger.Info("No sorted match with 1+%d stars", starCountExtractions.StarCount)
	return nil, false
}

func tryExtractAlmostMatchingFeed(
	starCountExtractions starCountExtractions, almostMatchThreshold int, fewerStarsCuris []CanonicalUri,
	mainLink *Link, guidedCtx *guidedCrawlContext, logger Logger,
) (*archivesSortedResult, bool) {
	feedEntryLinks := guidedCtx.FeedEntryLinks
	logger.Info("Trying almost feed match with %d stars", starCountExtractions.StarCount)

	var bestLinks []*maybeTitledLink
	var bestXPath string
	var bestLogStr string

	for _, extraction := range starCountExtractions.Extractions {
		links := extraction.LinksExtraction.Links
		curis := extraction.LinksExtraction.Curis
		curisSet := extraction.LinksExtraction.CurisSet

		if len(bestLinks) >= len(links) {
			continue
		}
		if len(links) >= feedEntryLinks.Length {
			continue
		}
		if len(links) < almostMatchThreshold {
			continue
		}
		if feedEntryLinks.countIncluded(&curisSet) < almostMatchThreshold {
			continue
		}
		feedContainsAll := true
		for _, curi := range curis {
			if !guidedCtx.FeedEntryCurisTitlesMap.Contains(curi) {
				feedContainsAll = false
				break
			}
		}
		if !feedContainsAll {
			continue
		}

		isMatchingFewerStarsLinks := true
		if fewerStarsCuris != nil {
			if len(curis) < len(fewerStarsCuris) {
				isMatchingFewerStarsLinks = false
			} else {
				for i, curi := range curis[:len(fewerStarsCuris)] {
					fewerStarsCuri := fewerStarsCuris[i]
					if !CanonicalUriEqual(curi, fewerStarsCuri, guidedCtx.CuriEqCfg) {
						isMatchingFewerStarsLinks = false
					}
				}
			}
		}
		if !isMatchingFewerStarsLinks {
			continue
		}

		bestLinks = dropHtml(links)
		bestXPath = extraction.MaskedXPath
		logLines := slices.Clone(extraction.LogLines)
		appendLogLinef(&logLines, "%d/%d feed links", len(links), feedEntryLinks.Length)
		bestLogStr = joinLogLines(logLines)
		logger.Info("Masked XPath almost matches feed: %s%s", bestXPath, bestLogStr)
	}

	if bestLinks != nil {
		return &archivesSortedResult{
			MainLnk:        *mainLink,
			Pattern:        "archives_feed_almost",
			Links:          feedEntryLinks.ToMaybeTitledSlice(),
			HasDates:       false,
			PostCategories: nil,
			Extra: []string{
				fmt.Sprintf("xpath: %s%s", bestXPath, bestLogStr),
			},
		}, true
	}

	logger.Info("No almost feed match with %d stars", starCountExtractions.StarCount)
	return nil, false
}

func tryExtractShuffled(
	page *htmlPage, starCountExtractions starCountExtractions, almostMatchThreshold int,
	minLinksCount int, mainLink *Link, guidedCtx *guidedCrawlContext, logger Logger,
) (*archivesShuffledResult, bool) {
	isAlmost := almostMatchThreshold != -1
	almostSuffix := ""
	if isAlmost {
		almostSuffix = "_almost"
	}
	feedEntryLinks := guidedCtx.FeedEntryLinks
	curiEqCfg := guidedCtx.CuriEqCfg
	logger.Info("Trying shuffled%s match with %d stars", almostSuffix, starCountExtractions.StarCount)

	var bestLinks []*maybeTitledLink
	var bestMaybeDates []*date
	var bestXPath string
	var bestLogStr string

	for _, extraction := range starCountExtractions.Extractions {
		links := extraction.LinksExtraction.Links
		curis := extraction.LinksExtraction.Curis
		curisSet := extraction.LinksExtraction.CurisSet
		maybeUrlDatesExtraction := extraction.MaybeUrlDatesExtraction
		someMarkupDatesExtraction := extraction.SomeMarkupDatesExtraction

		if len(bestLinks) >= len(links) {
			continue
		}
		if len(links) < feedEntryLinks.Length {
			continue
		}
		if len(links) < minLinksCount {
			continue
		}

		logLines := slices.Clone(extraction.LogLines)
		logLines = append(logLines, maybeUrlDatesExtraction.LogLines...)
		logLines = append(logLines, someMarkupDatesExtraction.LogLines...)
		if isAlmost {
			targetFeedEntryLinks := feedEntryLinks.filterIncluded(&curisSet)
			if targetFeedEntryLinks.Length == feedEntryLinks.Length {
				continue
			}
			if targetFeedEntryLinks.Length < almostMatchThreshold {
				continue
			}
			appendLogLinef(
				&logLines, "almost feed match %d/%d", targetFeedEntryLinks.Length, feedEntryLinks.Length,
			)
		} else {
			if !feedEntryLinks.allIncluded(&curisSet) {
				continue
			}
		}

		maybeDates := slices.Clone(maybeUrlDatesExtraction.MaybeDates)
		if someMarkupDatesExtraction.MaybeDates != nil {
			for i := range maybeDates {
				if maybeDates[i] == nil {
					maybeDates[i] = &someMarkupDatesExtraction.MaybeDates[i]
				}
			}
		}

		var dedupLinks []*maybeTitledHtmlLink
		var dedupMaybeDates []*date
		if len(curis) != curisSet.Length {
			dedupCurisSet := NewCanonicalUriSet(nil, curiEqCfg)
			for i, link := range links {
				if dedupCurisSet.Contains(link.Curi) {
					continue
				}

				dedupLinks = append(dedupLinks, link)
				dedupMaybeDates = append(dedupMaybeDates, maybeDates[i])
				dedupCurisSet.add(link.Curi)
			}
		} else {
			dedupLinks = links
			dedupMaybeDates = maybeDates
		}

		bestLinks = dropHtml(dedupLinks)
		bestMaybeDates = dedupMaybeDates
		bestXPath = extraction.MaskedXPath
		if len(links) > len(dedupLinks) {
			appendLogLinef(&logLines, "dedup %d -> %d", len(links), len(dedupLinks))
		}
		bestLogStr = joinLogLines(logLines)
		logger.Info("Masked XPath is good but shuffled: %s%s", bestXPath, bestLogStr)
	}

	if bestLinks != nil {
		datesPresent := 0
		for _, maybeDate := range bestMaybeDates {
			if maybeDate != nil {
				datesPresent++
			}
		}

		extra := []string{
			fmt.Sprintf("xpath: %s%s", bestXPath, bestLogStr),
			fmt.Sprintf("dates_present: %d/%d", datesPresent, len(bestLinks)),
		}

		var postCategories []HistoricalBlogPostCategory
		if CanonicalUriEqual(mainLink.Curi, hardcodedJuliaEvans, curiEqCfg) {
			jvnsCategories, err := extractJuliaEvansCategories(page, logger)
			if err != nil {
				logger.Info("Couldn't extract Julia Evans categories: %v", err)
				guidedCtx.HardcodedError = err
			} else {
				postCategories = jvnsCategories
				postCategoriesStr := categoryCountsString(postCategories)
				logger.Info("Categories: %s", postCategoriesStr)
				appendLogLinef(&extra, "categories: %s", postCategoriesStr)
			}
		}

		return &archivesShuffledResult{
			MainLnk:        *mainLink,
			Pattern:        fmt.Sprintf("archives_shuffled%s", almostSuffix),
			Links:          bestLinks,
			MaybeDates:     bestMaybeDates,
			PostCategories: postCategories,
			Extra:          extra,
		}, true
	}

	logger.Info("No shuffled match with %d stars", starCountExtractions.StarCount)
	return nil, false
}

func tryExtractLongFeed(
	feedEntryLinks *FeedEntryLinks, pageCurisSet *CanonicalUriSet, minLinksCount int, mainLink *Link,
	logger Logger,
) (*archivesLongFeedResult, bool) {
	logger.Info("Trying archives long feed match")

	if feedEntryLinks.Length >= 31 &&
		feedEntryLinks.Length >= minLinksCount &&
		feedEntryLinks.allIncluded(pageCurisSet) {

		logger.Info("Long feed is matching (%d links)", feedEntryLinks.Length)
		return &archivesLongFeedResult{
			MainLnk: *mainLink,
			Pattern: "archives_long_feed",
			Links:   feedEntryLinks.ToMaybeTitledSlice(),
			Extra:   nil,
		}, true
	} else {
		logger.Info("No archives long feed match")
		return nil, false
	}
}
