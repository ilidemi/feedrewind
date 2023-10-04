package models

import (
	"errors"
	"feedrewind/db/pgw"

	"github.com/jackc/pgx/v5"
	"github.com/mrz1836/postmark"
)

func PostmarkBounce_Exists(tx pgw.Queryable, bounceId int64) (bool, error) {
	row := tx.QueryRow("select 1 from postmark_bounces where id = $1", bounceId)
	var one int
	err := row.Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, err
	}

	return true, nil
}

func PostmarkBounce_Create(tx pgw.Queryable, bounce postmark.Bounce, bounceStr string) error {
	_, err := tx.Exec(`
		insert into postmark_bounces (id, bounce_type, message_id, payload)
		values ($1, $2, $3, $4)
	`, bounce.ID, bounce.Type, bounce.MessageID, bounceStr)
	return err
}

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
