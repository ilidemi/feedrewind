package models

import (
	"errors"
	"feedrewind/db/pgw"

	"github.com/jackc/pgx/v5"
)

func PostmarkBouncedUser_Exists(tx pgw.Queryable, userId UserId) (bool, error) {
	row := tx.QueryRow("select 1 from postmark_bounced_users where user_id = $1", userId)
	var one int
	err := row.Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, err
	}

	return true, nil
}
