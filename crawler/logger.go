package crawler

import (
	"feedrewind/log"
)

type Logger interface {
	Info(format string, args ...any)
	Warn(format string, args ...any)
	Error(format string, args ...any)
}

type ZeroLogger struct {
	Logger log.Logger
}

func (l *ZeroLogger) Info(format string, args ...any) {
	l.Logger.Info().Msgf(format, args...)
}

func (l *ZeroLogger) Warn(format string, args ...any) {
	l.Logger.Warn().Msgf(format, args...)
}

func (l *ZeroLogger) Error(format string, args ...any) {
	l.Logger.Error().Msgf(format, args...)
}

type DummyLogger struct{}

func (*DummyLogger) Info(format string, args ...any)  {}
func (*DummyLogger) Warn(format string, args ...any)  {}
func (*DummyLogger) Error(format string, args ...any) {}
