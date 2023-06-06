// Wraps zerolog logger, ensuring the timestamp goes in the beginning.
package log

import (
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/pkgerrors"
)

var logger zerolog.Logger

func init() {
	zerolog.ErrorStackMarshaler = pkgerrors.MarshalStack
	zerolog.DurationFieldInteger = true
	zerolog.TimeFieldFormat = time.RFC3339Nano
	logger = zerolog.New(os.Stderr).With().Stack().Logger()
}

func Info() *zerolog.Event {
	return logger.Info().Timestamp()
}

func Warn() *zerolog.Event {
	return logger.Warn().Timestamp()
}

func Error() *zerolog.Event {
	return logger.Error().Timestamp()
}
