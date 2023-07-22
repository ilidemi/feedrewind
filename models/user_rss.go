package models

import (
	"feedrewind/db/pgw"
)

func UserRss_Create(tx pgw.Queryable, userId UserId, body string) error {
	_, err := tx.Exec(`
		insert into user_rsses (user_id, body)
		values ($1, $2)
	`, userId, body)
	return err
}
