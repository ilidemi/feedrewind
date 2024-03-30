package middleware

import (
	"feedrewind/config"
	"net/http"
)

func RedirectHttpToHttps(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		if config.Cfg.Env == config.EnvProduction && r.Header.Get("x-forwarded-proto") != "https" {
			sslUrl := "https://" + r.Host + r.RequestURI
			http.Redirect(w, r, sslUrl, http.StatusPermanentRedirect)
			return
		}

		next.ServeHTTP(w, r)
	}
	return http.HandlerFunc(fn)
}
