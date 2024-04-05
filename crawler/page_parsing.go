package crawler

import (
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/antchfx/htmlquery"
	"golang.org/x/net/html"
)

type includeXPathMode int

const (
	includeXPathNone includeXPathMode = iota
	includeXPathOnly
	includeXPathAndClassXPath
)

var classBlacklistRegex *regexp.Regexp
var classReplacer strings.Replacer

func init() {
	classBlacklistRegex = regexp.MustCompile(`^post-\d+$`)
	classReplacer = *strings.NewReplacer(
		"/", "%2F",
		"[", "%5B",
		"]", "%5D",
		"(", "%28",
		")", "%29",
	)
}

func extractLinks(
	document *html.Node, fetchUri *url.URL, maybeAllowedHosts map[string]bool,
	redirects map[string]*Link, logger Logger, xpathMode includeXPathMode,
) []*xpathLink {
	var links []*xpathLink
	var traverse func(element *html.Node, xpathTokens []string, classXPathTokens []string)
	traverse = func(element *html.Node, xpathTokens []string, classXPathTokens []string) {
		if element.Type == html.ElementNode {
			isLink := element.Data == "a" || element.Data == "area"
			if element.Data == "link" {
				rel := findAttr(element, "rel")
				if rel == "next" || rel == "prev" {
					isLink = true
				}
			}
			if isLink {
				href := findAttr(element, "href")
				if href != "" {
					link, ok := ToCanonicalLink(href, logger, fetchUri)
					if ok {
						link, err := followCachedRedirects(link, redirects, nil)
						if err == nil {
							if maybeAllowedHosts == nil || maybeAllowedHosts[link.Curi.Host] {
								link := xpathLink{
									Link:       *link,
									Element:    element,
									XPath:      "",
									ClassXPath: "",
								}
								if xpathMode >= includeXPathOnly {
									link.XPath = strings.Join(xpathTokens, "")
									if xpathMode == includeXPathAndClassXPath {
										link.ClassXPath = strings.Join(classXPathTokens, "")
									}
								}
								links = append(links, &link)
							}
						}
					}
				}
			}
		}
		tagCounts := make(map[string]int)
		for child := element.FirstChild; child != nil; child = child.NextSibling {
			tag, ok := getXPathTag(child)
			if !ok {
				continue
			}

			var childXPathTokens []string
			var childClassXPathTokens []string
			if xpathMode >= includeXPathOnly {
				tagCounts[tag]++
				xpathToken := fmt.Sprintf("/%s[%d]", tag, tagCounts[tag])
				childXPathTokens = slices.Clone(xpathTokens)
				childXPathTokens = append(childXPathTokens, xpathToken)

				if xpathMode == includeXPathAndClassXPath {
					classesStr := getClassesStr(child)
					childClassXPathToken := fmt.Sprintf("/%s(%s)[%d]", tag, classesStr, tagCounts[tag])
					childClassXPathTokens = slices.Clone(classXPathTokens)
					childClassXPathTokens = append(childClassXPathTokens, childClassXPathToken)
				}
			}
			traverse(child, childXPathTokens, childClassXPathTokens)
		}
	}

	traverse(document, nil, nil)

	return links
}

func getXPathTag(element *html.Node) (string, bool) {
	switch element.Type {
	case html.ElementNode:
		return element.Data, true
	case html.TextNode:
		return "text()", true
	default:
		return "", false
	}
}

func getClassesStr(element *html.Node) string {
	classesAttr := findAttr(element, "class")
	if classesAttr == "" {
		return ""
	}
	classes := strings.Split(classesAttr, " ")
	filteredClasses := make([]string, 0, len(classes))
	for _, class := range classes {
		if class == "" {
			continue
		}
		if classBlacklistRegex.MatchString(class) {
			continue
		}
		class = classReplacer.Replace(class)
		class = strings.ToLower(class)
		filteredClasses = append(filteredClasses, class)
	}
	slices.Sort(filteredClasses)
	return strings.Join(filteredClasses, ",")
}

func parseHtml(content string, logger Logger) (*html.Node, error) {
	reader := strings.NewReader(content)
	document, err := html.Parse(reader)
	if err != nil {
		return nil, err
	}

	if document.Type == html.DoctypeNode {
		document = document.NextSibling
	}

	// Remove links with empty content as some bloggers hide links this way in favor of other links,
	// which messes up the layout
	// E.g. search for 'wikipedia' on https://maryrosecook.com/blog/archive
	removedLinksCount := 0
	for _, linkElement := range htmlquery.Find(document, "//a") {
		if linkElement.FirstChild == nil {
			hasAriaLabel := false
			for _, attr := range linkElement.Attr {
				if attr.Key == "aria-label" && attr.Val != "" {
					hasAriaLabel = true
					break
				}
			}
			if !hasAriaLabel {
				linkElement.Parent.RemoveChild(linkElement)
				removedLinksCount++
			}
		}
	}
	if removedLinksCount > 0 {
		logger.Info("Removed %d empty links", removedLinksCount)
	}

	return document, nil
}

func linkFromElement(
	element *html.Node, fetchUri *url.URL, titleRelativeXPaths []titleRelativeXPath, logger Logger,
) (*maybeTitledHtmlLink, bool) {
	href := findAttr(element, "href")
	if href == "" {
		return nil, false
	}

	link, ok := ToCanonicalLink(href, logger, fetchUri)
	if !ok {
		return nil, false
	}

	result := populateLinkTitle(link, element, titleRelativeXPaths)
	return result, true
}

func innerText(element *html.Node) string {
	var builder strings.Builder
	var traverse func(n *html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.TextNode {
			builder.WriteString(n.Data)
		} else {
			for child := n.FirstChild; child != nil; child = child.NextSibling {
				traverse(child)
			}
		}
	}
	traverse(element)
	return builder.String()
}

func findAttr(node *html.Node, name string) string {
	for _, attr := range node.Attr {
		if attr.Key == name {
			return attr.Val
		}
	}
	return ""
}

func findTitle(doc *html.Node) string {
	var traverse func(n *html.Node) *string
	traverse = func(n *html.Node) *string {
		if n.Type == html.ElementNode && n.Data == "title" {
			if n.FirstChild == nil {
				title := ""
				return &title
			}
			return &n.FirstChild.Data
		}

		for child := n.FirstChild; child != nil; child = child.NextSibling {
			title := traverse(child)
			if title != nil {
				return title
			}
		}

		return nil
	}

	title := traverse(doc)
	if title == nil {
		return ""
	}
	return *title
}
