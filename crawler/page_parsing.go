package crawler

import (
	"strings"

	"golang.org/x/net/html"
)

func innerText(node *html.Node) string {
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
	traverse(node)
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
