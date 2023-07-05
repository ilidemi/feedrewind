package models

import (
	"feedrewind/db/pgw"
)

func UserRss_MustCreate(tx pgw.Queryable, userId UserId, body string) {
	tx.MustExec(`
		insert into user_rsses (user_id, body)
		values ($1, $2)
	`, userId, body)
}
