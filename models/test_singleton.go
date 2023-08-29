package models

import "feedrewind/db/pgw"

func TestSingleton_GetValue(tx pgw.Queryable, key string) (string, error) {
	row := tx.QueryRow(`select value from test_singletons where key = $1`, key)
	var value string
	err := row.Scan(&value)
	return value, err
}
