package models

import "feedrewind/db/pgw"

func BlogDiscardedFeedEntry_MustListUrls(tx pgw.Queryable, blogId BlogId) map[string]bool {
	return blogFeedEntry_MustListUrls(tx, "blog_discarded_feed_entries", blogId)
}

func BlogMissingFromFeedEntry_MustListUrls(tx pgw.Queryable, blogId BlogId) map[string]bool {
	return blogFeedEntry_MustListUrls(tx, "blog_missing_from_feed_entries", blogId)
}

func blogFeedEntry_MustListUrls(tx pgw.Queryable, table string, blogId BlogId) map[string]bool {
	rows, err := tx.Query("select url from "+table+" where blog_id = $1", blogId)
	if err != nil {
		panic(err)
	}
	urls := make(map[string]bool)
	for rows.Next() {
		var url string
		err := rows.Scan(&url)
		if err != nil {
			panic(err)
		}
		urls[url] = true
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}
	return urls
}
