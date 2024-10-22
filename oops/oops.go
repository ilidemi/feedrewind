package oops

import (
	"fmt"
	"io"
	"strings"

	"github.com/pkg/errors"
)

type StackTracer interface {
	Error() string
	StackTrace() errors.StackTrace
}

type Error struct {
	Inner StackTracer
}

func Wrap(err error) error {
	if err == nil {
		return nil
	}

	if _, ok := err.(*Error); ok {
		return err
	}

	return &Error{
		Inner: errors.WithStack(err).(StackTracer),
	}
}

func Wrapf(err error, format string, a ...any) error {
	inner := errors.Wrapf(err, format, a...)
	return &Error{
		Inner: errors.WithStack(inner).(StackTracer),
	}
}

func New(message string) error {
	err := errors.New(message)
	return &Error{
		Inner: errors.WithStack(err).(StackTracer),
	}
}

func Newf(format string, a ...any) error {
	err := fmt.Errorf(format, a...)
	return &Error{
		Inner: errors.WithStack(err).(StackTracer),
	}
}

func (err *Error) Error() string {
	return err.Inner.Error()
}

func (err *Error) Unwrap() error {
	return err.Inner
}

func (err *Error) StackTrace() errors.StackTrace {
	return err.Inner.StackTrace()
}

func (err *Error) FullString() string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "Error: %v", err.Inner)
	err.Inner.StackTrace().Format(&state{Writer: &builder}, 'v')
	fmt.Fprintln(&builder)
	return builder.String()
}

type state struct {
	Writer io.Writer
}

// Write implement fmt.Formatter interface.
func (s *state) Write(b []byte) (n int, err error) {
	return s.Writer.Write(b)
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
	return c == '+'
}
