package models

import (
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

func UserSettings_Create(tx pgw.Queryable, userId UserId, timezone string) error {
	_, err := tx.Exec(`
		insert into user_settings(user_id, timezone, delivery_channel, version)
		values ($1, $2, null, 1)
	`, userId, timezone)
	return err
}

func UserSettings_GetById(tx pgw.Queryable, userId UserId) (*UserSettings, error) {
	row := tx.QueryRow(`
		select timezone, version, delivery_channel from user_settings where user_id = $1
	`, userId)
	var us UserSettings
	us.UserId = userId
	err := row.Scan(&us.Timezone, &us.Version, &us.DeliveryChannel)
	if err != nil {
		return nil, err
	}

	return &us, nil
}

func UserSettings_SaveTimezone(
	tx pgw.Queryable, userId UserId, timezone string, version int,
) error {
	_, err := tx.Exec(`
		update user_settings set timezone = $1, version = $2 where user_id = $3
	`, timezone, version, userId)
	return err
}

func UserSettings_SaveDeliveryChannel(
	tx pgw.Queryable, userId UserId, deliveryChannel DeliveryChannel, version int,
) error {
	_, err := tx.Exec(`
		update user_settings set delivery_channel = $1, version = $2 where user_id = $3
	`, deliveryChannel, version, userId)
	return err
}
