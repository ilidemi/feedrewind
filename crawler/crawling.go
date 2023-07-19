package crawler

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
)

type CrawlContext struct{}

type page struct {
	content  string
	document *html.Node
	fetchUri *url.URL
}

// TODO ProgressLogger
func crawlRequest(
	initialLink Link, isFeedExpected bool, crawlCtx *CrawlContext, httpClient *HttpClient, logger Logger,
) (page, error) {
	// TODO

	resp, err := http.Get(initialLink.Url)
	if err != nil {
		return page{}, err //nolint:exhaustruct
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return page{}, fmt.Errorf("crawl status %d", resp.StatusCode) //nolint:exhaustruct
	}
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return page{}, err //nolint:exhaustruct
	}

	var contentType string
	var body string
	contentTypeHeader := resp.Header.Get("Content-Type")
	if contentTypeHeader != "" {
		tokens := strings.Split(contentTypeHeader, ";")
		contentType = strings.TrimSpace(tokens[0])
		encoding, _, _ := charset.DetermineEncoding(bodyBytes, contentTypeHeader)
		decoder := encoding.NewDecoder()
		body, err = decoder.String(string(bodyBytes))
		if err != nil {
			body = string(bodyBytes)
		}
	} else {
		contentType = ""
		body = string(bodyBytes)
	}

	var content string
	var document *html.Node
	if contentType == "text/html" {
		content = body
		reader := bytes.NewReader(bodyBytes)
		document, err = html.Parse(reader)
		if err != nil {
			document = nil
		}
	} else if isFeedExpected && isFeed(body, logger) {
		content = body
		document = nil
	} else {
		content = ""
		document = nil
	}

	// TODO meta_refresh_content
	// TODO crawl_ctx and log

	return page{
		content:  content,
		document: document,
		fetchUri: initialLink.Uri,
	}, nil
}
