package routes

import (
	"feedrewind/db/pgw"
	"feedrewind/jobs"
	"feedrewind/models"
	"feedrewind/oops"
	"feedrewind/routes/rutil"
	"feedrewind/third_party/tzdata"
	"feedrewind/util"
	"feedrewind/util/schedule"
	"fmt"
	"math"
	"net/http"
	"time"
)

func AdminTest_RescheduleUserJob(w http.ResponseWriter, r *http.Request) {
	currentUserId := rutil.CurrentUserId(r)
	conn := rutil.DBConn(r)
	_, err := conn.Exec(`
		delete from delayed_jobs
		where (handler like '%class: PublishPostsJob%' or
			handler like '%class: EmailInitialItemJob%' or
			handler like '%class: EmailPostsJob%' or
			handler like '%class: EmailFinalItemJob%') and
			handler like concat('%', $1::text, '%')
	`, fmt.Sprint(currentUserId))
	if err != nil {
		panic(err)
	}

	userSettings, err := models.UserSettings_Get(conn, currentUserId)
	if err != nil {
		panic(err)
	}

	err = jobs.PublishPostsJob_ScheduleInitial(conn, currentUserId, userSettings)
	if err != nil {
		panic(err)
	}

	util.MustWrite(w, "OK")
}

func AdminTest_RunResetFailedBlogsJob(w http.ResponseWriter, r *http.Request) {
	conn := rutil.DBConn(r)
	err := jobs.ResetFailedBlogsJob_PerformNow(conn, false)
	if err != nil {
		panic(err)
	}
	util.MustWrite(w, "OK")
}

func AdminTest_DestroyUserSubscriptions(w http.ResponseWriter, r *http.Request) {
	currentUserId := rutil.CurrentUserId(r)
	conn := rutil.DBConn(r)
	_, err := conn.Exec(`delete from subscriptions_with_discarded where user_id = $1`, currentUserId)
	if err != nil {
		panic(err)
	}
	util.MustWrite(w, "OK")
}

func AdminTest_DestroyUser(w http.ResponseWriter, r *http.Request) {
	email := util.EnsureParamStr(r, "email")
	conn := rutil.DBConn(r)
	user, err := models.User_FindByEmail(conn, email)
	if err == models.ErrUserNotFound {
		util.MustWrite(w, "NotFound")
		return
	}

	_, err = conn.Exec(`delete from users where id = $1`, user.Id)
	if err != nil {
		panic(err)
	}
	util.MustWrite(w, "OK")
}

func AdminTest_SetEmailMetadata(w http.ResponseWriter, r *http.Request) {
	value := util.EnsureParamStr(r, "value")
	conn := rutil.DBConn(r)
	_, err := conn.Exec(`
		update test_singletons
		set value = $1
		where key = 'email_metadata'
	`, value)
	if err != nil {
		panic(err)
	}
	util.MustWrite(w, "OK")
}

func AdminTest_AssertEmailCountWithMetadata(w http.ResponseWriter, r *http.Request) {
	value := util.EnsureParamStr(r, "value")
	count := util.EnsureParamInt(r, "count")
	lastTimestamp := util.EnsureParamStr(r, "last_timestamp")
	lastTag := util.EnsureParamStr(r, "last_tag")

	pollCount := 0
	conn := rutil.DBConn(r)
	postmarkClient, _ := jobs.GetPostmarkClientAndMaybeMetadata(conn)
	for {
		messages, _, err := postmarkClient.GetOutboundMessages(
			r.Context(), 100, 0, map[string]any{"metadata_test": value},
		)
		if err != nil {
			panic(err)
		}

		if len(messages) == count {
			lastMessage := messages[0]
			for i := 1; i < len(messages); i++ {
				if messages[i].Metadata["server_timestamp"].(string) >
					lastMessage.Metadata["server_timestamp"].(string) {

					lastMessage = messages[i]
				}
			}

			if count != 0 && lastMessage.Metadata["server_timestamp"] != lastTimestamp {
				panic(oops.Newf(
					"Last message timestamp doesn't match. Expected: %s, actual: %s",
					lastTimestamp, lastMessage.Metadata["server_timestamp"],
				))
			}

			if count != 0 && lastMessage.Tag != lastTag {
				panic(oops.Newf(
					"Last message tag doesn't match. Expected: %s, actual: %s", lastTag, lastMessage.Tag,
				))
			}

			util.MustWrite(w, "OK")
			return
		}

		time.Sleep(time.Second)
		pollCount++
		if pollCount >= 20 {
			panic(oops.Newf("Email count doesn't match: expected %d, actual %d", count, len(messages)))
		}
	}
}

func AdminTest_DeleteEmailMetadata(w http.ResponseWriter, r *http.Request) {
	conn := rutil.DBConn(r)
	_, err := conn.Exec(`
		update test_singletons
		set value = null
		where key = 'email_metadata'
	`)
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

	conn := rutil.DBConn(r)
	commandId, err := util.RandomInt63()
	if err != nil {
		panic(err)
	}
	err = jobs.TimeTravelJob_PerformAtEpoch(conn, commandId, jobs.TimeTravelJobTravelTo, timestamp)
	if err != nil {
		panic(err)
	}
	err = adminTest_CompareTimestamps(conn, commandId)
	if err != nil {
		panic(err)
	}

	util.MustWrite(w, schedule.UTCNow().Format(time.RFC3339))
}

func AdminTest_TravelBack(w http.ResponseWriter, r *http.Request) {
	schedule.ResetUTCNowOverride()

	conn := rutil.DBConn(r)
	commandId, err := util.RandomInt63()
	if err != nil {
		panic(err)
	}
	err = jobs.TimeTravelJob_PerformAtEpoch(conn, commandId, jobs.TimeTravelJobTravelBack, time.Time{})
	if err != nil {
		panic(err)
	}
	err = adminTest_CompareTimestamps(conn, commandId)
	if err != nil {
		panic(err)
	}

	util.MustWrite(w, schedule.UTCNow().Format(time.RFC3339))
}

func AdminTest_WaitForPublishPostsJob(w http.ResponseWriter, r *http.Request) {
	currentUserId := rutil.CurrentUserId(r)

	utcNow := schedule.UTCNow()
	conn := rutil.DBConn(r)
	userSettings, err := models.UserSettings_Get(conn, currentUserId)
	if err != nil {
		panic(err)
	}
	localTime := utcNow.In(tzdata.LocationByName[userSettings.Timezone])
	localDate := localTime.Date()

	for pollCount := 0; pollCount < 50; pollCount++ {
		isScheduledForDate, err := jobs.PublishPostsJob_IsScheduledForDate(conn, currentUserId, localDate)
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
	conn := rutil.DBConn(r)
	query := util.EnsureParamStr(r, "query")
	jsonQuery := fmt.Sprintf(`
		with result_rows as (%s)
		SELECT array_to_json(array_agg(row_to_json(t))) FROM result_rows t
	`, query)
	row := conn.QueryRow(jsonQuery)
	var result *string
	err := row.Scan(&result)
	if err != nil {
		panic(err)
	}
	if result != nil {
		util.MustWrite(w, *result)
	}
}

func adminTest_CompareTimestamps(conn *pgw.Conn, commandId int64) error {
	commandIdStr := fmt.Sprint(commandId)
	pollCount := 0
	for {
		workerCommandIdStr, err := models.TestSingleton_GetValue(conn, "time_travel_command_id")
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
	maybeWorkerTimestampStr, err := models.TestSingleton_GetValue(conn, "time_travel_timestamp")
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
