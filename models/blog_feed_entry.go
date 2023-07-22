package models

import "feedrewind/db/pgw"

func blogDiscardedFeedEntry_ListUrls(tx pgw.Queryable, blogId BlogId) (map[string]bool, error) {
	return blogFeedEntry_ListUrls(tx, "blog_discarded_feed_entries", blogId)
}

func blogMissingFromFeedEntry_ListUrls(tx pgw.Queryable, blogId BlogId) (map[string]bool, error) {
	return blogFeedEntry_ListUrls(tx, "blog_missing_from_feed_entries", blogId)
}

func blogFeedEntry_ListUrls(tx pgw.Queryable, table string, blogId BlogId) (map[string]bool, error) {
	rows, err := tx.Query("select url from "+table+" where blog_id = $1", blogId)
	if err != nil {
		return nil, err
	}
	urls := make(map[string]bool)
	for rows.Next() {
		var url string
		err := rows.Scan(&url)
		if err != nil {
			return nil, err
		}
		urls[url] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return urls, nil
}
