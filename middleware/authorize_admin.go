package middleware

import (
	"feedrewind/config"
	"net/http"
)

func AuthorizeAdmin(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		if config.Cfg.Env == config.EnvDevelopment || config.Cfg.Env == config.EnvTesting {
			next.ServeHTTP(w, r)
		} else {
			currentUser := GetCurrentUser(r)
			if currentUser == nil || !config.Cfg.AdminUserIds[int64(currentUser.Id)] {
				w.WriteHeader(http.StatusNotFound)
				return
			}

			next.ServeHTTP(w, r)
		}
	}
	return http.HandlerFunc(fn)
}
