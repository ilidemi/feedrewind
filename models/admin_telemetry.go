package models

import (
	"feedrewind/db/pgw"
	"time"
)

func AdminTelemetry_Create(tx pgw.Queryable, key string, value float64, extra map[string]any) error {
	_, err := tx.Exec(`
		insert into admin_telemetries (key, value, extra) values ($1, $2, $3)
	`, key, value, extra)
	return err
}

type AdminTelemetry struct {
	Key       string
	Value     float64
	Extra     map[string]any
	CreatedAt time.Time
}

func AdminTelemetry_GetSince(tx pgw.Queryable, since time.Time) ([]AdminTelemetry, error) {
	rows, err := tx.Query(`
		select key, value, extra, created_at from admin_telemetries
		where created_at > $1
		order by created_at asc
	`, since)
	if err != nil {
		return nil, err
	}

	var result []AdminTelemetry
	for rows.Next() {
		var t AdminTelemetry
		err := rows.Scan(&t.Key, &t.Value, &t.Extra, &t.CreatedAt)
		if err != nil {
			panic(err)
		}

		result = append(result, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}
