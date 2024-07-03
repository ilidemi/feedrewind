package jobs

import (
	"bytes"
	"context"
	"feedrewind/config"
	"feedrewind/db/pgw"
	"feedrewind/models"
	"feedrewind/oops"
	"feedrewind/util/schedule"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/goccy/go-json"
)

func init() {
	registerJobNameFunc(
		"DispatchAmplitudeJob",
		func(ctx context.Context, id JobId, conn *pgw.Conn, args []any) error {
			if len(args) > 1 {
				return oops.Newf("Expected 0 or 1 arg, got %d: %v", len(args), args)
			}

			isManual := false
			if len(args) == 1 {
				var ok bool
				isManual, ok = args[0].(bool)
				if !ok {
					return oops.Newf("Failed to parse isManual (expected boolean): %v", args[0])
				}
			}

			return DispatchAmplitudeJob_Perform(ctx, conn, isManual)
		},
	)
}

func DispatchAmplitudeJob_PerformAt(tx pgw.Queryable, runAt schedule.Time) error {
	return performAt(tx, runAt, "DispatchAmplitudeJob", defaultQueue)
}

func DispatchAmplitudeJob_Perform(ctx context.Context, conn *pgw.Conn, isManual bool) error {
	logger := conn.Logger()
	eventsToDispatch, err := models.ProductEvent_GetNotDispatched(conn)
	if err != nil {
		return err
	}
	logger.Info().Msgf("Dispatching %d events", len(eventsToDispatch))

	dispatchedCount := 0
	botSkippedCount := 0
	botCounts := make(map[string]int)
	failedCount := 0
	for i, productEvent := range eventsToDispatch {
		if err := ctx.Err(); err != nil {
			return err
		}

		if i != 0 && i%100 == 0 {
			logger.Info().Msgf("Event %d", i)
		}

		if productEvent.MaybeBotName != nil && productEvent.MaybeUserProperties != nil {
			botIsAllowed, ok := productEvent.MaybeUserProperties["bot_is_allowed"].(bool)
			if ok && !botIsAllowed {
				botSkippedCount++
				botName := *productEvent.MaybeBotName
				botCounts[botName]++
				err := models.ProductEvent_MarkAsDispatched(conn, productEvent.Id, time.Now().UTC())
				if err != nil {
					return err
				}
				continue
			}
		}

		body := map[string]any{
			"api_key": config.Cfg.AmplitudeApiKey,
			"events": []map[string]any{{
				"user_id":          productEvent.ProductUserId,
				"event_type":       productEvent.EventType,
				"time":             fmt.Sprint(productEvent.CreatedAt.UnixMicro() / 1000),
				"event_properties": productEvent.MaybeEventProperties,
				"user_properties":  productEvent.MaybeUserProperties,
				"platform":         productEvent.MaybeBrowser,
				"os_name":          productEvent.MaybeOsName,
				"os_version":       productEvent.MaybeOsVersion,
				"ip":               productEvent.MaybeUserIp,
				"event_id":         productEvent.Id,
				"insert_id":        fmt.Sprint(productEvent.Id),
			}},
		}

		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return oops.Wrap(err)
		}

		req, err := http.NewRequest(
			http.MethodPost, "https://api.amplitude.com/2/httpapi", bytes.NewReader(bodyBytes),
		)
		if err != nil {
			return oops.Wrap(err)
		}
		req.Header.Add("Content-Type", "application/json")
		req.Header.Add("Accept", "*/*")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return oops.Wrap(err)
		}

		if resp.StatusCode == http.StatusOK {
			err := models.ProductEvent_MarkAsDispatched(conn, productEvent.Id, time.Now().UTC())
			if err != nil {
				return err
			}
			dispatchedCount++
		} else {
			respBody, err := io.ReadAll(resp.Body)
			if err != nil {
				logger.Error().Err(err).Msg("Error while reading Amplitude response body")
			}
			logger.Warn().Msgf(
				"Amplitude post failed for event %d: %d %s",
				productEvent.Id, resp.StatusCode, string(respBody),
			)
			failedCount++
		}
	}

	logger.Info().Msgf(
		"Dispatched %d events, skipped %d bot events (%v), failed %d events",
		dispatchedCount, botSkippedCount, botCounts, failedCount,
	)

	if !isManual {
		hourFromNow := schedule.UTCNow().Add(time.Hour)
		runAt := hourFromNow.BeginningOfHour()
		err := DispatchAmplitudeJob_PerformAt(conn, runAt)
		if err != nil {
			return err
		}
	}

	return nil
}
