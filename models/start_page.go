package models

import (
	"feedrewind/crawler"
	"feedrewind/db/pgw"
)

type StartPageId int64

func StartPage_Create(
	tx pgw.Queryable, discoveredStartPage crawler.DiscoveredStartPage,
) (StartPageId, error) {
	row := tx.QueryRow(`
		insert into start_pages (url, final_url, content)
		values ($1, $2, $3)
		returning id
	`, discoveredStartPage.Url, discoveredStartPage.FinalUrl, []byte(discoveredStartPage.Content))
	var id StartPageId
	err := row.Scan(&id)
	if err != nil {
		return 0, err
	}

	return id, nil
}
