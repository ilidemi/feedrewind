package crawler

import (
	"strings"
	"time"

	"golang.org/x/exp/slices"
)

type FeedEntryLinks struct {
	LinkBuckets    [][]maybeTitledLink
	Length         int
	IsOrderCertain bool
}

func feedEntryLinksFromLinksDates(links []maybeTitledLink, dates []time.Time) FeedEntryLinks {
	var isOrderCertain bool
	var linkBuckets [][]maybeTitledLink
	if len(dates) > 0 {
		isOrderCertain = true
		var lastDate time.Time
		for i := 0; i < len(links); i++ {
			link := links[i]
			date := dates[i]
			if i != 0 && date.Equal(lastDate) {
				lastBucket := &linkBuckets[len(linkBuckets)-1]
				*lastBucket = append(*lastBucket, link)
			} else {
				linkBuckets = append(linkBuckets, []maybeTitledLink{link})
			}
			lastDate = date
		}
	} else {
		isOrderCertain = false
		for _, link := range links {
			linkBuckets = append(linkBuckets, []maybeTitledLink{link})
		}
	}

	return FeedEntryLinks{
		LinkBuckets:    linkBuckets,
		Length:         len(links),
		IsOrderCertain: isOrderCertain,
	}
}

func (l *FeedEntryLinks) filterIncluded(curisSet *CanonicalUriSet) *FeedEntryLinks {
	var newBuckets [][]maybeTitledLink
	newLength := 0
	for _, bucket := range l.LinkBuckets {
		var newBucket []maybeTitledLink
		for _, link := range bucket {
			if curisSet.Contains(link.Curi) {
				newBucket = append(newBucket, link)
				newLength++
			}
		}
		if len(newBucket) > 0 {
			newBuckets = append(newBuckets, newBucket)
		}
	}
	return &FeedEntryLinks{
		LinkBuckets:    newBuckets,
		Length:         newLength,
		IsOrderCertain: l.IsOrderCertain,
	}
}

func (l *FeedEntryLinks) countIncluded(curisSet *CanonicalUriSet) int {
	count := 0
	for _, bucket := range l.LinkBuckets {
		for _, link := range bucket {
			if curisSet.Contains(link.Curi) {
				count++
			}
		}
	}
	return count
}

func (l *FeedEntryLinks) allIncluded(curisSet *CanonicalUriSet) bool {
	for _, bucket := range l.LinkBuckets {
		for _, link := range bucket {
			if !curisSet.Contains(link.Curi) {
				return false
			}
		}
	}
	return true
}

func (l *FeedEntryLinks) includedPrefixLength(curisSet *CanonicalUriSet) int {
	prefixLength := 0
	for _, bucket := range l.LinkBuckets {
		bucketIncludedCount := 0
		for _, link := range bucket {
			if curisSet.Contains(link.Curi) {
				bucketIncludedCount++
			}
		}
		prefixLength += bucketIncludedCount
		if bucketIncludedCount < len(bucket) {
			break
		}
	}
	return prefixLength
}

func (l *FeedEntryLinks) sequenceMatch(
	seqCuris []CanonicalUri, curiEqCfg *CanonicalEqualityConfig,
) ([]maybeTitledLink, bool) {
	return l.subsequenceMatch(seqCuris, 0, curiEqCfg)
}

func (l *FeedEntryLinks) subsequenceMatch(
	seqCuris []CanonicalUri, offset int, curiEqCfg *CanonicalEqualityConfig,
) ([]maybeTitledLink, bool) {
	if offset >= l.Length {
		return nil, true
	}

	currentBucketIndex := 0
	for offset >= len(l.LinkBuckets[currentBucketIndex]) {
		offset -= len(l.LinkBuckets[currentBucketIndex])
		currentBucketIndex++
	}

	remainingInBucket := len(l.LinkBuckets[currentBucketIndex]) - offset
	var subsequenceLinks []maybeTitledLink
	for _, seqCuri := range seqCuris {
		var matchingLink maybeTitledLink
		for _, bucketLink := range l.LinkBuckets[currentBucketIndex] {
			if CanonicalUriEqual(seqCuri, bucketLink.Curi, curiEqCfg) {
				matchingLink = bucketLink
				break
			}
		}
		if matchingLink == (maybeTitledLink{}) { //nolint:exhaustruct
			return nil, false
		}

		subsequenceLinks = append(subsequenceLinks, matchingLink)
		remainingInBucket--
		if remainingInBucket == 0 {
			currentBucketIndex++
			if currentBucketIndex >= len(l.LinkBuckets) {
				break
			}
			remainingInBucket = len(l.LinkBuckets[currentBucketIndex])
		}
	}

	return subsequenceLinks, true
}

func (l *FeedEntryLinks) sequenceMatchExceptFirst(
	seqCuris []CanonicalUri, curiEqCfg *CanonicalEqualityConfig,
) (bool, *maybeTitledLink) {
	if l.Length == 0 {
		return false, nil
	} else if l.Length == 1 {
		return true, &l.LinkBuckets[0][0]
	}

	firstBucket := l.LinkBuckets[0]
	if len(firstBucket) == 1 {
		_, isMatch := l.subsequenceMatch(seqCuris, 1, curiEqCfg)
		if isMatch {
			return true, &firstBucket[0]
		} else {
			return false, nil
		}
	} else if len(seqCuris) < len(firstBucket)-1 {
		// Feed starts with so many entries of the same date that we run out of sequence and don't know
		// which of the remaining links in the first bucket is the first link
		// We could return several first link candidates but let's keep things simple
		return false, nil
	} else {
		// Compare first bucket separately to see which link is not matching
		firstBucketRemaining := slices.Clone(firstBucket)
		for _, seqCuri := range seqCuris[:len(firstBucket)-1] {
			matchIndex := slices.IndexFunc(firstBucketRemaining, func(bucketLink maybeTitledLink) bool {
				return CanonicalUriEqual(seqCuri, bucketLink.Curi, curiEqCfg)
			})
			if matchIndex == -1 {
				return false, nil
			}

			firstBucketRemaining = slices.Delete(firstBucketRemaining, matchIndex, matchIndex+1)
		}

		_, isMatch := l.subsequenceMatch(seqCuris[len(firstBucket)-1:], len(firstBucket), curiEqCfg)
		if isMatch {
			return true, &firstBucketRemaining[0]
		} else {
			return false, nil
		}
	}
}

func (l *FeedEntryLinks) sequenceSuffixMatch(
	seqCuris []CanonicalUri, curiEqCfg *CanonicalEqualityConfig,
) (suffixLinks []maybeTitledLink, prefixLength int) {
	if len(seqCuris) == 0 {
		return nil, -1
	}

	startBucketIndex := slices.IndexFunc(l.LinkBuckets, func(bucket []maybeTitledLink) bool {
		return slices.IndexFunc(bucket, func(link maybeTitledLink) bool {
			return CanonicalUriEqual(link.Curi, seqCuris[0], curiEqCfg)
		}) != -1
	})
	if startBucketIndex == -1 {
		return nil, -1
	}

	startBucket := l.LinkBuckets[startBucketIndex]
	var startBucketMatchingLinks []maybeTitledLink
	seqOffset := 0
	for {
		if seqOffset >= len(seqCuris) {
			break
		}
		matchingLinkIndex := slices.IndexFunc(startBucket, func(link maybeTitledLink) bool {
			return CanonicalUriEqual(link.Curi, seqCuris[seqOffset], curiEqCfg)
		})
		if matchingLinkIndex == -1 {
			break
		}

		seqOffset++
		startBucketMatchingLinks = append(startBucketMatchingLinks, startBucket[matchingLinkIndex])
	}

	prefixLength = 0
	for _, bucket := range l.LinkBuckets[:startBucketIndex] {
		prefixLength += len(bucket)
	}
	prefixLength += len(startBucket) - seqOffset

	if seqOffset == len(seqCuris) && prefixLength+seqOffset < l.Length {
		return nil, -1
	}

	matchingLinksExceptStartBucket, ok := l.subsequenceMatch(
		seqCuris[seqOffset:], prefixLength+seqOffset, curiEqCfg,
	)
	if !ok {
		return nil, -1
	}

	suffixLinks = append(startBucketMatchingLinks, matchingLinksExceptStartBucket...)
	return suffixLinks, prefixLength
}

func (l *FeedEntryLinks) Except(curisSet CanonicalUriSet) FeedEntryLinks {
	var newLinkBuckets [][]maybeTitledLink
	length := l.Length
	for _, linkBucket := range l.LinkBuckets {
		var newLinkBucket []maybeTitledLink
		for _, link := range linkBucket {
			if curisSet.Contains(link.Curi) {
				length--
				continue
			}

			newLinkBucket = append(newLinkBucket, link)
		}
		if len(newLinkBucket) > 0 {
			newLinkBuckets = append(newLinkBuckets, newLinkBucket)
		}
	}
	return FeedEntryLinks{
		LinkBuckets:    newLinkBuckets,
		Length:         length,
		IsOrderCertain: l.IsOrderCertain,
	}
}

func (l *FeedEntryLinks) ToSlice() []*maybeTitledLink {
	result := make([]*maybeTitledLink, 0, l.Length)
	for _, linkBucket := range l.LinkBuckets {
		for i := range linkBucket {
			link := linkBucket[i]
			result = append(result, &link)
		}
	}
	return result
}

func (l *FeedEntryLinks) String() string {
	var sb strings.Builder
	sb.WriteString("[")
	for i, bucket := range l.LinkBuckets {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString("[")
		for j, link := range bucket {
			if j > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(link.Curi.String())
		}
		sb.WriteString("]")
	}
	sb.WriteString("]")
	return sb.String()
}
