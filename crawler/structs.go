package crawler

import neturl "net/url"

type Link struct {
	Curi CanonicalUri
	Uri  *neturl.URL
	Url  string
}

type MaybeTitledLink struct {
	Link
	MaybeTitle *LinkTitle
}

type PageLink struct {
	Link
	Title string
}
