package models

import "feedrewind/db/pgw"

func UserRss_GetBody(tx pgw.Queryable, userId UserId) (string, error) {
	row := tx.QueryRow(`select body from user_rsses where user_id = $1`, userId)
	var body string
	err := row.Scan(&body)
	return body, err
}

func UserRss_Upsert(tx pgw.Queryable, userId UserId, body string) error {
	_, err := tx.Exec(`
		insert into user_rsses (user_id, body) values ($1, $2)
		on conflict (user_id)
		do update set body = $2
	`, userId, body)
	return err
}

func SubscriptionRss_GetBody(tx pgw.Queryable, subscriptionId SubscriptionId) (string, error) {
	row := tx.QueryRow(`select body from subscription_rsses where subscription_id = $1`, subscriptionId)
	var body string
	err := row.Scan(&body)
	return body, err
}

func SubscriptionRss_Upsert(tx pgw.Queryable, subscriptionId SubscriptionId, body string) error {
	_, err := tx.Exec(`
		insert into subscription_rsses (subscription_id, body) values ($1, $2)
		on conflict (subscription_id)
		do update set body = $2
	`, subscriptionId, body)
	return err
}
