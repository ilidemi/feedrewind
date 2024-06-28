package middleware

import (
	"context"
	"errors"
	"feedrewind/db/pgw"
	"feedrewind/log"
	"feedrewind/models"
	"feedrewind/oops"
	"feedrewind/util"
	"feedrewind/util/schedule"
	"net/http"
	"regexp"
	"slices"
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

		type FormKV struct {
			Key   string
			Value any
		}

		var formErr error
		var formKVs []FormKV
		if err := r.ParseForm(); err != nil {
			formErr = err
		} else if len(r.PostForm) != 0 {
			var keys []string
			for key := range r.PostForm {
				keys = append(keys, key)
			}
			slices.Sort(keys)

			for _, key := range keys {
				values := r.PostForm[key]

				var kv FormKV
				switch {
				case formFilter.MatchString(key):
					kv = FormKV{key, "*******"}
				case len(values) == 1:
					kv = FormKV{key, values[0]}
				default:
					arr := zerolog.Arr()
					for _, value := range values {
						arr.Str(value)
					}
					kv = FormKV{key, arr}
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
			cookies := r.Cookies()
			if len(cookies) > 0 {
				cookiesDict := zerolog.Dict()
				for _, cookie := range cookies {
					if cookie.Name == "_rss_catchup_rails_session" || cookie.Name == "feedrewind_session" {
						cookiesDict.Bool(cookie.Name, true)
					} else {
						cookiesDict.Str(cookie.Name, cookie.Value)
					}
				}
				event.Dict("cookies", cookiesDict)
			}
		}

		requestId := r.Header.Get("X-Request-ID")
		logger := &WebLogger{
			MaybeUserId: nil, // To be set by CurrentUser middleware
			RequestId:   requestId,
		}

		isStaticFile := strings.HasPrefix(r.URL.Path, util.StaticUrlPrefix)
		if !isStaticFile {
			logger.
				Info().
				Func(commonFields).
				Str("ip", util.UserIp(r)).
				Str("referrer", r.Referer()).
				Str("user-agent", r.UserAgent()).
				Msg("started")
		}

		var errorWrapper errorWrapper
		r = pgw.WithDBDuration(withCurrentUserData(withLogger(withErrorWrapper(r, &errorWrapper), logger)))

		defer func() {
			status := ww.Status()
			isCsrfError := errorWrapper.err != nil && errors.Is(errorWrapper.err, csrfValidationFailed)
			if (status/100 == 4 || status/100 == 5) &&
				status != http.StatusMethodNotAllowed &&
				status != http.StatusNotFound &&
				!isCsrfError {

				event := logger.
					Error().
					Func(commonFields)
				if errorWrapper.err != nil {
					event.Err(errorWrapper.err)
				}
				dbDuration := pgw.DbDuration(r.Context())
				if dbDuration > time.Second {
					logger.Warn().Func(commonFields).Msgf("Long db duration: %v", dbDuration)
				}
				event.
					Int("status", status).
					TimeDiff("duration", time.Now(), t1).
					Dur("db_duration", dbDuration).
					Msg("failed")
			} else if !isStaticFile {
				dbDuration := pgw.DbDuration(r.Context())
				if dbDuration > time.Second {
					logger.Warn().Func(commonFields).Msgf("Long db duration: %v", dbDuration)
				}
				event := logger.
					Info().
					Func(commonFields).
					Int("status", status).
					TimeDiff("duration", time.Now(), t1).
					Dur("db_duration", dbDuration)
				if isCsrfError {
					event = event.Str("omitted_error", csrfValidationFailed.Error())
				}
				event.Msg("completed")
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

type loggerKeyType struct{}

var loggerKey = &loggerKeyType{}

func withLogger(r *http.Request, logger *WebLogger) *http.Request {
	r = r.WithContext(context.WithValue(r.Context(), loggerKey, logger))
	return r
}

func GetLogger(r *http.Request) *WebLogger {
	return r.Context().Value(loggerKey).(*WebLogger)
}

func setLoggerUserId(r *http.Request, userId models.UserId) {
	logger := GetLogger(r)
	logger.MaybeUserId = &userId
}

type WebLogger struct {
	MaybeUserId *models.UserId
	RequestId   string
}

func (l *WebLogger) Info() *zerolog.Event {
	event := log.Base.Info()
	event = l.logWebCommon(event)
	return event
}

func (l *WebLogger) Warn() *zerolog.Event {
	event := log.Base.Warn()
	event = l.logWebCommon(event)
	return event
}

func (l *WebLogger) Error() *zerolog.Event {
	event := log.Base.Error()
	event = l.logWebCommon(event)
	return event
}

func (l *WebLogger) logWebCommon(event *zerolog.Event) *zerolog.Event {
	event = event.Timestamp()
	if schedule.IsSetUTCNowOverride() {
		event = event.Time("time_override", time.Time(schedule.UTCNow()))
	}
	if l.MaybeUserId != nil {
		event = event.Int64("user_id", int64(*l.MaybeUserId))
	}
	event = event.Str("request_id", l.RequestId)
	return event
}
