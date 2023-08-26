//go:build e2etesting

package e2etest

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/go-rod/rod"
	"github.com/goccy/go-json"
)

func devUrl(path string) string {
	return fmt.Sprintf("http://localhost:3000/%s", path)
}

func adminUrl(path string) string {
	return fmt.Sprintf("http://localhost:3000/test/%s", path)
}

func adminSql(browser *rod.Browser, query string) []map[string]any {
	escapedQuery := url.QueryEscape(query)
	url := fmt.Sprintf("http://localhost:3000/test/execute_sql?query=%s", escapedQuery)
	page := browser.MustPage(url)
	text := page.MustElement("body").MustText()
	var result []map[string]any
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.UseNumber()
	err := decoder.Decode(&result)
	if err != nil {
		panic(err)
	}
	return result
}
