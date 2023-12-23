package models

import (
	"feedrewind/db/pgw"
)

func AdminTelemetry_Create(tx pgw.Queryable, key string, value float64, extra map[string]any) error {
	_, err := tx.Exec(`
		insert into admin_telemetries (key, value, extra) values ($1, $2, $3)
	`, key, value, extra)
	return err
}
