// Wraps zerolog logger, ensuring the timestamp goes in the beginning.
package log

import (
	"feedrewind/oops"
	"time"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

var Base zerolog.Logger

func init() {
	zerolog.ErrorStackMarshaler = marshalStack
	zerolog.DurationFieldInteger = true
	zerolog.TimeFieldFormat = time.RFC3339Nano
}

type Logger interface {
	Info() *zerolog.Event
	Warn() *zerolog.Event
	Error() *zerolog.Event
}

type BackgroundLogger struct{}

func (l *BackgroundLogger) Info() *zerolog.Event {
	event := Base.Info()
	event = l.logBackgroundCommon(event)
	return event
}

func (l *BackgroundLogger) Warn() *zerolog.Event {
	event := Base.Warn()
	event = l.logBackgroundCommon(event)
	return event
}

func (l *BackgroundLogger) Error() *zerolog.Event {
	event := Base.Error()
	event = l.logBackgroundCommon(event)
	return event
}

func (l *BackgroundLogger) logBackgroundCommon(event *zerolog.Event) *zerolog.Event {
	event = event.Timestamp().Str("logger", "background")
	return event
}

// Stack marshaling is copied from pkgerrors/stacktrace with quality of life modifications

type state struct {
	b []byte
}

// Write implement fmt.Formatter interface.
func (s *state) Write(b []byte) (n int, err error) {
	s.b = b
	return len(b), nil
}

// Width implement fmt.Formatter interface.
func (s *state) Width() (wid int, ok bool) {
	return 0, false
}

// Precision implement fmt.Formatter interface.
func (s *state) Precision() (prec int, ok bool) {
	return 0, false
}

// Flag implement fmt.Formatter interface.
func (s *state) Flag(c int) bool {
	return false
}

func frameField(f errors.Frame, s *state, c rune) string {
	f.Format(s, c)
	return string(s.b)
}

func marshalStack(err error) interface{} {
	sterr, ok := err.(*oops.Error)
	if !ok {
		return nil
	}
	st := sterr.StackTrace()
	var s state
	out := make([]map[string]string, 0, len(st))
	isOmittingMiddleware := false
	for i, frame := range st {
		source := frameField(frame, &s, 's')
		line := frameField(frame, &s, 'd')
		funcName := frameField(frame, &s, 'n')

		if i == 0 && source == "oops.go" {
			continue
		}

		if (i == 0 || i == 1) && source == "recoverer.go" && funcName == "Recoverer.func1.1" {
			continue
		}

		if (i == 1 || i == 2) && source == "panic.go" && funcName == "gopanic" {
			continue
		}

		if funcName == "(*Mux).ServeHTTP" {
			isOmittingMiddleware = false
		}

		if isOmittingMiddleware {
			continue
		}

		out = append(out, map[string]string{
			"source": source,
			"line":   line,
			"func":   funcName,
			"~":      "        ", // Visual spacing
		})
		if funcName == "(*Mux).routeHTTP" {
			// Omit middleware
			out = append(out, map[string]string{
				"middleware_omitted": "true",
				"~":                  "        ", // Visual spacing
			})
			isOmittingMiddleware = true
		}
	}

	return out
}
