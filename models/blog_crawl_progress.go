package models

import "feedrewind/db/pgw"

func BlogCrawlProgress_MustCreate(tx pgw.Queryable, blogId BlogId) {
	tx.MustExec(`
		insert into blog_crawl_progresses (blog_id, epoch) values ($1, 0)
	`, blogId)
}
