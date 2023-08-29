package jobs

import (
	"feedrewind/db/pgw"
	"time"
)

type TimeTravelJobAction string

const (
	TimeTravelJobTravelTo   TimeTravelJobAction = "travel_to"
	TimeTravelJobTravelBack TimeTravelJobAction = "travel_back"
)

func TimeTravelJob_PerformAtEpoch(
	tx pgw.Queryable, commandId int64, action TimeTravelJobAction, timestamp time.Time,
) error {
	return performAt(
		tx, time.Unix(0, 0), "TimeTravelJob", defaultQueue, int64ToYaml(commandId), strToYaml(string(action)),
		timeToYaml(timestamp),
	)
}
