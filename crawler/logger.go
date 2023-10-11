package crawler

import (
	"feedrewind/log"
	"fmt"
	"net/http"
	"os"
	"time"
)

type Logger interface {
	Info(format string, args ...any)
}

type ZeroLogger struct {
	Req *http.Request
}

func (l *ZeroLogger) Info(format string, args ...any) {
	log.Info(l.Req).Msgf(format, args...)
}

type FileLogger struct {
	File *os.File
}

func (l *FileLogger) Info(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(l.File, "%s %s\n", time.Now().Format(time.RFC3339), msg)
}

type DummyLogger struct{}

func (*DummyLogger) Info(format string, args ...any) {}
