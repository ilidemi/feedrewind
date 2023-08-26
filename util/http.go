package util

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

type HttpError struct {
	Status int
	Inner  error
}

func (e HttpError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.Status, e.Inner.Error())
}

func HttpPanic(status int, text string) {
	panic(HttpError{
		Status: status,
		Inner:  errors.New(text),
	})
}

func EnsureParamStr(r *http.Request, name string) string {
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

func EnsureParamInt64(r *http.Request, name string) int64 {
	str := EnsureParamStr(r, name)
	val, err := strconv.ParseInt(str, 10, 64)
	if err != nil {
		panic(HttpError{
			Status: http.StatusBadRequest,
			Inner:  fmt.Errorf("couldn't read int64: %s", str),
		})
	}
	return val
}

func EnsureParamInt(r *http.Request, name string) int {
	str := EnsureParamStr(r, name)
	val64, err := strconv.ParseInt(str, 10, 64)
	if err != nil {
		panic(HttpError{
			Status: http.StatusBadRequest,
			Inner:  fmt.Errorf("couldn't read int: %s", str),
		})
	}
	return int(val64)
}

func EnsureParamFloat64(r *http.Request, name string) float64 {
	str := EnsureParamStr(r, name)
	val, err := strconv.ParseFloat(str, 64)
	if err != nil {
		panic(HttpError{
			Status: http.StatusBadRequest,
			Inner:  fmt.Errorf("couldn't read float64: %s", str),
		})
	}
	return val
}

// Route params return bool ok instead of BadRequest because we may want to manually redirect the user
func URLParamInt64(r *http.Request, name string) (int64, bool) {
	str := chi.URLParam(r, name)
	result, err := strconv.ParseInt(str, 10, 64)
	if err != nil {
		return 0, false
	}

	return result, true
}

func URLParamStr(r *http.Request, name string) string {
	return chi.URLParam(r, name)
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

func SetSessionCookie(w http.ResponseWriter, name string, value string) {
	http.SetCookie(w, &http.Cookie{
		Name:  name,
		Value: value,
		Path:  "/",
	})
}

func DeleteCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:    name,
		Value:   "",
		Expires: time.Unix(0, 0),
		Path:    "/",
	})
}

func UserIp(r *http.Request) string {
	return r.Header.Get("X-Forwarded-For")
}
