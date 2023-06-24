package models

import (
	"context"
	"feedrewind/db/pgw"
)

func UserRss_MustCreate(tx pgw.Queryable, userId UserId, body string) {
	ctx := context.Background()
	tx.MustExec(ctx, `
		insert into user_rsses (user_id, body)
		values ($1, $2)
	`, userId, body)
}
