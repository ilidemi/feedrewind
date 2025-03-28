package middleware

import (
	"errors"
	"net/http"

	"feedrewind.com/oops"
	"feedrewind.com/templates"
	"feedrewind.com/util"
)

func Recoverer(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rvr := recover(); rvr != nil && rvr != http.ErrAbortHandler {
				err, ok := rvr.(error)
				if !ok {
					err = oops.Newf("%v", rvr)
				} else if oopsErr := (*oops.Error)(nil); !errors.As(err, &oopsErr) {
					err = oops.Wrap(err)
				}

				status := http.StatusInternalServerError
				var httpErr util.HttpError
				if errors.As(err, &httpErr) {
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
