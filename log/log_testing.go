//go:build testing

package log

import "github.com/rs/zerolog"

func init() {
	logger = zerolog.New(nil)
}
