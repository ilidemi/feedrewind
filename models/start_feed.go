package models

import (
	"feedrewind/crawler"
	"feedrewind/db/pgw"
	"feedrewind/models/mutil"
)

type StartFeedId int64

func StartFeed_MustCreateFetched(
	tx pgw.Queryable, startPageId StartPageId, discoveredFetchedFeed crawler.DiscoveredFetchedFeed,
) StartFeedId {
	id := StartFeedId(mutil.MustGenerateRandomId(tx, "start_feeds"))
	tx.MustExec(`
		insert into start_feeds (id, start_page_id, title, url, final_url, content)
		values ($1, $2, $3, $4, $5, $6)
	`, id, startPageId, discoveredFetchedFeed.Title, discoveredFetchedFeed.Url, discoveredFetchedFeed.FinalUrl,
		[]byte(discoveredFetchedFeed.Content))
	return id
}

func StartFeed_MustCreate(
	tx pgw.Queryable, startPageId StartPageId, discoveredFeed crawler.DiscoveredFeed,
) StartFeedId {
	id := StartFeedId(mutil.MustGenerateRandomId(tx, "start_feeds"))
	tx.MustExec(`
		insert into start_feeds (id, start_page_id, title, url, final_url, content)
		values ($1, $2, $3, $4, null, null)
		returning id
	`, id, startPageId, discoveredFeed.Title, discoveredFeed.Url)

	return id
}
