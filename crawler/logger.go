package crawler

import (
	"feedrewind/log"
	"net/http"
)

type Logger interface {
	Info(format string, args ...any)
	Warn(format string, args ...any)
	Error(format string, args ...any)
}

type ZeroLogger struct {
	Req *http.Request
}

func (l *ZeroLogger) Info(format string, args ...any) {
	log.Info(l.Req).Msgf(format, args...)
}

func (l *ZeroLogger) Warn(format string, args ...any) {
	log.Warn(l.Req).Msgf(format, args...)
}

func (l *ZeroLogger) Error(format string, args ...any) {
	log.Error(l.Req).Msgf(format, args...)
}

type DummyLogger struct{}

func (*DummyLogger) Info(format string, args ...any)  {}
func (*DummyLogger) Warn(format string, args ...any)  {}
func (*DummyLogger) Error(format string, args ...any) {}
