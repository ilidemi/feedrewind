package crawler

import (
	"regexp"
	"strings"
)

type LinkTitle struct {
	Value             string
	EqualizedValue    string
	Source            LinkTitleSource
	AltValuesBySource map[LinkTitleSource]string
}

type LinkTitleSource string

const (
	LinkTitleSourceFeed LinkTitleSource = "feed"
)

func NewLinkTitle(
	value string, source LinkTitleSource, altValuesBySource map[LinkTitleSource]string,
) LinkTitle {
	return LinkTitle{
		Value:             value,
		EqualizedValue:    equalizeTitle(value),
		Source:            source,
		AltValuesBySource: altValuesBySource,
	}
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
