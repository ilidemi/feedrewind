package crawler

import (
	"fmt"

	"feedrewind.com/log"

	"github.com/rs/zerolog"
)

type Logger interface {
	Info(format string, args ...any)
	Warn(format string, args ...any)
	Error(format string, args ...any)
	Screenshot(url string, source string, data []byte)
}

type ZeroLogger struct {
	Logger                 log.Logger
	MaybeLogScreenshotFunc func(url string, source string, data []byte)
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

func (l *ZeroLogger) Screenshot(url string, source string, data []byte) {
	if l.MaybeLogScreenshotFunc != nil {
		l.MaybeLogScreenshotFunc(url, source, data)
	} else {
		l.Logger.Info().Msgf("Skipped screenshot: %s %s (%d bytes)", url, source, len(data))
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

func (d *DummyLogger) log(level logLevel, format string, args ...any) {
	d.entries = append(d.entries, logEntry{
		Level:  level,
		Format: format,
		Args:   args,
	})
}

func (d *DummyLogger) Screenshot(url string, source string, data []byte) {
	d.log(logLevelInfo, "Skipped screenshot: %s %s (%d bytes)", url, source, len(data))
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
