package util

import (
	"context"
	"time"

	"feedrewind.com/log"

	"github.com/heroku/x/hmetrics"
)

func ReportHerokuMetrics(logger log.Logger) {
	var lastErrorTime time.Time
	// Adapted from https://github.com/heroku/x/blob/v0.1.0/hmetrics/onload/init.go
	var errorHandler hmetrics.ErrHandler = func(err error) error {
		if time.Since(lastErrorTime) < time.Hour {
			logger.Error().Err(err).
				Msgf("Error sending heroku metrics (previously errored at %v)", lastErrorTime)
		} else {
			logger.Info().Err(err).
				Msgf("Error sending heroku metrics (first error within an hour)")
		}
		lastErrorTime = time.Now().UTC()
		return nil
	}
	for backoff := int64(1); ; backoff++ {
		start := time.Now()
		err := hmetrics.Report(context.Background(), hmetrics.DefaultEndpoint, errorHandler) //nolint:gocritic
		if time.Since(start) > 5*time.Minute {
			backoff = 1
		}
		if err != nil {
			_ = errorHandler(err)
		}
		time.Sleep(time.Duration(backoff*10) * time.Second)
	}
}
