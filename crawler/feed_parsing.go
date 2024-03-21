package crawler

import (
	"errors"
	"feedrewind/oops"
	"fmt"
	"html"
	"io"
	neturl "net/url"
	"sort"
	"strings"
	"time"

	"github.com/antchfx/xmlquery"
	"golang.org/x/text/encoding/charmap"
)

func isFeed(body string, logger Logger) bool {
	if body == "" {
		return false
	}

	xml, err := parseXML(body)
	if err != nil {
		return false
	}

	return isRSS(xml) || isRDF(xml) || isAtom(xml)
}

func parseXML(body string) (*xmlquery.Node, error) {
	reader := strings.NewReader(body)
	xml, err := xmlquery.ParseWithOptions(reader, xmlquery.ParserOptions{
		Decoder: &xmlquery.DecoderOptions{ //nolint:exhaustruct
			Strict: false,
			CharsetReader: func(charset string, input io.Reader) (io.Reader, error) {
				if strings.ToLower(charset) == "iso-8859-1" {
					return charmap.ISO8859_1.NewDecoder().Reader(input), nil
				}
				return nil, fmt.Errorf("Unknown XML charset: %s", charset)
			},
		},
	})
	return xml, err
}

func isRSS(xml *xmlquery.Node) bool {
	channel := xmlquery.FindOne(xml, "/rss/channel")
	return channel != nil
}

func isRDF(xml *xmlquery.Node) bool {
	rdf := xmlquery.FindOne(xml, "/rdf:RDF[@xmlns='http://purl.org/rss/1.0/'][@xmlns:rdf]")
	if rdf == nil {
		return false
	}

	channel := xmlquery.FindOne(rdf, "/channel")
	return channel != nil
}

func isAtom(xml *xmlquery.Node) bool {
	feed := xmlquery.FindOne(xml, "/feed")
	if feed == nil {
		return false
	}

	if feed.NamespaceURI != "http://www.w3.org/2005/Atom" {
		return false
	}

	return true
}

type ParsedFeed struct {
	Title      string
	RootLink   *Link
	EntryLinks FeedEntryLinks
	Generator  FeedGenerator
}

type FeedGenerator string

const (
	FeedGeneratorOther   FeedGenerator = ""
	FeedGeneratorTumblr  FeedGenerator = "tumblr"
	FeedGeneratorBlogger FeedGenerator = "blogger"
	FeedGeneratorMedium  FeedGenerator = "medium"
)

type feedEntry struct {
	title   string
	pubDate time.Time
	url     string
}

func ParseFeed(content string, fetchUri *neturl.URL, logger Logger) (*ParsedFeed, error) {
	xml, err := parseXML(content)
	if err != nil {
		return nil, oops.Wrap(err)
	}

	hasFeedburnerNamespace := xmlquery.FindOne(xml, " //*[@xmlns:feedburner]") != nil

	var feedTitle string
	var rootUrl string
	var entries []feedEntry
	generator := FeedGeneratorOther

	if isRSS(xml) {
		logger.Info("RSS Feed")

		channel := xmlquery.FindOne(xml, "/rss/channel")
		feedTitleNode := xmlquery.FindOne(channel, "title")
		if feedTitleNode != nil {
			feedTitle = strings.TrimSpace(feedTitleNode.InnerText())
		}
		rootUrlNodes := xmlquery.Find(channel, "link")
		for _, node := range rootUrlNodes {
			if node.NamespaceURI == "" {
				rootUrl = node.InnerText()
				break
			}
		}

		isPermalinkGuidUsed := false
		itemNodes := xmlquery.Find(channel, "item")
		for _, itemNode := range itemNodes {
			var itemTitle string
			itemTitleNode := xmlquery.FindOne(itemNode, "title")
			if itemTitleNode != nil {
				itemTitle = strings.TrimSpace(itemTitleNode.InnerText())
			}

			var pubDate time.Time
			pubDateNodes := xmlquery.Find(itemNode, "pubDate")
			if len(pubDateNodes) == 1 {
				var err error
				text := pubDateNodes[0].InnerText()
				pubDate, err = time.Parse(time.RFC1123, text)
				if err != nil {
					pubDate, err = time.Parse(time.RFC1123Z, text)
					if err != nil {
						logger.Info("Invalid pubDate: %s", text)
						pubDate = time.Time{} //nolint:exhaustruct
					}
				}
			}

			if hasFeedburnerNamespace {
				logger.Info("Feed is from Feedburner")
				feedburnerOrigLinkNode := xmlquery.FindOne(itemNode, "feedburner:origLink")
				if feedburnerOrigLinkNode != nil {
					entries = append(entries, feedEntry{
						title:   itemTitle,
						pubDate: pubDate,
						url:     feedburnerOrigLinkNode.InnerText(),
					})
					continue
				}
			}

			linkNode := xmlquery.FindOne(itemNode, "link")
			if linkNode != nil {
				entries = append(entries, feedEntry{
					title:   itemTitle,
					pubDate: pubDate,
					url:     linkNode.InnerText(),
				})
				continue
			}

			permalinkGuidNode := xmlquery.FindOne(itemNode, "guid[@isPermaLink='true']")
			if permalinkGuidNode != nil {
				isPermalinkGuidUsed = true
				entries = append(entries, feedEntry{
					title:   itemTitle,
					pubDate: pubDate,
					url:     permalinkGuidNode.InnerText(),
				})
				continue
			}

			return nil, oops.New("couldn't extract item urls from RSS")
		}

		if isPermalinkGuidUsed {
			logger.Info("Permalink guid used")
		}

		generatorNode := xmlquery.FindOne(channel, "generator")
		if generatorNode != nil {
			generatorText := strings.ToLower(generatorNode.InnerText())
			if strings.HasPrefix(generatorText, "tumblr") {
				generator = FeedGeneratorTumblr
			} else if generatorText == "blogger" {
				generator = FeedGeneratorBlogger
			} else if generatorText == "medium" {
				generator = FeedGeneratorMedium
			}

			if generator != FeedGeneratorOther {
				logger.Info("Feed generator: %s", generator)
			}
		}
	} else if isRDF(xml) {
		logger.Info("RDF feed")

		channel := xmlquery.FindOne(xml, "/rdf:RDF/channel")
		feedTitleNode := xmlquery.FindOne(channel, "title")
		if feedTitleNode != nil {
			feedTitle = strings.TrimSpace(feedTitleNode.InnerText())
		}
		rootUrlNode := xmlquery.FindOne(channel, "link")
		if rootUrlNode != nil {
			rootUrl = rootUrlNode.InnerText()
		}

		itemNodes := xmlquery.Find(xml, "/rdf:RDF/channel/item")
		for _, itemNode := range itemNodes {
			var itemTitle string
			itemTitleNode := xmlquery.FindOne(itemNode, "title")
			if itemTitleNode != nil {
				itemTitle = itemTitleNode.InnerText()
			}

			var pubDate time.Time
			dateNodes := xmlquery.Find(itemNode, "dc:date")
			if len(dateNodes) == 1 {
				var ok bool
				text := dateNodes[0].InnerText()
				pubDate, ok = parseISO8601(text)
				if !ok {
					logger.Info("Invalid date: %s", text)
					pubDate = time.Time{} //nolint:exhaustruct
				}
			}

			linkNode := xmlquery.FindOne(itemNode, "link")
			if linkNode != nil {
				entries = append(entries, feedEntry{
					title:   itemTitle,
					pubDate: pubDate,
					url:     linkNode.InnerText(),
				})
				continue
			}

			return nil, oops.New("couldn't extract item urls from RDF")
		}

		// Generator stays uninitialized

	} else {
		logger.Info("Atom feed")
		if hasFeedburnerNamespace {
			logger.Info("Feed is from feedburner")
		}

		atomFeed := xmlquery.FindOne(xml, "/feed")
		feedTitleNode := xmlquery.FindOne(atomFeed, "title")
		if feedTitleNode != nil {
			feedTitle = strings.TrimSpace(feedTitleNode.InnerText())
		}
		rootUrl, err = getAtomUrl(atomFeed, false)
		if err != nil {
			logger.Info("Couldn't extract root url: %v", err)
		}

		entryNodes := xmlquery.Find(atomFeed, "entry")
		isPublishedDateUsed := false
		isUpdatedDateUsed := false
		for _, entryNode := range entryNodes {
			var entryTitle string
			entryTitleNode := xmlquery.FindOne(entryNode, "title")
			if entryTitleNode != nil {
				entryTitle = strings.TrimSpace(entryTitleNode.InnerText())
			}

			dateNodes := xmlquery.Find(entryNode, "published")
			if len(dateNodes) > 0 {
				isPublishedDateUsed = true
			} else {
				dateNodes = xmlquery.Find(entryNode, "updated")
				if len(dateNodes) > 0 {
					isUpdatedDateUsed = true
				}
			}

			var pubDate time.Time
			if len(dateNodes) == 1 {
				var ok bool
				text := dateNodes[0].InnerText()
				pubDate, ok = parseISO8601(text)
				if !ok {
					logger.Info("Invalid date: %s", text)
					pubDate = time.Time{} //nolint:exhaustruct
				}
			}

			url, err := getAtomUrl(entryNode, hasFeedburnerNamespace)
			if err != nil {
				return nil, oops.Newf("couldn't extract entry urls from Atom: %w", err)
			}

			entries = append(entries, feedEntry{
				title:   entryTitle,
				pubDate: pubDate,
				url:     url,
			})
		}

		if isPublishedDateUsed && isUpdatedDateUsed {
			logger.Info("Published and updated dates used")
		} else if isPublishedDateUsed {
			logger.Info("Published dates used")
		} else if isUpdatedDateUsed {
			logger.Info("Updated dates used")
		}

		generatorNode := xmlquery.FindOne(atomFeed, "generator")
		if generatorNode != nil {
			generatorText := strings.ToLower(generatorNode.InnerText())
			if generatorText == "blogger" {
				generator = FeedGeneratorBlogger
			}

			if generator != FeedGeneratorOther {
				logger.Info("Feed generator: %s", generator)
			}
		}
	}

	var normalizedFeedTitle string
	if feedTitle == "" {
		logger.Info("Feed title is absent")
		normalizedFeedTitle = fetchUri.Host
	} else {
		logger.Info("Feed title is present")
		decodedFeedTitle := decodeHtmlTitle(feedTitle)
		if decodedFeedTitle != feedTitle {
			logger.Info("Feed title needs HTML decoding")
		}
		normalizedFeedTitle = normalizeTitle(decodedFeedTitle)
	}
	logger.Info("Feed title: %s", normalizedFeedTitle)

	var rootLink *Link
	if rootUrl != "" {
		var ok bool
		rootLink, ok = ToCanonicalLink(rootUrl, logger, fetchUri)
		if !ok {
			logger.Info("Malformed root url: %s", rootUrl)
		}
	}
	if rootLink != nil {
		logger.Info("Feed root url: %s", rootLink.Url)
	} else {
		logger.Info("Feed root url is absent")
	}

	sortedEntries, areDatesCertain := trySortReverseChronological(entries, logger)
	entryTitleCount := 0
	entryTitleNeedsDecodingCount := 0
	var entryLinks []FeedEntryLink
	for _, entry := range sortedEntries {
		link, ok := ToCanonicalLink(entry.url, logger, fetchUri)
		if !ok {
			return nil, oops.Newf("couldn't parse link: %s", entry.url)
		}
		decodedEntryTitle := decodeHtmlTitle(entry.title)
		if decodedEntryTitle != entry.title {
			entryTitleNeedsDecodingCount++
		}
		linkTitleValue := normalizeTitle(decodedEntryTitle)
		var maybeLinkTitle *LinkTitle
		if linkTitleValue != "" {
			entryTitleCount++
			linkTitle := NewLinkTitle(linkTitleValue, LinkTitleSourceFeed, nil)
			maybeLinkTitle = &linkTitle
		}
		var maybeDate *time.Time
		if areDatesCertain {
			date := entry.pubDate
			maybeDate = &date
		}
		entryLinks = append(entryLinks, FeedEntryLink{
			maybeTitledLink: maybeTitledLink{
				Link:       *link,
				MaybeTitle: maybeLinkTitle,
			},
			MaybeDate: maybeDate,
		})
	}

	feedEntryLinks := newFeedEntryLinks(entryLinks)
	logger.Info("Feed entries: %d", feedEntryLinks.Length)
	logger.Info("Feed entry titles present: %d", entryTitleCount)
	logger.Info("Feed entry titles needed HTML decoding: %d", entryTitleNeedsDecodingCount)
	logger.Info("Feed entry order certain: %t", feedEntryLinks.IsOrderCertain)

	return &ParsedFeed{
		Title:      normalizedFeedTitle,
		RootLink:   rootLink,
		EntryLinks: feedEntryLinks,
		Generator:  generator,
	}, nil
}

func parseISO8601(value string) (time.Time, bool) {
	result, err := time.Parse(time.RFC3339, value)
	if err != nil {
		result, err = time.Parse("2006-01-02T15:04:05Z0700", value) // No colon in tz
		if err != nil {
			result, err = time.Parse("2006-01-02", value)
			if err != nil {
				return time.Time{}, false //nolint:exhaustruct
			}
		}
	}
	return result, true
}

func getAtomUrl(nodeWithLink *xmlquery.Node, hasFeedburnerNamespace bool) (string, error) {
	if hasFeedburnerNamespace {
		feedburnerOrigLinkNode := xmlquery.FindOne(nodeWithLink, "feedburner:origLink")
		if feedburnerOrigLinkNode != nil {
			return feedburnerOrigLinkNode.InnerText(), nil
		}
	}

	linkNodes := xmlquery.Find(nodeWithLink, "link")
	var linkCandidates []*xmlquery.Node
	for _, linkNode := range linkNodes {
		if linkNode.SelectAttr("rel") == "alternate" {
			linkCandidates = append(linkCandidates, linkNode)
		}
	}
	if len(linkCandidates) == 0 {
		for _, linkNode := range linkNodes {
			if linkNode.SelectAttr("rel") == "" {
				linkCandidates = append(linkCandidates, linkNode)
			}
		}
	}
	if len(linkCandidates) == 0 {
		return "", errors.New("no candidate links")
	}
	if len(linkCandidates) != 1 {
		return "", fmt.Errorf("more than one candidate link: %d", len(linkCandidates))
	}
	var url string
	for _, attr := range linkCandidates[0].Attr {
		if attr.Name.Local == "href" {
			url = attr.Value
		}
	}
	if url == "" {
		return "", fmt.Errorf("no url in link")
	}

	return url, nil
}

func decodeHtmlTitle(title string) string {
	title = html.UnescapeString(title)
	title = strings.ReplaceAll(title, "<br>", "\n")
	title = strings.ReplaceAll(title, "<br/>", "\n")
	return title
}

func trySortReverseChronological(
	items []feedEntry, logger Logger,
) (sortedItems []feedEntry, areDatesCertain bool) {
	for _, item := range items {
		if item.pubDate == (time.Time{}) { //nolint:exhaustruct
			logger.Info("Dates are missing")
			return items, false
		}
	}

	if len(items) < 2 {
		logger.Info("Feed has less than 2 items")
		return items, false
	}

	allDatesEqual := true
	areDatesAscending := true
	areDatesDescending := true
	for i := 0; i < len(items)-1; i++ {
		date1 := items[i].pubDate
		date2 := items[i+1].pubDate
		if !date1.Equal(date2) {
			allDatesEqual = false
		}
		if date1.After(date2) {
			areDatesAscending = false
		}
		if date2.After(date1) {
			areDatesDescending = false
		}
	}

	if allDatesEqual {
		logger.Info("All item dates are equal")
	}

	if !areDatesAscending && !areDatesDescending {
		logger.Info("Item dates are unsorted")
		sortedItems = make([]feedEntry, len(items))
		copy(sortedItems, items)
		sort.SliceStable(sortedItems, func(i, j int) bool {
			return sortedItems[i].pubDate.After(sortedItems[j].pubDate)
		})
	} else if areDatesAscending {
		logger.Info("Item dates are ascending")
		sortedItems = make([]feedEntry, len(items))
		for i, item := range items {
			sortedItems[len(items)-i-1] = item
		}
	} else {
		logger.Info("Item dates are descending")
		sortedItems = items
	}
	return sortedItems, true
}
