package middleware

import (
	"feedrewind/oops"
	"feedrewind/templates"
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

				if r.Header.Get("Connection") != "Upgrade" {
					w.WriteHeader(status)

					if status == http.StatusInternalServerError {
						type InternalServerErrorResult struct {
							Title string
						}
						templates.MustWrite(w, "misc/500", InternalServerErrorResult{
							Title: "FeedRewind",
						})
					}
				}

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
