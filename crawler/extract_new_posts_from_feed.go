package crawler

import (
	"fmt"
	neturl "net/url"
)

func MustExtractNewPostsFromFeed(
	feedContent string, feedUri *neturl.URL, existingPostCuris []CanonicalUri,
	discardedFeedEntryUrls map[string]bool, missingFromFeedEntryUrls map[string]bool,
	curiEqCfg CanonicalEqualityConfig, logger Logger,
	parseFeedLogger Logger,
) (
	newLinks []MaybeTitledLink, ok bool,
) {
	missingFromFeedEntryCurisSet := NewCanonicalUriSet(nil, curiEqCfg)
	for url := range missingFromFeedEntryUrls {
		canonicalLink, ok := ToCanonicalLink(url, logger, nil)
		if !ok {
			panic(fmt.Errorf("couldn't parse link: %s", url))
		}
		missingFromFeedEntryCurisSet.add(canonicalLink.Curi)
	}

	var expectedExistingPostCuris []CanonicalUri
	expectedExistingPostCurisSet := NewCanonicalUriSet(nil, curiEqCfg)
	for _, curi := range existingPostCuris {
		if !missingFromFeedEntryCurisSet.Contains(curi) {
			expectedExistingPostCuris = append(expectedExistingPostCuris, curi)
			expectedExistingPostCurisSet.add(curi)
		}
	}

	discardedFeedEntryCurisSet := NewCanonicalUriSet(nil, curiEqCfg)
	for url := range discardedFeedEntryUrls {
		canonicalLink, ok := ToCanonicalLink(url, logger, nil)
		if !ok {
			panic(fmt.Errorf("couldn't parse link: %s", url))
		}
		discardedFeedEntryCurisSet.add(canonicalLink.Curi)
	}

	parsedFeed, err := ParseFeed(feedContent, feedUri, parseFeedLogger)
	if err != nil {
		panic(err)
	}
	feedEntryLinks := parsedFeed.EntryLinks.Except(discardedFeedEntryCurisSet)
	feedEntryLinksSlice := feedEntryLinks.ToSlice()
	newPostsCount := 0
	for _, feedLink := range feedEntryLinksSlice {
		if !expectedExistingPostCurisSet.Contains(feedLink.Curi) {
			newPostsCount++
		}
	}
	if newPostsCount == 0 {
		return nil, true
	}

	overlappingPostsCount := feedEntryLinks.Length - newPostsCount
	if overlappingPostsCount < 3 {
		logger.Info("Can't update from feed because the overlap with existing posts isn't long enough")
		return nil, false
	}

	suffixLength := feedEntryLinks.sequenceSuffixLength(expectedExistingPostCuris, curiEqCfg)
	if suffixLength == 0 {
		logger.Info("Can't update from feed because the existing posts don't match")
		return nil, false
	}

	// Protects from feed out of order as without it feed [6] [1] [2] [3] [4] [5] would still find suffix in
	// existing posts [2 3 4 5 6]
	if suffixLength != overlappingPostsCount {
		logger.Info("Can't update from feed because the existing posts match but the count is wrong")
		return nil, false
	}

	return feedEntryLinksSlice[:newPostsCount], true
}
