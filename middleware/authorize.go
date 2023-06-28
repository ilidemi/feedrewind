package middleware

import (
	"feedrewind/util"
	"net/http"
)

func Authorize(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		if GetCurrentUser(r) == nil {
			redirectUrl := util.LoginPathWithRedirect(r)
			http.Redirect(w, r, redirectUrl, http.StatusFound)
			return
		}

		next.ServeHTTP(w, r)
	}
	return http.HandlerFunc(fn)
}
