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

type PostmarkBounce struct {
	Id         int64
	BounceType string
	MessageId  PostmarkMessageId
}

func PostmarkBounce_GetById(tx pgw.Queryable, bounceId int64) (*PostmarkBounce, error) {
	row := tx.QueryRow(`
		select id, bounce_type, message_id
		from postmark_bounces
		where id = $1
	`, bounceId)
	var bounce PostmarkBounce
	err := row.Scan(&bounce.Id, &bounce.BounceType, &bounce.MessageId)
	if err != nil {
		return nil, err
	}

	return &bounce, nil
}

func PostmarkBounce_CreateIfNotExists(tx pgw.Queryable, bounce postmark.Bounce) error {
	_, err := tx.Exec(`
		insert into postmark_bounces (id, bounce_type, message_id, payload)
		values ($1, $2, $3, $4)
		on conflict do nothing
	`, bounce.ID, bounce.Type, bounce.MessageID, bounce)
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

func PostmarkBouncedUser_Create(tx pgw.Queryable, userId UserId, bounceId int64) error {
	_, err := tx.Exec(`
		insert into postmark_bounced_users (user_id, example_bounce_id)
		values ($1, $2)
	`, userId, bounceId)
	return err
}

type PostmarkMessageId string

type PostmarkMessageType string

const (
	PostmarkMessageSubInitial PostmarkMessageType = "sub_initial"
	PostmarkMessageSubFinal   PostmarkMessageType = "sub_final"
	PostmarkMessageSubPost    PostmarkMessageType = "sub_post"
)

func PostmarkMessage_GetAttemptCount(
	tx pgw.Queryable, messageType PostmarkMessageType, subscriptionId SubscriptionId,
	maybeSubscriptionPostId *SubscriptionPostId,
) (int, error) {
	var row *pgw.Row
	if maybeSubscriptionPostId != nil {
		row = tx.QueryRow(`
			select count(*)
			from postmark_messages
			where message_type = $1 and subscription_id = $2 and subscription_post_id = $3
		`, messageType, subscriptionId, *maybeSubscriptionPostId)
	} else {
		row = tx.QueryRow(`
			select count(*)
			from postmark_messages
			where message_type = $1 and subscription_id = $2 and subscription_post_id is null
		`, messageType, subscriptionId)
	}
	var count int
	err := row.Scan(&count)
	if err != nil {
		return 0, err
	}

	return count, nil
}
