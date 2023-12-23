//go:build testing

package log

import "github.com/rs/zerolog"

func init() {
	Base = zerolog.New(nil)
}
