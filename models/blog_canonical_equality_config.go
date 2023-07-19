package models

import (
	"feedrewind/crawler"
	"feedrewind/db/pgw"
)

func BlogCanonicalEqualityConfig_MustGet(tx pgw.Queryable, blogId BlogId) crawler.CanonicalEqualityConfig {
	row := tx.QueryRow(`
		select same_hosts, expect_tumblr_paths from blog_canonical_equality_configs
		where blog_id = $1
	`, blogId)
	var sameHostsSlice []string
	var expectTumblrPaths bool
	err := row.Scan(&sameHostsSlice, &expectTumblrPaths)
	if err != nil {
		panic(err)
	}
	sameHosts := make(map[string]bool)
	for _, sameHost := range sameHostsSlice {
		sameHosts[sameHost] = true
	}
	return crawler.CanonicalEqualityConfig{
		SameHosts:         sameHosts,
		ExpectTumblrPaths: expectTumblrPaths,
	}
}
