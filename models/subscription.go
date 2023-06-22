package models

import (
	"context"
	"errors"
	"feedrewind/db"

	"github.com/jackc/pgx/v5"
)

type SubscriptionId int64

func Subscription_MustExists(id SubscriptionId) bool {
	ctx := context.Background()
	row := db.Conn.QueryRow(ctx, "select 1 from subscriptions where id = $1", id)
	var one int
	err := row.Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return false
	} else if err != nil {
		panic(err)
	}

	return true
}

func Subscription_MustSetUserId(id SubscriptionId, userId UserId) {
	ctx := context.Background()
	db.Conn.MustExec(ctx, "update subscriptions set user_id = $1 where id = $2", userId, id)
}
