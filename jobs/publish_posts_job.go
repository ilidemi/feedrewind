package jobs

import (
	"errors"
	"feedrewind/db/pgw"
	"feedrewind/models"
	"feedrewind/third_party/tzdata"
	"feedrewind/util"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

func PublishPostsJob_ScheduleInitial(
	tx pgw.Queryable, userId models.UserId, userSettings *models.UserSettings,
) error {
	utcNow := time.Now().UTC()
	location := tzdata.LocationByName[userSettings.Timezone]
	date := time.Date(utcNow.Year(), utcNow.Month(), utcNow.Day(), 0, 0, 0, 0, location).AddDate(0, 0, -1)
	hourOfDay := PublishPostsJob_GetHourOfDay(*userSettings.DeliveryChannel)
	nextRun := date.Add(time.Duration(hourOfDay) * time.Hour).UTC()
	for nextRun.Before(utcNow) {
		date = date.AddDate(0, 0, 1)
		nextRun = date.Add(time.Duration(hourOfDay) * time.Hour).UTC()
	}
	nextRunDate, err := util.Schedule_ToUTCStr(nextRun)
	if err != nil {
		return err
	}

	return performAt(
		tx, nextRun, "PublishPostsJob", defaultQueue,
		yamlString(fmt.Sprint(userId)),
		strToYaml(string(util.Schedule_Date(date))),
		strToYaml(nextRunDate),
	)
}

type LockedPublishPostsJob struct {
	Id       JobId
	LockedBy string
}

func PublishPostsJob_Lock(tx pgw.Queryable, userId models.UserId) ([]LockedPublishPostsJob, error) {
	rows, err := tx.Query(`
		select id, locked_by
		from delayed_jobs
		where (handler like concat(E'%class: PublishPostsJob\n%'))
			and handler like concat(E'%arguments:\n  - ', $1::text, E'\n%')
		for update
	`, fmt.Sprint(userId))
	if err != nil {
		return nil, err
	}

	var locks []LockedPublishPostsJob
	for rows.Next() {
		var lock LockedPublishPostsJob
		var lockedBy *string
		err := rows.Scan(&lock.Id, &lockedBy)
		if err != nil {
			return nil, err
		}
		if lockedBy != nil {
			lock.LockedBy = *lockedBy
		}
		locks = append(locks, lock)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return locks, nil
}

// Returns zero value if the job was not found
func PublishPostsJob_GetNextScheduledDate(tx pgw.Queryable, userId models.UserId) (util.Date, error) {
	row := tx.QueryRow(`
		select (regexp_match(handler, concat(E'arguments:\n  - ', $1::text, E'\n  - ''([0-9-]+)''')))[1]
		from delayed_jobs
		where handler like concat(E'%class: PublishPostsJob\n%') and
			handler like concat(E'%arguments:\n  - ', $1::text, E'\n%')
		order by run_at desc
	`, fmt.Sprint(userId))
	var date util.Date
	err := row.Scan(&date)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	} else if err != nil {
		return "", err
	}

	return date, nil
}

func PublishPostsJob_GetHourOfDay(deliveryChannel models.DeliveryChannel) int {
	switch deliveryChannel {
	case models.DeliveryChannelEmail:
		return 5
	case models.DeliveryChannelMultipleFeeds, models.DeliveryChannelSingleFeed:
		return 2
	default:
		panic(fmt.Errorf("UnknownDeliveryChannel: %s", deliveryChannel))
	}
}

func PublishPostsJob_UpdateRunAt(tx pgw.Queryable, jobId JobId, runAt time.Time) error {
	_, err := tx.Exec(`
		update delayed_jobs set run_at = $1 where id = $2
	`, runAt.Format(runAtFormat), jobId)
	return err
}
