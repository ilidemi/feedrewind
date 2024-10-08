package crawler

import (
	"fmt"
	neturl "net/url"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"unsafe"

	"golang.org/x/net/html"
)

type Link struct {
	Curi CanonicalUri
	Uri  *neturl.URL
	Url  string
}

func (l *Link) DeepCopy() Link {
	var userInfo *neturl.Userinfo
	if l.Uri.User != nil {
		val := reflect.ValueOf(l.Uri.User).Elem()
		typeOfUserinfo := val.Type()
		userInfoValue := reflect.New(typeOfUserinfo).Elem()
		for i := 0; i < val.NumField(); i++ {
			field := val.Field(i)
			if field.CanSet() {
				userInfoValue.Field(i).Set(field)
			} else {
				reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Set(field)
			}
		}
		userInfo = userInfoValue.Addr().Interface().(*neturl.Userinfo)
	}

	return Link{
		Curi: l.Curi.DeepCopy(),
		Uri: &neturl.URL{
			Scheme:      strings.Clone(l.Uri.Scheme),
			Opaque:      strings.Clone(l.Uri.Opaque),
			User:        userInfo,
			Host:        strings.Clone(l.Uri.Host),
			Path:        strings.Clone(l.Uri.Path),
			RawPath:     strings.Clone(l.Uri.RawPath),
			OmitHost:    l.Uri.OmitHost,
			ForceQuery:  l.Uri.ForceQuery,
			RawQuery:    strings.Clone(l.Uri.RawQuery),
			Fragment:    strings.Clone(l.Uri.Fragment),
			RawFragment: strings.Clone(l.Uri.RawFragment),
		},
		Url: string(l.Url),
	}
}

type maybeTitledLink struct {
	Link
	MaybeTitle *LinkTitle
}

func (l *maybeTitledLink) DeepCopy() maybeTitledLink {
	var maybeTitle *LinkTitle
	if l.MaybeTitle != nil {
		maybeTitleVal := l.MaybeTitle.DeepCopy()
		maybeTitle = &maybeTitleVal
	}
	return maybeTitledLink{
		Link:       l.Lnk().DeepCopy(),
		MaybeTitle: maybeTitle,
	}
}

func MaybeTitledLinksDeepCopy(links []*maybeTitledLink) []*maybeTitledLink {
	copy := make([]*maybeTitledLink, len(links))
	for i, link := range links {
		linkVal := link.DeepCopy()
		copy[i] = &linkVal
	}
	return copy
}

type maybeTitledHtmlLink struct {
	maybeTitledLink
	Element *html.Node
}

func dropHtml(links []*maybeTitledHtmlLink) []*maybeTitledLink {
	result := make([]*maybeTitledLink, len(links))
	for i, link := range links {
		result[i] = &link.maybeTitledLink
	}
	return result
}

type titledLink struct {
	Link
	Title LinkTitle
}

type xpathLink struct {
	Link
	Element    *html.Node
	XPath      string
	ClassXPath string
}

type ilink interface {
	Lnk() *Link
}

func (l *Link) Lnk() *Link                { return l }
func (l *maybeTitledLink) Lnk() *Link     { return &l.Link }
func (l *maybeTitledHtmlLink) Lnk() *Link { return &l.Link }
func (l *titledLink) Lnk() *Link          { return &l.Link }
func (l *xpathLink) Lnk() *Link           { return &l.Link }

func ToCanonicalUris[L ilink](links []L) []CanonicalUri {
	curis := make([]CanonicalUri, len(links))
	for i, link := range links {
		curis[i] = link.Lnk().Curi
	}
	return curis
}

var leadingWhitespaceRegex *regexp.Regexp
var trailingWhitespaceRegex *regexp.Regexp

func init() {
	leadingWhitespaceRegex =
		regexp.MustCompile("(?i)\\A( |\t|\n|\x00|\v|\f|\r|%20|%09|%0a|%00|%0b|%0c|%0d)+")
	trailingWhitespaceRegex =
		regexp.MustCompile("(?i)( |\t|\n|\x00|\v|\f|\r|%20|%09|%0a|%00|%0b|%0c|%0d)+\\z")
}

func ToCanonicalLink(url string, logger Logger, fetchUri *neturl.URL) (link *Link, ok bool) {
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
		return nil, false
	}

	if uri.Scheme == "mailto" {
		return nil, false
	}

	if uri.Scheme != "" && uri.Host == "" && uri.Path == "" {
		return nil, false
	}

	if uri.Scheme == "" && fetchUri != nil {
		// Relative uri
		uri = fetchUri.ResolveReference(uri)
	}

	if uri.Scheme != "http" && uri.Scheme != "https" {
		return nil, false
	}

	if uri.User != nil {
		logger.Info("Invalid URL: %q%s has userinfo: %s", uri, fetchUriStr, uri.User)
		return nil, false
	}
	if uri.Opaque != "" {
		logger.Info("Invalid URL: %q%s has opaque: %s", uri, fetchUriStr, uri.Opaque)
		return nil, false
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
	return &Link{
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
		type QueryToken struct {
			key   string
			value string
		}
		var queryTokens []QueryToken
		for key, values := range uri.Query() {
			if !whitelistedQueryParams[key] {
				continue
			}
			queryTokens = append(queryTokens, QueryToken{
				key:   key,
				value: values[0],
			})
		}
		slices.SortFunc(queryTokens, func(a, b QueryToken) int {
			return strings.Compare(a.key, b.key)
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

func (c CanonicalUri) String() string {
	return fmt.Sprintf("%s%s%s%s", c.Host, c.Port, c.Path, c.Query)
}

func (c CanonicalUri) DeepCopy() CanonicalUri {
	return CanonicalUri{
		Host:        strings.Clone(c.Host),
		Port:        strings.Clone(c.Port),
		Path:        strings.Clone(c.Path),
		TrimmedPath: strings.Clone(c.TrimmedPath),
		Query:       strings.Clone(c.Query), // technically not needed but just in case
	}
}

func CanonicalUrisDeepCopy(curis []CanonicalUri) []CanonicalUri {
	copy := make([]CanonicalUri, len(curis))
	for i, curi := range curis {
		copy[i] = curi.DeepCopy()
	}
	return copy
}

type CanonicalEqualityConfig struct {
	SameHosts         map[string]bool
	ExpectTumblrPaths bool
}

func NewCanonicalEqualityConfig() CanonicalEqualityConfig {
	return CanonicalEqualityConfig{
		SameHosts:         nil,
		ExpectTumblrPaths: false,
	}
}

func CanonicalEqualityConfigEqual(curiEqCfg1, curiEqCfg2 *CanonicalEqualityConfig) bool {
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

func CanonicalUriEqual(curi1, curi2 CanonicalUri, curiEqCfg *CanonicalEqualityConfig) bool {
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

type CanonicalUriMap[T any] struct {
	Links  []Link
	Length int

	curiEqCfg   *CanonicalEqualityConfig
	valuesByKey map[string]T
}

func NewCanonicalUriMap[T any](curiEqCfg *CanonicalEqualityConfig) CanonicalUriMap[T] {
	return CanonicalUriMap[T]{
		Links:       nil,
		Length:      0,
		curiEqCfg:   curiEqCfg,
		valuesByKey: make(map[string]T),
	}
}

func (m *CanonicalUriMap[T]) Add(link Link, value T) {
	key := canonicalUriGetKey(link.Curi, m.curiEqCfg)
	if _, ok := m.valuesByKey[key]; ok {
		return
	}

	m.valuesByKey[key] = value
	m.Links = append(m.Links, link)
	m.Length++
}

func (m *CanonicalUriMap[T]) Contains(curi CanonicalUri) bool {
	key := canonicalUriGetKey(curi, m.curiEqCfg)
	_, ok := m.valuesByKey[key]
	return ok
}

func (m *CanonicalUriMap[T]) Get(curi CanonicalUri) (T, bool) {
	key := canonicalUriGetKey(curi, m.curiEqCfg)
	value, ok := m.valuesByKey[key]
	return value, ok
}

type CanonicalUriSet struct {
	Curis  []CanonicalUri
	Length int

	curiEqCfg *CanonicalEqualityConfig
	keys      map[string]bool
}

func NewCanonicalUriSet(curis []CanonicalUri, curiEqCfg *CanonicalEqualityConfig) CanonicalUriSet {
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

func (s *CanonicalUriSet) updateEqualityConfig(curiEqCfg *CanonicalEqualityConfig) {
	curis := s.Curis
	s.Curis = nil
	s.Length = 0
	s.curiEqCfg = curiEqCfg
	s.keys = make(map[string]bool)
	s.addMany(curis)
}

func (s *CanonicalUriSet) clone() CanonicalUriSet {
	return CanonicalUriSet{
		Curis:     slices.Clone(s.Curis),
		Length:    s.Length,
		curiEqCfg: s.curiEqCfg,
		keys:      s.keys,
	}
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

func (s *CanonicalUriSet) DeepCopy() CanonicalUriSet {
	return CanonicalUriSet{
		Curis:     CanonicalUrisDeepCopy(s.Curis),
		Length:    s.Length,
		curiEqCfg: s.curiEqCfg,
		keys:      s.keys,
	}
}

func canonicalUriGetKey(curi CanonicalUri, curiEqCfg *CanonicalEqualityConfig) string {
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
