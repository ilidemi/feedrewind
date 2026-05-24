package routes

import (
	"fmt"
	"math"
	"net/http"
	"time"

	"feedrewind.com/db/pgw"
	"feedrewind.com/jobs"
	"feedrewind.com/models"
	"feedrewind.com/oops"
	"feedrewind.com/routes/rutil"
	"feedrewind.com/third_party/tzdata"
	"feedrewind.com/util"
	"feedrewind.com/util/schedule"
)

func AdminTest_RescheduleUserJob(w http.ResponseWriter, r *http.Request) {
	currentUserId := rutil.CurrentUserId(r)
	pool := rutil.DBPool(r)
	_, err := pool.Exec(`
		delete from delayed_jobs
		where handler like '%class: PublishPostsJob%' and
			handler like concat('%- ', $1::text, E'\n%')
	`, fmt.Sprint(currentUserId))
	if err != nil {
		panic(err)
	}

	userSettings, err := models.UserSettings_Get(pool, currentUserId)
	if err != nil {
		panic(err)
	}
	if userSettings.MaybeDeliveryChannel == nil {
		dc := models.DeliveryChannelMultipleFeeds
		userSettings.MaybeDeliveryChannel = &dc
	}

	err = jobs.PublishPostsJob_ScheduleInitial(pool, currentUserId, userSettings, false)
	if err != nil {
		panic(err)
	}

	util.MustWrite(w, "OK")
}

func AdminTest_RunResetFailedBlogsJob(w http.ResponseWriter, r *http.Request) {
	pool := rutil.DBPool(r)
	err := jobs.ResetFailedBlogsJob_PerformNow(pool, false)
	if err != nil {
		panic(err)
	}
	util.MustWrite(w, "OK")
}

func AdminTest_DestroyUserSubscriptions(w http.ResponseWriter, r *http.Request) {
	currentUserId := rutil.CurrentUserId(r)
	pool := rutil.DBPool(r)
	_, err := pool.Exec(`delete from subscriptions_with_discarded where user_id = $1`, currentUserId)
	if err != nil {
		panic(err)
	}
	util.MustWrite(w, "OK")
}

func AdminTest_DestroyUser(w http.ResponseWriter, r *http.Request) {
	email := util.EnsureParamStr(r, "email")
	pool := rutil.DBPool(r)

	rows, err := pool.Query(`delete from users_with_discarded where email = $1 returning id`, email)
	if err != nil {
		panic(err)
	}
	var deletedIds []models.UserId
	for rows.Next() {
		var userId models.UserId
		err := rows.Scan(&userId)
		if err != nil {
			panic(err)
		}
		deletedIds = append(deletedIds, userId)
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}
	if len(deletedIds) == 0 {
		util.MustWrite(w, "NotFound")
		return
	}

	for _, userId := range deletedIds {
		_, err := pool.Exec(`
			delete from delayed_jobs
			where handler like '%class: PublishPostsJob%'
				and handler like concat(E'%arguments:\n  - ', $1::text, E'\n%')
		`, fmt.Sprint(userId))
		if err != nil {
			panic(err)
		}
	}
	util.MustWrite(w, "OK")
}

func AdminTest_GetTestSingleton(w http.ResponseWriter, r *http.Request) {
	key := util.EnsureParamStr(r, "key")
	pool := rutil.DBPool(r)
	row := pool.QueryRow(`select value from test_singletons where key = $1`, key)
	var maybeValue *string
	err := row.Scan(&maybeValue)
	if err != nil {
		panic(err)
	}
	value := "<nil>"
	if maybeValue != nil {
		value = *maybeValue
	}
	util.MustWrite(w, value)
}

func AdminTest_SetTestSingleton(w http.ResponseWriter, r *http.Request) {
	key := util.EnsureParamStr(r, "key")
	value := util.EnsureParamStr(r, "value")
	pool := rutil.DBPool(r)
	_, err := pool.Exec(`update test_singletons set value = $1 where key = $2`, value, key)
	if err != nil {
		panic(err)
	}
	util.MustWrite(w, "OK")
}

func AdminTest_DeleteTestSingleton(w http.ResponseWriter, r *http.Request) {
	key := util.EnsureParamStr(r, "key")
	pool := rutil.DBPool(r)
	_, err := pool.Exec(`update test_singletons set value = null where key = $1`, key)
	if err != nil {
		panic(err)
	}
	util.MustWrite(w, "OK")
}

func AdminTest_TravelTo(w http.ResponseWriter, r *http.Request) {
	timestampStr := util.EnsureParamStr(r, "timestamp")
	timestamp, err := time.Parse(time.RFC3339, timestampStr)
	if err != nil {
		panic(err)
	}
	if timestamp.Location() != nil && timestamp.Location().String() != "UTC" {
		panic("Expected UTC timestamp")
	}
	schedule.MustSetUTCNowOverride(timestamp)

	pool := rutil.DBPool(r)
	commandId, err := util.RandomInt63()
	if err != nil {
		panic(err)
	}
	err = jobs.TimeTravelJob_PerformAtEpoch(pool, commandId, jobs.TimeTravelJobTravelTo, timestamp)
	if err != nil {
		panic(err)
	}
	err = adminTest_CompareTimestamps(pool, commandId)
	if err != nil {
		panic(err)
	}

	util.MustWrite(w, schedule.UTCNow().Format(time.RFC3339))
}

func AdminTest_TravelBack(w http.ResponseWriter, r *http.Request) {
	schedule.ResetUTCNowOverride()

	pool := rutil.DBPool(r)
	commandId, err := util.RandomInt63()
	if err != nil {
		panic(err)
	}
	err = jobs.TimeTravelJob_PerformAtEpoch(pool, commandId, jobs.TimeTravelJobTravelBack, time.Time{})
	if err != nil {
		panic(err)
	}
	err = adminTest_CompareTimestamps(pool, commandId)
	if err != nil {
		panic(err)
	}

	util.MustWrite(w, schedule.UTCNow().Format(time.RFC3339))
}

func AdminTest_WaitForPublishPostsJob(w http.ResponseWriter, r *http.Request) {
	currentUserId := rutil.CurrentUserId(r)

	utcNow := schedule.UTCNow()
	pool := rutil.DBPool(r)
	userSettings, err := models.UserSettings_Get(pool, currentUserId)
	if err != nil {
		panic(err)
	}
	localTime := utcNow.In(tzdata.LocationByName[userSettings.Timezone])
	localDate := localTime.Date()

	for pollCount := 0; pollCount < 50; pollCount++ {
		isScheduledForDate, err := jobs.PublishPostsJob_IsScheduledForDate(pool, currentUserId, localDate)
		if err != nil {
			panic(err)
		}
		if !isScheduledForDate {
			util.MustWrite(w, "OK")
			return
		}

		time.Sleep(100 * time.Millisecond)
	}
	panic("Job didn't run")
}

func AdminTest_ExecuteSql(w http.ResponseWriter, r *http.Request) {
	pool := rutil.DBPool(r)
	query := util.EnsureParamStr(r, "query")
	jsonQuery := fmt.Sprintf(`
		with result_rows as (%s)
		SELECT array_to_json(array_agg(row_to_json(t))) FROM result_rows t
	`, query)
	row := pool.QueryRow(jsonQuery)
	var result *string
	err := row.Scan(&result)
	if err != nil {
		util.MustWrite(w, fmt.Sprintf("ERROR: %v", err))
	}
	if result != nil {
		util.MustWrite(w, *result)
	}
}

func adminTest_CompareTimestamps(pool *pgw.Pool, commandId int64) error {
	commandIdStr := fmt.Sprint(commandId)
	pollCount := 0
	for {
		workerCommandIdStr, err := models.TestSingleton_GetValue(pool, "time_travel_command_id")
		if err != nil {
			return err
		}
		if workerCommandIdStr != nil && *workerCommandIdStr == commandIdStr {
			break
		}

		time.Sleep(100 * time.Millisecond)
		pollCount++
		if pollCount >= 50 {
			return oops.Newf("Worker didn't time travel (command id %s)", commandIdStr)
		}
	}

	webTimestamp := schedule.UTCNow()
	maybeWorkerTimestampStr, err := models.TestSingleton_GetValue(pool, "time_travel_timestamp")
	if err != nil {
		return err
	}
	if maybeWorkerTimestampStr == nil {
		return oops.New("Time travel timestamp can't be null")
	}
	workerTimestamp, err := schedule.ParseTime(jobs.TimeTravelFormat, *maybeWorkerTimestampStr)
	if err != nil {
		return oops.Wrap(err)
	}
	difference := workerTimestamp.Sub(webTimestamp).Seconds()
	if math.Abs(difference) > 60 {
		return oops.Newf("Web timestamp %s doesn't match worker timestamp %s", webTimestamp, workerTimestamp)
	}
	return nil
}
