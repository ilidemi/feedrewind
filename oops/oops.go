package oops

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
)

type Error struct {
	Inner StackTracer
}

func (err *Error) Error() string {
	st := err.StackTrace()
	var b strings.Builder
	for i, frame := range st {
		if i > 0 {
			fmt.Fprint(&b, "\n")
		}
		frameText, _ := frame.MarshalText()
		fmt.Fprint(&b, string(frameText))
	}
	return fmt.Sprintf("%+v\b%s", err.Inner.Error(), b.String())
}

func (err *Error) Is(target error) bool {
	return errors.Is(err.Inner, target)
}

func (err *Error) As(target any) bool {
	return errors.As(err.Inner, target)
}

func (err *Error) StackTrace() errors.StackTrace {
	return err.Inner.StackTrace()
}

type StackTracer interface {
	Error() string
	StackTrace() errors.StackTrace
}

func Wrap(err error) error {
	if err == nil {
		return nil
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
