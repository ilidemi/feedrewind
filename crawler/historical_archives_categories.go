package crawler

import (
	"fmt"
	"slices"
	"strings"

	om "github.com/wk8/go-ordered-map/v2"
)

type archivesCategoriesState struct {
	MainLink                           *Link
	CategoryResultsByFeedBitmapByLevel *om.OrderedMap[int, *om.OrderedMap[string, archivesCategoryResult]]
}

func newArchivesCategoriesState(mainLink *Link) archivesCategoriesState {
	return archivesCategoriesState{
		MainLink:                           mainLink,
		CategoryResultsByFeedBitmapByLevel: om.New[int, *om.OrderedMap[string, archivesCategoryResult]](),
	}
}

type archivesCategoryResult struct {
	Level           int
	FeedMatchBitmap string
	MaskedXPath     string
	Links           []*pristineMaybeTitledLink
	MaybeDates      []*date
	Curi            pristineCanonicalUri
	FetchUri        *pristineUri
	LogStr          string
}

type ArchivesCategoriesResult struct {
	MainLnk    pristineLink
	Pattern    string
	Links      []*pristineMaybeTitledLink
	MaybeDates []*date
	Extra      []string
}

func (r *ArchivesCategoriesResult) mainLink() pristineLink {
	return r.MainLnk
}

func (r *ArchivesCategoriesResult) speculativeCount() int {
	return len(r.Links)
}

func (r *ArchivesCategoriesResult) isSame(
	other crawlHistoricalResult, curiEqCfg *CanonicalEqualityConfig,
) bool {
	aOther, ok := other.(*ArchivesCategoriesResult)
	if !ok {
		return false
	}
	if !areLinksEqual(r.Links, aOther.Links, curiEqCfg) {
		return false
	}
	for i, maybeDate := range r.MaybeDates {
		maybeOtherDate := aOther.MaybeDates[i]
		if (maybeDate == nil) != (maybeOtherDate == nil) {
			return false
		}
		if maybeDate != nil && maybeOtherDate != nil && maybeDate.Compare(*maybeOtherDate) != 0 {
			return false
		}
	}
	return true
}

func tryExtractArchivesCategories(
	page *htmlPage, pageCurisSet *CanonicalUriSet, extractionsByStarCount []starCountExtractions,
	guidedCtx *guidedCrawlContext, logger Logger,
) (crawlHistoricalResult, bool) {
	feedEntryLinks := guidedCtx.FeedEntryLinks
	curiEqCfg := guidedCtx.CuriEqCfg
	state := guidedCtx.ArchivesCategoriesState
	if feedEntryLinks.countIncluded(pageCurisSet) < 2 {
		return nil, false
	}

	// Assuming all links in the feed are unique for simpler merging of categories
	if feedEntryLinks.Length != guidedCtx.FeedEntryCurisTitlesMap.Length {
		return nil, false
	}

	var bestLinks []*maybeTitledLink
	var bestMaybeDates []*date
	var bestFeedMatchingCurisSet CanonicalUriSet
	var bestXPath string
	var bestLogStr string

	for _, starCountExtractions := range extractionsByStarCount {
		logger.Info("Trying category match with %d stars", starCountExtractions.StarCount)

		for _, extraction := range starCountExtractions.Extractions {
			maskedXPath := extraction.MaskedXPath
			links := extraction.LinksExtraction.Links
			curis := extraction.LinksExtraction.Curis
			curisSet := extraction.LinksExtraction.CurisSet
			maybeUrlDates := extraction.MaybeUrlDatesExtraction.MaybeDates
			someMarkupDates := extraction.SomeMarkupDatesExtraction.MaybeDates

			if len(bestLinks) >= len(links) {
				continue
			}
			if len(links) < 2 {
				continue
			}

			feedMatchingLinks := feedEntryLinks.filterIncluded(&curisSet).ToSlice()
			feedMatchingCuris := ToCanonicalUris(feedMatchingLinks)
			feedMatchingCurisSet := NewCanonicalUriSet(feedMatchingCuris, curiEqCfg)
			if feedMatchingCurisSet.Length < 2 {
				continue
			}
			if feedMatchingCurisSet.Length > feedEntryLinks.Length-2 {
				// Too close to just archives
				continue
			}

			maybeDates := slices.Clone(maybeUrlDates)
			if someMarkupDates != nil {
				for i := range maybeDates {
					if maybeDates[i] == nil {
						maybeDates[i] = &someMarkupDates[i]
					}
				}
			}
			logLines := slices.Clone(extraction.LogLines)
			logLines = append(logLines, extraction.MaybeUrlDatesExtraction.LogLines...)
			logLines = append(logLines, extraction.SomeMarkupDatesExtraction.LogLines...)
			var dedupLinks []*maybeTitledHtmlLink
			var dedupMaybeDates []*date
			if len(curis) != curisSet.Length {
				dedupCurisSet := NewCanonicalUriSet(nil, curiEqCfg)
				for i, link := range links {
					if dedupCurisSet.Contains(link.Curi) {
						continue
					}

					dedupCurisSet.add(link.Curi)
					dedupLinks = append(dedupLinks, link)
					dedupMaybeDates = append(dedupMaybeDates, maybeDates[i])
				}
				appendLogLinef(&logLines, "dedup %d -> %d", len(links), len(dedupLinks))
			} else {
				dedupLinks = links
				dedupMaybeDates = maybeDates
			}

			bestLinks = dropHtml(dedupLinks)
			bestMaybeDates = dedupMaybeDates
			bestFeedMatchingCurisSet = feedMatchingCurisSet
			bestXPath = maskedXPath
			bestLogStr = joinLogLines(logLines)
			logger.Info("Masked XPath looks like a category: %s%s", maskedXPath, bestLogStr)
		}
	}

	if bestLinks == nil {
		logger.Info("No archives categories match")
		return nil, false
	}

	level := strings.Count(page.Curi.TrimmedPath, "/")
	var sb strings.Builder
	sb.Grow(feedEntryLinks.Length)
	for _, link := range feedEntryLinks.ToSlice() {
		if bestFeedMatchingCurisSet.Contains(link.Curi) {
			sb.WriteByte('1')
		} else {
			sb.WriteByte('0')
		}
	}
	feedMatchBitmap := sb.String()

	stateMap := state.CategoryResultsByFeedBitmapByLevel
	if _, ok := stateMap.Get(level); !ok {
		stateMap.Set(level, om.New[string, archivesCategoryResult]())
	}
	categoryResultsByFeedBitmap, _ := stateMap.Get(level)

	if r, ok := categoryResultsByFeedBitmap.Get(feedMatchBitmap); !ok || len(r.Links) < len(bestLinks) {
		categoryResultsByFeedBitmap.Set(feedMatchBitmap, archivesCategoryResult{
			Level:           level,
			FeedMatchBitmap: feedMatchBitmap,
			MaskedXPath:     bestXPath,
			Links:           NewPristineMaybeTitledLinks(bestLinks),
			MaybeDates:      bestMaybeDates,
			Curi:            NewPristineCanonicalUri(page.Curi),
			FetchUri:        NewPristineUri(page.FetchUri),
			LogStr:          bestLogStr,
		})
	}

	almostMatchThreshold := getArchivesCategoriesAlmostMatchThreshold(feedEntryLinks.Length)

	combinationsChecked := 0
	for levelPair := stateMap.Oldest(); levelPair != nil; levelPair = levelPair.Next() {
		categoryResultsByFeedBitmap := levelPair.Value

		for pair1 := categoryResultsByFeedBitmap.Oldest(); pair1 != nil; pair1 = pair1.Next() {
			for pair2 := categoryResultsByFeedBitmap.Oldest(); pair2 != pair1; pair2 = pair2.Next() {
				combinationsChecked++
				categories := []archivesCategoryResult{pair1.Value, pair2.Value}
				if result, ok := checkCombination(
					categories, feedEntryLinks, curiEqCfg, almostMatchThreshold, combinationsChecked,
					state.MainLink, logger,
				); ok {
					return result, true
				}
			}
		}
	}

	for levelPair := stateMap.Oldest(); levelPair != nil; levelPair = levelPair.Next() {
		categoryResultsByFeedBitmap := levelPair.Value

		for pair1 := categoryResultsByFeedBitmap.Oldest(); pair1 != nil; pair1 = pair1.Next() {
			for pair2 := categoryResultsByFeedBitmap.Oldest(); pair2 != pair1; pair2 = pair2.Next() {
				for pair3 := categoryResultsByFeedBitmap.Oldest(); pair3 != pair2; pair3 = pair3.Next() {
					combinationsChecked++
					categories := []archivesCategoryResult{pair1.Value, pair2.Value, pair3.Value}
					if result, ok := checkCombination(
						categories, feedEntryLinks, curiEqCfg, almostMatchThreshold, combinationsChecked,
						state.MainLink, logger,
					); ok {
						return result, true
					}
				}
			}
		}
	}

	logger.Info("No archives categories match. Combinations checked: %d", combinationsChecked)
	return nil, false
}

func getArchivesCategoriesAlmostMatchThreshold(feedLength int) int {
	switch {
	case feedLength <= 9:
		return feedLength
	case feedLength <= 19:
		return feedLength - 1
	default:
		return feedLength - 2
	}
}

func checkCombination(
	categories []archivesCategoryResult, feedEntryLinks *FeedEntryLinks, curiEqCfg *CanonicalEqualityConfig,
	almostMatchThreshold int, combinationsChecked int, mainLink *Link, logger Logger,
) (crawlHistoricalResult, bool) {
	feedOverlap := 0
	for i := range feedEntryLinks.Length {
		for _, category := range categories {
			if category.FeedMatchBitmap[i] == '1' {
				feedOverlap++
				break
			}
		}
	}
	if feedOverlap < almostMatchThreshold {
		return nil, false
	}

	var missingLinks []*maybeTitledLink
	var almostSuffix string
	if feedOverlap < feedEntryLinks.Length {
		for i, link := range feedEntryLinks.ToSlice() {
			found := false
			for _, category := range categories {
				if category.FeedMatchBitmap[i] == '1' {
					found = true
					break
				}
			}
			if !found {
				missingLinks = append(missingLinks, &link.maybeTitledLink)
			}
		}
		almostSuffix = "_almost"
	}

	sumLength := 0
	for _, category := range categories {
		sumLength += len(category.Links)
	}
	sumLength += len(missingLinks)
	mergedLinks := make([]*maybeTitledLink, 0, sumLength)
	mergedMaybeDates := make([]*date, 0, sumLength)
	for _, category := range categories {
		for _, link := range category.Links {
			mergedLinks = append(mergedLinks, link.Unwrap())
		}
		mergedMaybeDates = append(mergedMaybeDates, category.MaybeDates...)
	}
	mergedLinks = append(mergedLinks, missingLinks...)
	mergedMaybeDates = mergedMaybeDates[:sumLength]

	dedupLinks := make([]*maybeTitledLink, 0, sumLength)
	dedupMaybeDates := make([]*date, 0, sumLength)
	curisSet := NewCanonicalUriSet(nil, curiEqCfg)
	for i, link := range mergedLinks {
		if curisSet.Contains(link.Curi) {
			continue
		}

		curisSet.add(link.Curi)
		dedupLinks = append(dedupLinks, link)
		dedupMaybeDates = append(dedupMaybeDates, mergedMaybeDates[i])
	}

	var totalLogLines []string
	if len(mergedLinks) != len(dedupLinks) {
		appendLogLinef(&totalLogLines, "dedup %d -> %d", len(mergedLinks), len(dedupLinks))
	}
	appendLogLinef(&totalLogLines, "%d links total", len(dedupLinks))
	totalLogStr := joinLogLines(totalLogLines)
	for i, category := range categories {
		logger.Info(
			"Category %d: %d links, url %s, masked xpath %s%s",
			i+1, len(category.Links), category.Curi, category.MaskedXPath, category.LogStr,
		)
	}
	logger.Info("Missing links: %d", len(missingLinks))
	logger.Info("Combinations checked: %d", combinationsChecked)

	var extra []string
	for i, category := range categories {
		appendLogLinef(&extra, `cat%d_count: %d`, i+1, len(category.Links))
		appendLogLinef(&extra, `cat%d_url: <a href="%s">%s</a>`, i+1, category.FetchUri, category.Curi)
		appendLogLinef(&extra, `cat%d_xpath: %s%s`, i+1, category.MaskedXPath, category.LogStr)
	}
	appendLogLinef(&extra, "missing_count: %d", len(missingLinks))
	appendLogLinef(&extra, "total: %s", totalLogStr)

	return &ArchivesCategoriesResult{
		MainLnk:    *NewPristineLink(mainLink),
		Pattern:    fmt.Sprintf("archives_categories%s", almostSuffix),
		Links:      NewPristineMaybeTitledLinks(dedupLinks),
		MaybeDates: dedupMaybeDates,
		Extra:      extra,
	}, true
}
