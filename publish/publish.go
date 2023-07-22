package publish

import (
	"bytes"
	"encoding/xml"
	"feedrewind/db/pgw"
	"feedrewind/log"
	"feedrewind/models"
	"feedrewind/oops"
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

func CreateEmptyUserFeed(tx pgw.Queryable, userId models.UserId) error {
	userRssText, err := generateUserRss(nil)
	if err != nil {
		return err
	}
	err = models.UserRss_Create(tx, userId, userRssText)
	if err != nil {
		return err
	}
	log.Info().Msg("Created empty user RSS")
	return nil
}

func generateUserRss(items []item) (string, error) {
	return generateRss("FeedRewind", "https://feedrewind.com", items)
}

func generateRss(title string, url string, items []item) (string, error) {
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
		return "", oops.Wrap(err)
	}

	return buf.String(), nil
}
