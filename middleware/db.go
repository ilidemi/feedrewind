package middleware

import (
	"context"
	"feedrewind/db"
	"feedrewind/db/pgw"
	"net/http"
)

func DB(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		logger := GetLogger(r)
		pool := db.RootPool.Child(r.Context(), logger)
		next.ServeHTTP(w, withDBPool(r, pool))
	}
	return http.HandlerFunc(fn)
}

type dbPoolKeyType struct{}

var dbPoolKey = &dbPoolKeyType{}

func withDBPool(r *http.Request, pool *pgw.Pool) *http.Request {
	r = r.WithContext(context.WithValue(r.Context(), dbPoolKey, pool))
	return r
}

func GetDBPool(r *http.Request) *pgw.Pool {
	return r.Context().Value(dbPoolKey).(*pgw.Pool)
}
