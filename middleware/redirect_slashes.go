package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

func RedirectSlashes(excludePrefix string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			var path string
			rctx := chi.RouteContext(r.Context())
			if rctx != nil && rctx.RoutePath != "" {
				path = rctx.RoutePath
			} else {
				path = r.URL.Path
			}
			if len(path) > 1 && path[len(path)-1] == '/' &&
				!(strings.HasPrefix(path, excludePrefix) && path != excludePrefix) {

				if r.URL.RawQuery != "" {
					path = fmt.Sprintf("%s?%s", path[:len(path)-1], r.URL.RawQuery)
				} else {
					path = path[:len(path)-1]
				}
				redirectURL := fmt.Sprintf("//%s%s", r.Host, path)
				http.Redirect(w, r, redirectURL, http.StatusMovedPermanently)
				return
			}
			next.ServeHTTP(w, r)
		}
		return http.HandlerFunc(fn)
	}
}
