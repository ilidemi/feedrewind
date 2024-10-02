package crawler

import (
	"feedrewind/log"
	"fmt"

	"github.com/rs/zerolog"
)

type Logger interface {
	Info(format string, args ...any)
	Warn(format string, args ...any)
	Error(format string, args ...any)
	Blob(key string, value []byte)
}

type ZeroLogger struct {
	Logger       log.Logger
	MaybeLogBlob func(key string, value []byte)
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

func (l *ZeroLogger) Blob(key string, value []byte) {
	if l.MaybeLogBlob != nil {
		l.MaybeLogBlob(key, value)
	} else {
		l.Logger.Info().Msgf("logging a blob skipped (%d bytes)", len(value))
	}
}

type DummyLogger struct {
	entries []logEntry
}

type logLevel int

const (
	logLevelInfo logLevel = iota
	logLevelWarn
	logLevelError
)

type logEntry struct {
	Level  logLevel
	Format string
	Args   []any
}

func NewDummyLogger() *DummyLogger {
	return &DummyLogger{
		entries: nil,
	}
}

func (d *DummyLogger) Info(format string, args ...any) {
	d.log(logLevelInfo, format, args)
}

func (d *DummyLogger) Warn(format string, args ...any) {
	d.log(logLevelWarn, format, args)
}

func (d *DummyLogger) Error(format string, args ...any) {
	d.log(logLevelError, format, args)
}

func (d *DummyLogger) Blob(key string, value []byte) {
	d.log(logLevelInfo, "logging a blob skipped (%d bytes)", len(value))
}

func (d *DummyLogger) log(level logLevel, format string, args ...any) {
	d.entries = append(d.entries, logEntry{
		Level:  level,
		Format: format,
		Args:   args,
	})
}

func (d *DummyLogger) Replay(logger log.Logger) {
	for _, entry := range d.entries {
		var event *zerolog.Event
		switch entry.Level {
		case logLevelInfo:
			event = logger.Info()
		case logLevelWarn:
			event = logger.Warn()
		case logLevelError:
			event = logger.Error()
		default:
			panic(fmt.Errorf("Unknown log level: %d", entry.Level))
		}
		event = event.Bool("replay", true)
		args := entry.Args[0].([]any)
		event.Msgf(entry.Format, args...)
	}
}
