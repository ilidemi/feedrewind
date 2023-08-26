//go:build !testing

package log

import (
	"os"

	"github.com/rs/zerolog"
)

func init() {
	logger = zerolog.New(os.Stderr).With().Stack().Logger()
}
