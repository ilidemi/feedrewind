package crawler

import (
	"fmt"
	neturl "net/url"
	"regexp"
	"sort"
	"strings"
)

var leadingWhitespaceRegex *regexp.Regexp
var trailingWhitespaceRegex *regexp.Regexp

func init() {
	leadingWhitespaceRegex =
		regexp.MustCompile("(?i)\\A( |\t|\n|\x00|\v|\f|\r|%20|%09|%0a|%00|%0b|%0c|%0d)+")
	trailingWhitespaceRegex =
		regexp.MustCompile("(?i)( |\t|\n|\x00|\v|\f|\r|%20|%09|%0a|%00|%0b|%0c|%0d)+\\z")
}

func ToCanonicalLink(url string, logger Logger, fetchUri *neturl.URL) (Link, bool) {
	urlStripped := leadingWhitespaceRegex.ReplaceAllString(url, "")
	urlStripped = trailingWhitespaceRegex.ReplaceAllString(urlStripped, "")
	urlNewlinesRemoved := strings.ReplaceAll(urlStripped, "\n", "")

	fetchUriStr := ""
	if fetchUri != nil {
		fetchUriStr = fmt.Sprintf(" from %q", fetchUri)
	}

	uri, err := neturl.Parse(urlNewlinesRemoved)
	if err != nil {
		logger.Info("Invalid URL: %q%s has %v", url, fetchUriStr, err)
		return Link{}, false //nolint:exhaustruct
	}

	if uri.Scheme == "mailto" {
		return Link{}, false //nolint:exhaustruct
	}

	if uri.Scheme != "" && uri.Host == "" && uri.Path == "" {
		return Link{}, false //nolint:exhaustruct
	}

	if uri.Scheme == "" && fetchUri != nil {
		// Relative uri
		uri = fetchUri.ResolveReference(uri)
	}

	if uri.Scheme != "http" && uri.Scheme != "https" {
		return Link{}, false //nolint:exhaustruct
	}

	if uri.User != nil {
		logger.Info("Invalid URL: %q%s has userinfo: %s", uri, uri.User)
		return Link{}, false //nolint:exhaustruct
	}
	if uri.Opaque != "" {
		logger.Info("Invalid URL: %q%s has opaque: %s", uri, uri.Opaque)
		return Link{}, false //nolint:exhaustruct
	}

	if uri.Scheme == "http" {
		uri.Host = strings.TrimSuffix(uri.Host, ":80")
	}
	if uri.Scheme == "https" {
		uri.Host = strings.TrimSuffix(uri.Host, ":443")
	}
	uri.Path = strings.ReplaceAll(uri.Path, "//", "/")
	uri.RawPath = strings.ReplaceAll(uri.RawPath, "//", "/")
	uri.Fragment = ""
	uri.RawFragment = ""

	curi := CanonicalUriFromUri(uri)
	return Link{
		Curi: curi,
		Uri:  uri,
		Url:  uri.String(),
	}, true
}

type CanonicalUri struct {
	Host        string
	Port        string
	Path        string
	TrimmedPath string
	Query       string
}

var whitelistedQueryParams = map[string]bool{
	"page":        true,
	"year":        true,
	"m":           true, // month (apenwarr)
	"start":       true,
	"offset":      true,
	"skip":        true,
	"updated-max": true, // blogspot
	"sort":        true,
	"order":       true,
	"format":      true,
}

func CanonicalUriFromUri(uri *neturl.URL) CanonicalUri {
	var port string
	uriPort := uri.Port()
	if uriPort == "" ||
		(uriPort == "80" && uri.Scheme == "http") ||
		(uriPort == "443" && uri.Scheme == "https") {
		port = ""
	} else {
		port = ":" + uriPort
	}

	var path string
	if uri.Path == "/" && uri.RawQuery == "" {
		path = ""
	} else {
		path = uri.Path
	}

	var query string
	if uri.RawQuery != "" {
		type queryToken struct {
			key   string
			value string
		}
		var queryTokens []queryToken
		for key, values := range uri.Query() {
			if !whitelistedQueryParams[key] {
				continue
			}
			queryTokens = append(queryTokens, queryToken{
				key:   key,
				value: values[0],
			})
		}
		sort.Slice(queryTokens, func(i, j int) bool {
			return queryTokens[i].key < queryTokens[j].key
		})
		if len(queryTokens) > 0 {
			var builder strings.Builder
			for i, token := range queryTokens {
				if i == 0 {
					builder.WriteString("?")
				} else {
					builder.WriteString("&")
				}
				builder.WriteString(token.key)
				builder.WriteString("=")
				builder.WriteString(token.value)
			}
			query = builder.String()
		} else {
			query = ""
		}
	} else {
		query = ""
	}

	return CanonicalUri{
		Host:        uri.Host,
		Port:        port,
		Path:        path,
		TrimmedPath: strings.TrimRight(path, "/"),
		Query:       query,
	}
}

func CanonicalUriFromDbString(dbString string) CanonicalUri {
	dummyUri, err := neturl.Parse("http://" + dbString)
	if err != nil {
		panic(err)
	}
	return CanonicalUriFromUri(dummyUri)
}

func (c *CanonicalUri) String() string {
	return fmt.Sprintf("%s%s%s%s", c.Host, c.Port, c.Path, c.Query)
}

type CanonicalEqualityConfig struct {
	SameHosts         map[string]bool
	ExpectTumblrPaths bool
}

func CanonicalEqualityConfigEqual(curiEqCfg1, curiEqCfg2 CanonicalEqualityConfig) bool {
	if curiEqCfg1.ExpectTumblrPaths != curiEqCfg2.ExpectTumblrPaths {
		return false
	}

	if len(curiEqCfg1.SameHosts) != len(curiEqCfg2.SameHosts) {
		return false
	}

	for sameHost := range curiEqCfg1.SameHosts {
		if !curiEqCfg2.SameHosts[sameHost] {
			return false
		}
	}

	return true
}

func CanonicalUriPathEqual(curi1, curi2 CanonicalUri) bool {
	return curi1.TrimmedPath == curi2.TrimmedPath
}

var tumblrPathRegex *regexp.Regexp

func init() {
	tumblrPathRegex = regexp.MustCompile(`^(/post/\d+)(?:/[^/]+)?/?$`)
}

func CanonicalUriEqual(curi1, curi2 CanonicalUri, curiEqCfg CanonicalEqualityConfig) bool {
	if !(curi1.Host == curi2.Host || (curiEqCfg.SameHosts[curi1.Host] && curiEqCfg.SameHosts[curi2.Host])) {
		return false
	}

	if curiEqCfg.ExpectTumblrPaths {
		tumblrMatch1 := tumblrPathRegex.FindStringSubmatch(curi1.Path)
		tumblrMatch2 := tumblrPathRegex.FindStringSubmatch(curi2.Path)
		if tumblrMatch1 != nil && tumblrMatch2 != nil && tumblrMatch1[1] == tumblrMatch2[1] {
			return true
		}
	}
	if !CanonicalUriPathEqual(curi1, curi2) {
		return false
	}

	return curi1.Query == curi2.Query
}

type CanonicalUriSet struct {
	Curis  []CanonicalUri
	Length int

	curiEqCfg CanonicalEqualityConfig
	keys      map[string]bool
}

func NewCanonicalUriSet(curis []CanonicalUri, curiEqCfg CanonicalEqualityConfig) CanonicalUriSet {
	result := CanonicalUriSet{
		Curis:     nil,
		Length:    0,
		curiEqCfg: curiEqCfg,
		keys:      make(map[string]bool),
	}
	result.addMany(curis)
	return result
}

func (s *CanonicalUriSet) Contains(curi CanonicalUri) bool {
	key := canonicalUriGetKey(curi, s.curiEqCfg)
	return s.keys[key]
}

//nolint:unused
func (s *CanonicalUriSet) updateEqualityConfig(curiEqCfg CanonicalEqualityConfig) {
	curis := s.Curis
	s.Curis = nil
	s.Length = 0
	s.curiEqCfg = curiEqCfg
	s.keys = make(map[string]bool)
	s.addMany(curis)
}

//nolint:unused
func (s *CanonicalUriSet) merge(other *CanonicalUriSet) CanonicalUriSet {
	if !CanonicalEqualityConfigEqual(s.curiEqCfg, other.curiEqCfg) {
		panic("canonical equality configs are not equal")
	}

	result := CanonicalUriSet{
		Curis:     s.Curis,
		Length:    s.Length,
		curiEqCfg: s.curiEqCfg,
		keys:      s.keys,
	}
	result.addMany(other.Curis)
	return result
}

func (s *CanonicalUriSet) addMany(curis []CanonicalUri) {
	for _, curi := range curis {
		s.add(curi)
	}
}

func (s *CanonicalUriSet) add(curi CanonicalUri) {
	key := canonicalUriGetKey(curi, s.curiEqCfg)
	if s.keys[key] {
		return
	}

	s.keys[key] = true
	s.Curis = append(s.Curis, curi)
	s.Length++
}

func canonicalUriGetKey(curi CanonicalUri, curiEqCfg CanonicalEqualityConfig) string {
	server := curi.Host + curi.Port
	serverKey := server
	if curiEqCfg.SameHosts[server] {
		serverKey = "__same_hosts"
	}

	trimmedPath := curi.TrimmedPath
	if curiEqCfg.ExpectTumblrPaths {
		tumblrMatch := tumblrPathRegex.FindStringSubmatch(curi.Path)
		if tumblrMatch != nil {
			trimmedPath = tumblrMatch[1]
		}
	}

	return fmt.Sprintf("%s/%s?%s", serverKey, trimmedPath, curi.Query)
}
