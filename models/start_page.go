package models

import (
	"feedrewind/crawler"
	"feedrewind/db/pgw"
)

type StartPageId int64

func StartPage_MustCreate(tx pgw.Queryable, discoveredStartPage crawler.DiscoveredStartPage) StartPageId {
	row := tx.QueryRow(`
		insert into start_pages (url, final_url, content)
		values ($1, $2, $3)
		returning id
	`, discoveredStartPage.Url, discoveredStartPage.FinalUrl, discoveredStartPage.Content)
	var id StartPageId
	err := row.Scan(&id)
	if err != nil {
		panic(err)
	}

	return id
}
