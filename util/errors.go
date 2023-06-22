package util

import (
	"errors"
	"fmt"
)

type HttpError struct {
	Status int
	Inner  error
}

func (e HttpError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.Status, e.Inner.Error())
}

func HttpPanic(status int, text string) {
	panic(HttpError{
		Status: status,
		Inner:  errors.New(text),
	})
}
