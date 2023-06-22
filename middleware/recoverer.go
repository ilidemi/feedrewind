package middleware

import (
	"feedrewind/util"
	"fmt"
	"net/http"

	"github.com/pkg/errors"
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
				setError(r, errors.WithStack(err))
			}
		}()

		next.ServeHTTP(w, r)
	}

	return http.HandlerFunc(fn)
}
