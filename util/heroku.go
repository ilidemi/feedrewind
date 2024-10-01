package util

import (
	"context"
	"feedrewind/log"
	"time"

	"github.com/heroku/x/hmetrics"
)

func ReportHerokuMetrics(logger log.Logger) {
	// Adapted from https://github.com/heroku/x/blob/v0.1.0/hmetrics/onload/init.go
	var errorHandler hmetrics.ErrHandler = func(err error) error {
		logger.Error().Err(err).Msg("Error sending heroku metrics")
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
