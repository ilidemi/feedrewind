package models

import (
	"context"
	"errors"
	"feedrewind/db/pgw"

	"github.com/jackc/pgx/v5"
)

func PostmarkBouncedUser_MustExists(ctx context.Context, tx pgw.Queryable, userId UserId) bool {
	row := tx.QueryRow(ctx, "select 1 from postmark_bounced_users where user_id = $1", userId)
	var one int
	err := row.Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return false
	} else if err != nil {
		panic(err)
	}

	return true
}
