package models

import (
	"context"
	"feedrewind/db/pgw"
)

func UserSettings_MustCreate(tx pgw.Queryable, userId UserId, timezone string) {
	ctx := context.Background()
	tx.MustExec(ctx, `
		insert into user_settings(user_id, timezone, delivery_channel, version)
		values ($1, $2, null, 1)
	`, userId, timezone)
}
