package models

import (
	"context"
	"feedrewind/db/pgw"
)

type DeliveryChannel string

const (
	DeliveryChannelSingleFeed    = "single_feed"
	DeliveryChannelMultipleFeeds = "multiple_feeds"
	DeliveryChannelEmail         = "email"
)

type UserSettings struct {
	UserId          UserId
	Timezone        string
	Version         int
	DeliveryChannel *DeliveryChannel
}

func UserSettings_MustCreate(ctx context.Context, tx pgw.Queryable, userId UserId, timezone string) {
	tx.MustExec(ctx, `
		insert into user_settings(user_id, timezone, delivery_channel, version)
		values ($1, $2, null, 1)
	`, userId, timezone)
}

func UserSettings_MustGetById(ctx context.Context, tx pgw.Queryable, userId UserId) UserSettings {
	row := tx.QueryRow(ctx, `
		select timezone, version, delivery_channel from user_settings where user_id = $1
	`, userId)
	var us UserSettings
	us.UserId = userId
	err := row.Scan(&us.Timezone, &us.Version, &us.DeliveryChannel)
	if err != nil {
		panic(err)
	}
	return us
}

func UserSettings_MustSaveTimezone(
	ctx context.Context, tx pgw.Queryable, userId UserId, timezone string, version int,
) {
	tx.MustExec(ctx, `
		update user_settings set timezone = $1, version = $2 where user_id = $3
	`, timezone, version, userId)
}

func UserSettings_MustSaveDeliveryChannel(
	ctx context.Context, tx pgw.Queryable, userId UserId, deliveryChannel DeliveryChannel, version int,
) {
	tx.MustExec(ctx, `
		update user_settings set delivery_channel = $1, version = $2 where user_id = $3
	`, deliveryChannel, version, userId)
}
