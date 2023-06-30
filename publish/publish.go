package publish

import (
	"bytes"
	"context"
	"encoding/xml"
	"feedrewind/db/pgw"
	"feedrewind/log"
	"feedrewind/models"
	"fmt"
)

type rss struct {
	Version          string  `xml:"version,attr"`
	ContentNamespace string  `xml:"xmlns:content,attr"`
	Channel          channel `xml:"channel"`
}

type channel struct {
	Title string `xml:"title"`
	Link  string `xml:"link"`
	Items []item `xml:"item"`
}

type item struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	Guid        string `xml:"guid"`
	PubDate     string `xml:"pubDate"`
}

func MustCreateEmptyUserFeed(ctx context.Context, tx pgw.Queryable, userId models.UserId) {
	userRssText := mustGenerateUserRss(nil)
	models.UserRss_MustCreate(ctx, tx, userId, userRssText)
	log.Info().Msg("Created empty user RSS")
}

func mustGenerateUserRss(items []item) string {
	return mustGenerateRss("FeedRewind", "https://feedrewind.com", items)
}

func mustGenerateRss(title string, url string, items []item) string {
	var buf bytes.Buffer
	_, _ = fmt.Fprint(&buf, xml.Header)
	encoder := xml.NewEncoder(&buf)
	encoder.Indent("", "  ")
	err := encoder.Encode(rss{
		Version:          "2.0",
		ContentNamespace: "http://purl.org/rss/1.0/modules/content/",
		Channel: channel{
			Title: title,
			Link:  url,
			Items: items,
		},
	})
	if err != nil {
		panic(err)
	}

	return buf.String()
}
