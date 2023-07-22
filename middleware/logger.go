package middleware

import (
	"context"
	"feedrewind/db/pgw"
	"feedrewind/log"
	"feedrewind/oops"
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

		type formKV struct {
			Key   string
			Value any
		}

		var formErr error
		var formKVs []formKV
		if err := r.ParseForm(); err != nil {
			formErr = err
		} else if len(r.PostForm) != 0 {
			var keys []string
			for key := range r.PostForm {
				keys = append(keys, key)
			}
			sort.Strings(keys)

			for _, key := range keys {
				values := r.PostForm[key]

				var kv formKV
				if formFilter.MatchString(key) {
					kv = formKV{key, "*******"}
				} else if len(values) == 1 {
					kv = formKV{key, values[0]}
				} else {
					arr := zerolog.Arr()
					for _, value := range values {
						arr.Str(value)
					}
					kv = formKV{key, arr}
				}
				formKVs = append(formKVs, kv)
			}
		}

		commonFields := func(event *zerolog.Event) {
			event.
				Str("method", r.Method).
				Str("path", path)
			if formErr != nil {
				event.Str("form_err", formErr.Error())
			}
			if len(formKVs) > 0 {
				formDict := zerolog.Dict()
				for _, kv := range formKVs {
					formDict.Any(kv.Key, kv.Value)
				}
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
				event := log.Error().Func(commonFields)
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
	err *oops.Error
}

func withErrorWrapper(r *http.Request, errorWrapper *errorWrapper) *http.Request {
	r = r.WithContext(context.WithValue(r.Context(), errorWrapperKey, errorWrapper))
	return r
}

func setError(r *http.Request, error *oops.Error) {
	errorWrapper, _ := r.Context().Value(errorWrapperKey).(*errorWrapper)
	errorWrapper.err = error
}
