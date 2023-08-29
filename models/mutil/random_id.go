package mutil

import (
	"errors"
	"feedrewind/db/pgw"
	"feedrewind/util"

	"github.com/jackc/pgx/v5"
)

func RandomId(tx pgw.Queryable, tableName string) (int64, error) {
	for {
		id, err := util.RandomInt63()
		if err != nil {
			return 0, err
		}

		row := tx.QueryRow("select 1 from "+tableName+" where id = $1", id)
		var one int
		err = row.Scan(&one)
		if errors.Is(err, pgx.ErrNoRows) {
			return id, nil
		} else if err != nil {
			return 0, err
		}

		// continue
	}
}
