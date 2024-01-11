package middleware

import (
	"feedrewind/models"
	"feedrewind/util"
	"net/http"
	"reflect"
	"runtime"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
)

var handlerNamesOnce sync.Once
var handlerNames = map[string]string{}

func EmitVisit(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)

		rCtx := chi.RouteContext(r.Context())
		handlerNamesOnce.Do(func() {
			_ = chi.Walk(
				rCtx.Routes,
				func(
					method, route string, handler http.Handler,
					middlewares ...func(http.Handler) http.Handler,
				) error {
					route = strings.Replace(route, "/*/", "/", -1)
					route = strings.TrimSuffix(route, "//")
					route = strings.TrimSuffix(route, "/")
					key := method + " " + route
					fullName := runtime.FuncForPC(reflect.ValueOf(handler).Pointer()).Name()
					nameStart := strings.Index(fullName, ".") + 1
					name := fullName[nameStart:]
					handlerNames[key] = name
					return nil
				},
			)
		})

		logger := GetLogger(r)
		conn := GetDBConn(r)
		key := r.Method + " " + rCtx.RoutePattern()
		var handlerName string
		var ok bool
		if handlerName, ok = handlerNames[key]; !ok {
			handlerName = "N/A"
			logger.Warn().Msgf("Couldn't find handler name for key %q", key)
		}
		models.ProductEvent_DummyEmitOrLog(conn, r, true, "visit", map[string]any{
			"action":  handlerName,
			"referer": util.CollapseReferer(r),
		}, logger)
	}
	return http.HandlerFunc(fn)
}
