package middleware

import (
	"net/http"
	"reflect"
	"runtime"
	"strings"
	"sync"

	"feedrewind.com/models"
	"feedrewind.com/util"

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
					rawMethod, route string, handler http.Handler,
					middlewares ...func(http.Handler) http.Handler,
				) error {
					route = strings.ReplaceAll(route, "/*/", "/")
					route = strings.TrimSuffix(route, "//")
					route = strings.TrimSuffix(route, "/")
					var methods []string
					if rawMethod == "GET" {
						methods = []string{"GET", "HEAD"}
					} else {
						methods = []string{rawMethod}
					}
					for _, method := range methods {
						key := method + " " + route
						fullName := runtime.FuncForPC(reflect.ValueOf(handler).Pointer()).Name()
						nameStart := strings.Index(fullName, ".") + 1
						name := fullName[nameStart:]
						handlerNames[key] = name
					}
					return nil
				},
			)
		})

		logger := GetLogger(r)
		key := r.Method + " " + rCtx.RoutePattern()
		var handlerName string
		var ok bool
		if handlerName, ok = handlerNames[key]; !ok {
			handlerName = "N/A"
			logger.Warn().Msgf("Couldn't find handler name for key %q", key)
		}
		models.ProductEvent_QueueDummyEmit(r, true, "visit", map[string]any{
			"action":  handlerName,
			"referer": util.CollapseReferer(r),
		})
	}
	return http.HandlerFunc(fn)
}
