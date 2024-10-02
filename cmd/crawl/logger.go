package crawl

import (
	"fmt"
	"os"
	"time"
)

type FileLogger struct {
	File *os.File
}

func (l *FileLogger) Info(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(l.File, "%s %s\n", time.Now().Format(time.RFC3339), msg)
}

func (l *FileLogger) Warn(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	line := fmt.Sprintf("WARN: %s", msg)
	fmt.Fprintf(l.File, "%s %s\n", time.Now().Format(time.RFC3339), line)
}

func (l *FileLogger) Error(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	line := fmt.Sprintf("ERROR: %s", msg)
	fmt.Fprintf(l.File, "%s %s\n", time.Now().Format(time.RFC3339), line)
}
