package crawler

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/antchfx/htmlquery"
	"golang.org/x/net/html"
)

type LinkTitle struct {
	Value             string
	EqualizedValue    string
	Source            linkTitleSource
	AltValuesBySource map[linkTitleSource]string
}

type linkTitleSource string // relative xpath or a constant

const (
	LinkTitleSourceFeed        linkTitleSource = "feed"
	LinkTitleSourceInnerText   linkTitleSource = "inner_text"
	LinkTitleSourceUrl         linkTitleSource = "url"
	LinkTitleSourcePageTitle   linkTitleSource = "page_title"
	LinkTitleSourceCollapsed   linkTitleSource = "collapsed"
	LinkTitleSourceTumblr      linkTitleSource = "tumblr"
	LinkTitleSourceGroundTruth linkTitleSource = "ground_truth"
)

func NewLinkTitle(
	value string, source linkTitleSource, altValuesBySource map[linkTitleSource]string,
) LinkTitle {
	if source == "" {
		source = LinkTitleSourceInnerText
	}
	return LinkTitle{
		Value:             value,
		EqualizedValue:    equalizeTitle(value),
		Source:            source,
		AltValuesBySource: altValuesBySource,
	}
}

func (t *LinkTitle) PrintString() string {
	alternatives := ""
	if len(t.AltValuesBySource) > 0 {
		alternatives = fmt.Sprintf(", alternatives: %v", t.AltValuesBySource)
	}
	return fmt.Sprintf("%q (%s)%s", t.Value, t.Source, alternatives)
}

func getPageTitle(page *htmlPage, feedGenerator FeedGenerator, logger Logger) string {
	ogTitleElement := htmlquery.FindOne(page.Document, "/html/head/meta[@property='og:title'][@content]")
	var title string
	if feedGenerator != FeedGeneratorTumblr && ogTitleElement != nil {
		rawTitle := findAttr(ogTitleElement, "content")
		title = normalizeTitle(rawTitle)
		logger.Info("Parsed og:title: %s", title)
	} else {
		titleElement := htmlquery.FindOne(page.Document, "/html/head/title")
		if titleElement != nil &&
			titleElement.FirstChild != nil &&
			titleElement.FirstChild.Type == html.TextNode {

			title = normalizeTitle(titleElement.FirstChild.Data)
			logger.Info("Parsed <title>: %s", title)
		} else {
			title = page.FetchUri.String()
			logger.Info("Page doesn't have title, using url instead: %s", title)
		}
	}
	return title
}

var titleEqReplacer *strings.Replacer

func init() {
	titleEqReplacer = strings.NewReplacer(
		`’`, `'`,
		`‘`, `'`,
		`”`, `"`,
		`“`, `"`,
		`…`, `...`,
		"\u200A", ` `, // Hair space
	)
}

func equalizeTitle(title string) string {
	repalcedTitle := titleEqReplacer.Replace(title)
	return strings.ToLower(repalcedTitle)
}

var newlineRegex *regexp.Regexp
var spacesRegex *regexp.Regexp

func init() {
	newlineRegex = regexp.MustCompile("\r\n|\r|\n")
	spacesRegex = regexp.MustCompile(" +")
}

func normalizeTitle(title string) string {
	if title == "" {
		return ""
	}

	title = strings.TrimSpace(title)
	if title == "" {
		return ""
	}

	title = strings.ReplaceAll(title, "\u00A0", " ") // Non-breaking space
	title = newlineRegex.ReplaceAllString(title, " ")
	title = spacesRegex.ReplaceAllString(title, " ")
	title = strings.TrimSpace(title)
	return title
}

func getElementTitle(element *html.Node) string {
	var getElementInnerText func(element *html.Node) string
	getElementInnerText = func(element *html.Node) string {
		if element.Type == html.TextNode {
			return element.Data
		}

		var tokens []string
		for child := element.FirstChild; child != nil; child = child.NextSibling {
			var token string
			if child.Type == html.ElementNode && child.Data == "br" {
				token = "\n"
			} else {
				token = getElementInnerText(child)
			}
			tokens = append(tokens, token)
		}
		return strings.Join(tokens, "")
	}

	rawTitle := getElementInnerText(element)
	return normalizeTitle(rawTitle)
}
