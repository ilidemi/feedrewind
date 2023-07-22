package models

import "feedrewind/db/pgw"

func blogCrawlProgress_Create(tx pgw.Queryable, blogId BlogId) error {
	_, err := tx.Exec(`
		insert into blog_crawl_progresses (blog_id, epoch) values ($1, 0)
	`, blogId)
	return err
}
