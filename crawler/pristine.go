package crawler

import (
	"fmt"
	neturl "net/url"
	"reflect"
	"slices"
	"strings"
	"unsafe"
)

// The memory of pristine objects are guaranteed to not refer to outside data, like heavy htmls

type pristineUri struct {
	Uri neturl.URL
}

func NewPristineUri(uri *neturl.URL) *pristineUri {
	var userInfo *neturl.Userinfo
	if uri.User != nil {
		val := reflect.ValueOf(uri.User).Elem()
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
	return &pristineUri{
		Uri: neturl.URL{
			Scheme:      strings.Clone(uri.Scheme),
			Opaque:      strings.Clone(uri.Opaque),
			User:        userInfo,
			Host:        strings.Clone(uri.Host),
			Path:        strings.Clone(uri.Path),
			RawPath:     strings.Clone(uri.RawPath),
			OmitHost:    uri.OmitHost,
			ForceQuery:  uri.ForceQuery,
			RawQuery:    strings.Clone(uri.RawQuery),
			Fragment:    strings.Clone(uri.Fragment),
			RawFragment: strings.Clone(uri.RawFragment),
		},
	}
}

func (u *pristineUri) String() string {
	return u.Uri.String()
}

type pristineCanonicalUri struct {
	Curi CanonicalUri
}

func NewPristineCanonicalUri(curi CanonicalUri) pristineCanonicalUri {
	return pristineCanonicalUri{
		Curi: CanonicalUri{
			Host:        strings.Clone(curi.Host),
			Port:        strings.Clone(curi.Port),
			Path:        strings.Clone(curi.Path),
			TrimmedPath: strings.Clone(curi.TrimmedPath),
			Query:       strings.Clone(curi.Query), // technically not needed but just in case
		},
	}
}

func NewPristineCanonicalUris(curis []CanonicalUri) []pristineCanonicalUri {
	copy := make([]pristineCanonicalUri, len(curis))
	for i, curi := range curis {
		copy[i] = NewPristineCanonicalUri(curi)
	}
	return copy
}

type pristineCanonicalUriSet struct {
	CuriSet CanonicalUriSet
}

func NewPristineCanonicalUriSet(curiSet CanonicalUriSet) pristineCanonicalUriSet {
	curis := make([]CanonicalUri, len(curiSet.Curis))
	for i, curi := range curiSet.Curis {
		curis[i] = NewPristineCanonicalUri(curi).Curi
	}
	return pristineCanonicalUriSet{
		CuriSet: CanonicalUriSet{
			Curis:     curis,
			Length:    curiSet.Length,
			curiEqCfg: curiSet.curiEqCfg,
			keys:      curiSet.keys,
		},
	}
}

func (s pristineCanonicalUriSet) Contains(curi CanonicalUri) bool {
	return s.CuriSet.Contains(curi)
}

func (s pristineCanonicalUriSet) clone() pristineCanonicalUriSet {
	return pristineCanonicalUriSet{
		CuriSet: s.CuriSet.clone(),
	}
}

func (s pristineCanonicalUriSet) addMany(curis []CanonicalUri) {
	pristineCuris := make([]CanonicalUri, len(curis))
	for i, curi := range curis {
		pristineCuris[i] = NewPristineCanonicalUri(curi).Curi
	}
	s.CuriSet.addMany(pristineCuris)
}

type pristineLink struct {
	PristineCuri pristineCanonicalUri
	Uri          *pristineUri
	Url          string
}

func NewPristineLink(link *Link) *pristineLink {
	return &pristineLink{
		PristineCuri: NewPristineCanonicalUri(link.Curi),
		Uri:          NewPristineUri(link.Uri),
		Url:          strings.Clone(link.Url),
	}
}

func (l *pristineLink) Unwrap() *Link {
	return &Link{
		Curi: l.PristineCuri.Curi,
		Uri:  &l.Uri.Uri,
		Url:  l.Url,
	}
}

func (l *pristineLink) Curi() CanonicalUri {
	return l.PristineCuri.Curi
}

type pristineMaybeTitledLink struct {
	Link       pristineLink
	MaybeTitle *pristineLinkTitle
}

func NewPristineMaybeTitledLink(link *maybeTitledLink) *pristineMaybeTitledLink {
	var maybeTitle *pristineLinkTitle
	if link.MaybeTitle != nil {
		maybeTitleVal := NewPristineLinkTitle(*link.MaybeTitle)
		maybeTitle = &maybeTitleVal
	}
	return &pristineMaybeTitledLink{
		Link:       *NewPristineLink(&link.Link),
		MaybeTitle: maybeTitle,
	}
}

func NewPristineMaybeTitledLinks(links []*maybeTitledLink) []*pristineMaybeTitledLink {
	result := make([]*pristineMaybeTitledLink, len(links))
	for i, link := range links {
		result[i] = NewPristineMaybeTitledLink(link)
	}
	return result
}

func (l *pristineMaybeTitledLink) Curi() CanonicalUri {
	return l.Link.Curi()
}

func (l *pristineMaybeTitledLink) Unwrap() *maybeTitledLink {
	var maybeTitle *LinkTitle
	if l.MaybeTitle != nil {
		maybeTitle = &l.MaybeTitle.Title
	}
	return &maybeTitledLink{
		Link:       *l.Link.Unwrap(),
		MaybeTitle: maybeTitle,
	}
}

type pristineLinkTitle struct {
	Title LinkTitle
}

func NewPristineLinkTitle(title LinkTitle) pristineLinkTitle {
	var altValuesBySource map[linkTitleSource]string
	if title.MaybeAltValuesBySource != nil {
		altValuesBySource = map[linkTitleSource]string{}
		for source, value := range title.MaybeAltValuesBySource {
			altValuesBySource[source] = strings.Clone(value)
		}
	}
	return pristineLinkTitle{
		Title: LinkTitle{
			Value:                  strings.Clone(title.Value),
			EqualizedValue:         strings.Clone(title.EqualizedValue),
			Source:                 title.Source,
			MaybeAltValuesBySource: altValuesBySource,
		},
	}
}

type pristineHistoricalBlogPostCategory struct {
	Name      string
	IsTop     bool
	PostLinks []pristineLink
}

func NewPristineHistoricalBlogPostCategory(
	name string, isTop bool, postLinks []Link,
) pristineHistoricalBlogPostCategory {
	pristineLinks := make([]pristineLink, len(postLinks))
	for i, link := range postLinks {
		pristineLinks[i] = *NewPristineLink(&link)
	}
	return pristineHistoricalBlogPostCategory{
		Name:      strings.Clone(name),
		IsTop:     isTop,
		PostLinks: pristineLinks,
	}
}

func (c pristineHistoricalBlogPostCategory) Clone() *pristineHistoricalBlogPostCategory {
	return &pristineHistoricalBlogPostCategory{
		Name:      c.Name,
		IsTop:     c.IsTop,
		PostLinks: slices.Clone(c.PostLinks),
	}
}

func categoryCountsString(categories []pristineHistoricalBlogPostCategory) string {
	var sb strings.Builder
	for i, category := range categories {
		if i > 0 {
			sb.WriteString(", ")
		}
		if category.IsTop {
			sb.WriteRune('!')
		}
		sb.WriteString(category.Name)
		fmt.Fprintf(&sb, " (%d)", len(category.PostLinks))
	}
	return sb.String()
}

func PristineHistoricalBlogPostCategoriesUnwrap(
	categories []pristineHistoricalBlogPostCategory,
) []HistoricalBlogPostCategory {
	result := make([]HistoricalBlogPostCategory, len(categories))
	for i, category := range categories {
		links := make([]Link, len(category.PostLinks))
		for j, link := range category.PostLinks {
			links[j] = *link.Unwrap()
		}
		result[i] = HistoricalBlogPostCategory{
			Name:      category.Name,
			IsTop:     category.IsTop,
			PostLinks: links,
		}
	}
	return result
}
