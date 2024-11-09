package jobs

import (
	"context"
	"fmt"
	"time"

	"feedrewind.com/config"
	"feedrewind.com/db/pgw"
	"feedrewind.com/models"
	"feedrewind.com/oops"
	"feedrewind.com/util"
	"feedrewind.com/util/schedule"
)

type TimeTravelJobAction string

const (
	TimeTravelJobTravelTo   TimeTravelJobAction = "travel_to"
	TimeTravelJobTravelBack TimeTravelJobAction = "travel_back"
)

const TimeTravelFormat = "2006-01-02 15:04:05 MST"

func init() {
	registerJobNameFunc(
		"TimeTravelJob",
		func(ctx context.Context, id JobId, pool *pgw.Pool, args []any) error {
			if len(args) != 3 {
				return oops.Newf("Expected 3 args, got %d: %v", len(args), args)
			}

			commandId, ok := args[0].(int64)
			if !ok {
				commandIdInt, ok := args[0].(int)
				if !ok {
					return oops.Newf("Failed to parse commandId (expected int64 or int): %v", args[0])
				}
				commandId = int64(commandIdInt)
			}

			actionStr, ok := args[1].(string)
			if !ok {
				return oops.Newf("Failed to parse action (expected string): %v", args[1])
			}
			action := TimeTravelJobAction(actionStr)

			timestampMap, ok := args[2].(map[string]any)
			if !ok {
				return oops.Newf("Failed to parse timestamp (expected map): %v", args[2])
			}
			timestampValue, ok := timestampMap["value"]
			if !ok {
				return oops.Newf("Failed to get timestamp value: %v", timestampMap)
			}
			timestampStr, ok := timestampValue.(string)
			if !ok {
				return oops.Newf("Failed to parse timestamp value (expected string): %v", timestampValue)
			}
			timestamp, err := time.Parse(yamlTimeFormat, timestampStr)
			if err != nil {
				return oops.Wrap(err)
			}

			return TimeTravelJob_Perform(ctx, pool, commandId, action, timestamp)
		},
	)
}

func TimeTravelJob_PerformAtEpoch(
	qu pgw.Queryable, commandId int64, action TimeTravelJobAction, timestamp time.Time,
) error {
	return performAt(
		qu, schedule.EpochTime, "TimeTravelJob", defaultQueue, int64ToYaml(commandId),
		strToYaml(string(action)), timeToYaml(timestamp),
	)
}

func TimeTravelJob_Perform(
	ctx context.Context, pool *pgw.Pool, commandId int64, action TimeTravelJobAction, timestamp time.Time,
) error {
	if !config.Cfg.Env.IsDevOrTest() {
		return oops.New("No time travel in production!")
	}

	logger := pool.Logger()
	switch action {
	case TimeTravelJobTravelTo:
		schedule.MustSetUTCNowOverride(timestamp)
	case TimeTravelJobTravelBack:
		schedule.ResetUTCNowOverride()
	default:
		return oops.Newf("Unknown action: %s", action)
	}

	utcNowStr := schedule.UTCNow().Format(TimeTravelFormat)
	logger.Info().Msgf("Current time: %s", utcNowStr)

	err := util.Tx(pool, func(tx *pgw.Tx, pool util.Clobber) error {
		err := models.TestSingleton_SetValue(tx, "time_travel_command_id", fmt.Sprint(commandId))
		if err != nil {
			return err
		}
		err = models.TestSingleton_SetValue(tx, "time_travel_timestamp", utcNowStr)
		if err != nil {
			return err
		}
		return nil
	})

	return err
}
