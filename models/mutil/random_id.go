package mutil

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"feedrewind/db/pgw"

	"github.com/jackc/pgx/v5"
)

func MustGenerateRandomId(tx pgw.Queryable, tableName string) int64 {
	buf := make([]byte, 8)
	for {
		_, err := rand.Read(buf)
		if err != nil {
			panic(err)
		}
		uId := binary.LittleEndian.Uint64(buf)
		id := int64(uId & ((1 << 63) - 1))
		if id == 0 {
			continue
		}

		row := tx.QueryRow("select 1 from "+tableName+" where id = $1", id)
		var one int
		err = row.Scan(&one)
		if errors.Is(err, pgx.ErrNoRows) {
			return id
		} else if err != nil {
			panic(err)
		}

		// continue
	}
}
