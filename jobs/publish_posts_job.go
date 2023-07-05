package jobs

import (
	"feedrewind/db/pgw"
	"feedrewind/models"
	"feedrewind/third_party/tzdata"
	"feedrewind/util"
	"fmt"
	"time"
)

func PublishPostsJob_MustInitialSchedule(
	tx pgw.Queryable, userId models.UserId, userSettings models.UserSettings,
) {
	utcNow := time.Now().UTC()
	location := tzdata.LocationByName[userSettings.Timezone]
	date := time.Date(utcNow.Year(), utcNow.Month(), utcNow.Day(), 0, 0, 0, 0, location).AddDate(0, 0, -1)
	hourOfDay := PublishPostsJob_GetHourOfDay(*userSettings.DeliveryChannel)
	nextRun := date.Add(time.Duration(hourOfDay) * time.Hour).UTC()
	for nextRun.Before(utcNow) {
		date = date.AddDate(0, 0, 1)
		nextRun = date.Add(time.Duration(hourOfDay) * time.Hour).UTC()
	}

	mustPerformAt(
		tx, nextRun, "PublishPostsJob", defaultQueue,
		yamlString(fmt.Sprint(userId)),
		strToYaml(util.Schedule_DateStr(date)),
		strToYaml(util.Schedule_MustUTCStr(nextRun)),
	)
}

type LockedPublishPostsJob struct {
	Id       JobId
	LockedBy string
}

func PublishPostsJob_MustLock(tx pgw.Queryable, userId models.UserId) []LockedPublishPostsJob {
	rows, err := tx.Query(`
		select id, locked_by
		from delayed_jobs
		where (handler like concat(E'%class: PublishPostsJob\n%'))
			and handler like concat(E'%arguments:\n  - ', $1::text, E'\n%')
		for update
	`, fmt.Sprint(userId))
	if err != nil {
		panic(err)
	}

	var locks []LockedPublishPostsJob
	for rows.Next() {
		var lock LockedPublishPostsJob
		var lockedBy *string
		err := rows.Scan(&lock.Id, &lockedBy)
		if err != nil {
			panic(err)
		}
		if lockedBy != nil {
			lock.LockedBy = *lockedBy
		}
		locks = append(locks, lock)
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}

	return locks
}

func PublishPostsJob_MustGetNextScheduledDate(tx pgw.Queryable, userId models.UserId) string {
	row := tx.QueryRow(`
		select (regexp_match(handler, concat(E'arguments:\n  - ', $1::text, E'\n  - ''([0-9-]+)''')))[1]
		from delayed_jobs
		where handler like concat(E'%class: PublishPostsJob\n%') and
			handler like concat(E'%arguments:\n  - ', $1::text, E'\n%')
		order by run_at desc
	`, fmt.Sprint(userId))
	var date string
	err := row.Scan(&date)
	if err != nil {
		panic(err)
	}

	return date
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

func PublishPostsJob_MustUpdateRunAt(tx pgw.Queryable, jobId JobId, runAt time.Time) {
	tx.MustExec(`
		update delayed_jobs set run_at = $1 where id = $2
	`, runAt.Format(runAtFormat), jobId)
}
