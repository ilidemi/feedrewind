package util

import (
	"fmt"
	"net/http"
	"time"
)

func EnsureParam(r *http.Request, name string) string {
	if r.Form == nil {
		panic("call r.ParseForm() before util.EnsureParam()")
	}

	value, ok := r.Form[name]
	if !ok {
		panic(HttpError{
			Status: http.StatusBadRequest,
			Inner:  fmt.Errorf("missing in form: %s", name),
		})
	}

	return value[0]
}

func FindCookie(r *http.Request, name string) (string, bool) {
	for _, cookie := range r.Cookies() {
		if cookie.Name != name {
			continue
		}

		return cookie.Value, true
	}

	return "", false
}

func DeleteCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:    name,
		Value:   "",
		Expires: time.Unix(0, 0),
	})
}

func UserIp(r *http.Request) string {
	return r.Header.Get("X-Forwarded-For")
}
