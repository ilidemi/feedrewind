package models

import (
	"feedrewind/db/pgw"
	"feedrewind/oops"
)

func TestSingleton_GetValue(tx pgw.Queryable, key string) (*string, error) {
	row := tx.QueryRow(`select value from test_singletons where key = $1`, key)
	var maybeValue *string
	err := row.Scan(&maybeValue)
	return maybeValue, err
}

func TestSingleton_SetValue(tx pgw.Queryable, key, value string) error {
	tag, err := tx.Exec(`
		update test_singletons
		set value = $1
		where key = $2
	`, value, key)
	if err != nil {
		return err
	}

	rowsAffected := tag.RowsAffected()
	if rowsAffected != 1 {
		return oops.Newf("Expected to update 1 row, got %d", rowsAffected)
	}

	return nil
}
