package middleware

import (
	"feedrewind/oops"
	"feedrewind/util"
	"fmt"
	"net/http"
)

func Recoverer(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rvr := recover(); rvr != nil && rvr != http.ErrAbortHandler {
				err, ok := rvr.(error)
				if !ok {
					err = fmt.Errorf("%v", rvr)
				}

				status := http.StatusInternalServerError
				if httpErr, ok := err.(util.HttpError); ok {
					status = httpErr.Status
					err = httpErr.Inner
				}

				w.WriteHeader(status)
				sterr, ok := err.(*oops.Error)
				if !ok {
					sterr = oops.Wrap(err).(*oops.Error)
				}
				setError(r, sterr)
			}
		}()

		next.ServeHTTP(w, r)
	}

	return http.HandlerFunc(fn)
}
