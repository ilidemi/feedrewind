package models

import "feedrewind/db/pgw"

type TypedBlogUrlResult string

const (
	TypedBlogUrlResultHardcoded             TypedBlogUrlResult = "hardcoded"
	TypedBlogUrlResultFeed                  TypedBlogUrlResult = "feed"
	TypedBlogUrlResultKnownUnsupported      TypedBlogUrlResult = "known_unsupported"
	TypedBlogUrlResultPageWithMultipleFeeds TypedBlogUrlResult = "page_with_multiple_feeds"
	TypedBlogUrlResultNotAUrl               TypedBlogUrlResult = "not_a_url"
	TypedBlogUrlResultNoFeeds               TypedBlogUrlResult = "no_feeds"
	TypedBlogUrlResultCouldNotReach         TypedBlogUrlResult = "could_not_reach"
	TypedBlogUrlResultBadFeed               TypedBlogUrlResult = "bad_feed"
)

func TypedBlogUrl_Create(
	tx pgw.Queryable, typedUrl string, strippedUrl string, source string, result TypedBlogUrlResult,
	maybeUserId *UserId,
) error {
	_, err := tx.Exec(`
		insert into typed_blog_urls (typed_url, stripped_url, source, result, user_id)
		values ($1, $2, $3, $4, $5)
	`, typedUrl, strippedUrl, source, result, maybeUserId)
	return err
}
