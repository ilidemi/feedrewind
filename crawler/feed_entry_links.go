package crawler

import "time"

type FeedEntryLinks struct {
	LinkBuckets    [][]MaybeTitledLink
	Length         int
	IsOrderCertain bool
}

func feedEntryLinksFromLinksDates(links []MaybeTitledLink, dates []time.Time) FeedEntryLinks {
	var isOrderCertain bool
	var linkBuckets [][]MaybeTitledLink
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
				linkBuckets = append(linkBuckets, []MaybeTitledLink{link})
			}
			lastDate = date
		}
	} else {
		isOrderCertain = false
		for _, link := range links {
			linkBuckets = append(linkBuckets, []MaybeTitledLink{link})
		}
	}

	return FeedEntryLinks{
		LinkBuckets:    linkBuckets,
		Length:         len(links),
		IsOrderCertain: isOrderCertain,
	}
}

func (l *FeedEntryLinks) sequenceMatch(
	seqCuris []CanonicalUri, curiEqCfg CanonicalEqualityConfig,
) []MaybeTitledLink {
	return l.subsequenceMatch(seqCuris, 0, curiEqCfg)
}

func (l *FeedEntryLinks) subsequenceMatch(
	seqCuris []CanonicalUri, offset int, curiEqCfg CanonicalEqualityConfig,
) []MaybeTitledLink {
	if offset >= l.Length {
		return nil
	}

	currentBucketIndex := 0
	for offset >= len(l.LinkBuckets[currentBucketIndex]) {
		offset -= len(l.LinkBuckets[currentBucketIndex])
		currentBucketIndex++
	}

	remainingInBucket := len(l.LinkBuckets[currentBucketIndex]) - offset
	var subsequenceLinks []MaybeTitledLink
	for _, seqCuri := range seqCuris {
		var matchingLink MaybeTitledLink
		for _, bucketLink := range l.LinkBuckets[currentBucketIndex] {
			if CanonicalUriEqual(seqCuri, bucketLink.Curi, curiEqCfg) {
				matchingLink = bucketLink
				break
			}
		}
		if matchingLink == (MaybeTitledLink{}) { //nolint:exhaustruct
			return nil
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

	return subsequenceLinks
}

func (l *FeedEntryLinks) sequenceSuffixLength(
	seqCuris []CanonicalUri, curiEqCfg CanonicalEqualityConfig,
) int {
	if len(seqCuris) == 0 {
		return 0
	}

	currentBucketIndex := -1
outer:
	for i := 0; i < len(l.LinkBuckets); i++ {
		for _, link := range l.LinkBuckets[i] {
			if CanonicalUriEqual(link.Curi, seqCuris[0], curiEqCfg) {
				currentBucketIndex = i
				break outer
			}
		}
	}
	if currentBucketIndex == -1 {
		return 0
	}

	seqIndex := 0
	for {
		if seqIndex >= len(seqCuris) {
			if currentBucketIndex == len(l.LinkBuckets)-1 {
				// Ran out at the same time
				return seqIndex
			} else {
				// Ran out of sequence before running out of entries
				return 0
			}
		}

		found := false
		for _, link := range l.LinkBuckets[currentBucketIndex] {
			if CanonicalUriEqual(link.Curi, seqCuris[seqIndex], curiEqCfg) {
				found = true
				break
			}
		}
		if !found {
			break
		}

		seqIndex++
	}

	if currentBucketIndex == len(l.LinkBuckets)-1 {
		return seqIndex
	}

	currentBucketIndex++
	remainingInBucket := len(l.LinkBuckets[currentBucketIndex])
	for ; seqIndex < len(seqCuris); seqIndex++ {
		found := false
		for _, bucketLink := range l.LinkBuckets[currentBucketIndex] {
			if CanonicalUriEqual(seqCuris[seqIndex], bucketLink.Curi, curiEqCfg) {
				found = true
				break
			}
		}
		if !found {
			return 0
		}

		remainingInBucket--
		if remainingInBucket == 0 {
			currentBucketIndex++
			if currentBucketIndex >= len(l.LinkBuckets) {
				return seqIndex + 1
			}
			remainingInBucket = len(l.LinkBuckets[currentBucketIndex])
		}
	}

	if remainingInBucket == 0 && currentBucketIndex == len(l.LinkBuckets)-1 {
		return seqIndex + 1
	} else {
		return 0
	}
}

func (l *FeedEntryLinks) Except(curisSet CanonicalUriSet) FeedEntryLinks {
	var newLinkBuckets [][]MaybeTitledLink
	length := l.Length
	for _, linkBucket := range l.LinkBuckets {
		var newLinkBucket []MaybeTitledLink
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

func (l *FeedEntryLinks) ToSlice() []MaybeTitledLink {
	result := make([]MaybeTitledLink, 0, l.Length)
	for _, linkBucket := range l.LinkBuckets {
		result = append(result, linkBucket...)
	}
	return result
}
