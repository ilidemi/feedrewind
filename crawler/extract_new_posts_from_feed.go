package crawler

import (
	"errors"
	"feedrewind/oops"
	neturl "net/url"
)

var ErrExtractNewPostsNoMatch = errors.New("extract new posts no match")

func ExtractNewPostsFromFeed(
	parsedFeed *ParsedFeed, feedUri *neturl.URL, existingPostCuris []CanonicalUri,
	discardedFeedEntryUrls map[string]bool, missingFromFeedEntryUrls map[string]bool,
	curiEqCfg *CanonicalEqualityConfig, logger Logger,
	parseFeedLogger Logger,
) (
	newLinks []*FeedEntryLink, err error,
) {
	missingFromFeedEntryCurisSet := NewCanonicalUriSet(nil, curiEqCfg)
	for url := range missingFromFeedEntryUrls {
		canonicalLink, ok := ToCanonicalLink(url, logger, nil)
		if !ok {
			return nil, oops.Newf("couldn't parse link: %s", url)
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
			return nil, oops.Newf("couldn't parse link: %s", url)
		}
		discardedFeedEntryCurisSet.add(canonicalLink.Curi)
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
		return nil, nil
	}

	overlappingPostsCount := feedEntryLinks.Length - newPostsCount
	if overlappingPostsCount < 3 {
		logger.Info("Can't update from feed because the overlap with existing posts isn't long enough")
		return nil, ErrExtractNewPostsNoMatch
	}

	suffixLinks, _ := feedEntryLinks.sequenceSuffixMatch(expectedExistingPostCuris, curiEqCfg)
	if suffixLinks == nil {
		logger.Info("Can't update from feed because the existing posts don't match")
		return nil, ErrExtractNewPostsNoMatch
	}
	suffixLinksSet := NewCanonicalUriSet(nil, curiEqCfg)
	for _, suffixLink := range suffixLinks {
		suffixLinksSet.add(suffixLink.Curi)
	}

	// Protects from feed out of order as without it feed [6] [1] [2] [3] [4] [5] would still find suffix in
	// existing posts [2 3 4 5 6]
	if len(suffixLinks) != overlappingPostsCount {
		logger.Info("Can't update from feed because the existing posts match but the count is wrong")
		return nil, ErrExtractNewPostsNoMatch
	}

	for _, feedLink := range feedEntryLinksSlice {
		if !suffixLinksSet.Contains(feedLink.Curi) {
			newLinks = append(newLinks, feedLink)
		}
	}
	return newLinks, nil
}
