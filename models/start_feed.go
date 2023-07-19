package models

import (
	"feedrewind/crawler"
	"feedrewind/db/pgw"
	"feedrewind/models/mutil"
)

type StartFeedId int64

type StartFeed struct {
	Id              StartFeedId
	Title           string
	Url             string
	MaybeParsedFeed *crawler.ParsedFeed
}

func StartFeed_MustCreateFetched(
	tx pgw.Queryable, startPageId StartPageId, discoveredFetchedFeed crawler.DiscoveredFetchedFeed,
) StartFeed {
	id := StartFeedId(mutil.MustGenerateRandomId(tx, "start_feeds"))
	tx.MustExec(`
		insert into start_feeds (id, start_page_id, title, url, final_url, content)
		values ($1, $2, $3, $4, $5, $6)
	`, id, startPageId, discoveredFetchedFeed.Title, discoveredFetchedFeed.Url,
		discoveredFetchedFeed.FinalUrl, []byte(discoveredFetchedFeed.Content))

	return StartFeed{
		Id:              id,
		Title:           discoveredFetchedFeed.Title,
		Url:             discoveredFetchedFeed.FinalUrl,
		MaybeParsedFeed: &discoveredFetchedFeed.ParsedFeed,
	}
}

func StartFeed_MustCreate(
	tx pgw.Queryable, startPageId StartPageId, discoveredFeed crawler.DiscoveredFeed,
) StartFeed {
	id := StartFeedId(mutil.MustGenerateRandomId(tx, "start_feeds"))
	tx.MustExec(`
		insert into start_feeds (id, start_page_id, title, url, final_url, content)
		values ($1, $2, $3, $4, null, null)
		returning id
	`, id, startPageId, discoveredFeed.Title, discoveredFeed.Url)

	return StartFeed{
		Id:              id,
		Title:           discoveredFeed.Title,
		Url:             discoveredFeed.Url,
		MaybeParsedFeed: nil,
	}
}

func StartFeed_MustGetUnfetched(tx pgw.Queryable, id StartFeedId) StartFeed {
	row := tx.QueryRow(`select title, url from start_feeds where id = $1`, id)
	var title, url string
	err := row.Scan(&title, &url)
	if err != nil {
		panic(err)
	}

	return StartFeed{
		Id:              id,
		Title:           title,
		Url:             url,
		MaybeParsedFeed: nil,
	}
}

func StartFeed_MustUpdateFetched(
	tx pgw.Queryable, startFeed StartFeed, finalUrl string, content string, parsedFeed *crawler.ParsedFeed,
) StartFeed {
	tx.MustExec(`
		update start_feeds set final_url = $1, content = $2 where id = $3
	`, finalUrl, content, startFeed.Id)
	return StartFeed{
		Id:              startFeed.Id,
		Title:           startFeed.Title,
		Url:             finalUrl,
		MaybeParsedFeed: parsedFeed,
	}
}
