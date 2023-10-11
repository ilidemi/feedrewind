package middleware

import (
	"context"
	"feedrewind/db"
	"feedrewind/db/pgw"
	"net/http"
)

func DB(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		conn, err := db.Pool.Acquire(r)
		if err != nil {
			panic(err)
		}
		defer conn.Release()

		next.ServeHTTP(w, withDBConn(r, conn))
	}
	return http.HandlerFunc(fn)
}

type dbConnKeyType struct{}

var dbConnKey = &dbConnKeyType{}

func withDBConn(r *http.Request, conn *pgw.Conn) *http.Request {
	r = r.WithContext(context.WithValue(r.Context(), dbConnKey, conn))
	return r
}

func GetDBConn(r *http.Request) *pgw.Conn {
	return r.Context().Value(dbConnKey).(*pgw.Conn)
}
