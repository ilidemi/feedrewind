package middleware

import (
	"context"
	"feedrewind/db/pgw"
	"feedrewind/log"
	"feedrewind/util"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"
)

var formFilter *regexp.Regexp

func init() {
	formFilter = regexp.MustCompile("(passw|secret|token|_key|crypt|salt|certificate|otp|ssn)")
}

// Logger should come before Recoverer
func Logger(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		t1 := time.Now()

		path := r.URL.Path
		if r.URL.RawQuery != "" {
			path += "?" + r.URL.RawQuery
		}
		if r.URL.Fragment != "" {
			path += "#" + r.URL.EscapedFragment()
		}

		var formErr error
		var formDict *zerolog.Event
		if err := r.ParseForm(); err != nil {
			formErr = err
		} else if len(r.PostForm) != 0 {
			var keys []string
			for key := range r.PostForm {
				keys = append(keys, key)
			}
			sort.Strings(keys)

			formDict = zerolog.Dict()
			for _, key := range keys {
				values := r.PostForm[key]

				if formFilter.MatchString(key) {
					formDict.Str(key, "*******")
				} else if len(values) == 1 {
					formDict.Str(key, values[0])
				} else {
					arr := zerolog.Arr()
					for _, value := range values {
						arr.Str(value)
					}
					formDict.Array(key, arr)
				}
			}
		}

		commonFields := func(event *zerolog.Event) {
			event.
				Str("method", r.Method).
				Str("path", path)
			if formErr != nil {
				event.Str("form_err", formErr.Error())
			}
			if formDict != nil {
				event.Dict("form", formDict)
			}
		}

		isStaticFile := strings.HasPrefix(r.URL.Path, util.StaticUrlPrefix)
		if !isStaticFile {
			log.Info().
				Func(commonFields).
				Str("ip", util.UserIp(r)).
				Str("referrer", r.Referer()).
				Str("user-agent", r.UserAgent()).
				Msg("started")
		}

		var errorWrapper errorWrapper
		r = pgw.WithDBDuration(withErrorWrapper(r, &errorWrapper))

		defer func() {
			status := ww.Status()
			if status/100 == 4 || status/100 == 5 {
				event := log.Error().
					Func(commonFields)
				if errorWrapper.err != nil {
					event.Err(errorWrapper.err)
				}
				event.
					Int("status", status).
					TimeDiff("duration", time.Now(), t1).
					Dur("db_duration", pgw.DbDuration(r.Context())).
					Msg("failed")
			} else if !isStaticFile {
				log.Info().
					Func(commonFields).
					Int("status", status).
					TimeDiff("duration", time.Now(), t1).
					Dur("db_duration", pgw.DbDuration(r.Context())).
					Msg("completed")
			}
		}()
		next.ServeHTTP(ww, r)
	}
	return http.HandlerFunc(fn)
}

type errorWrapperKeyType struct{}

var errorWrapperKey = &errorWrapperKeyType{}

type errorWrapper struct {
	err error
}

func withErrorWrapper(r *http.Request, errorWrapper *errorWrapper) *http.Request {
	r = r.WithContext(context.WithValue(r.Context(), errorWrapperKey, errorWrapper))
	return r
}

func setError(r *http.Request, error error) {
	errorWrapper, _ := r.Context().Value(errorWrapperKey).(*errorWrapper)
	errorWrapper.err = error
}
