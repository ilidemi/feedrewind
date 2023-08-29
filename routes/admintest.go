package routes

import (
	"feedrewind/db/pgw"
	"feedrewind/jobs"
	"feedrewind/models"
	"feedrewind/oops"
	"feedrewind/routes/rutil"
	"feedrewind/third_party/tzdata"
	"feedrewind/util"
	"fmt"
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

	_, err = w.Write([]byte("OK"))
	if err != nil {
		panic(err)
	}
}

func AdminTest_DestroyUserSubscriptions(w http.ResponseWriter, r *http.Request) {
	currentUserId := rutil.CurrentUserId(r)
	conn := rutil.DBConn(r)
	_, err := conn.Exec(`delete from subscriptions where user_id = $1`, currentUserId)
	if err != nil {
		panic(err)
	}
	_, err = w.Write([]byte("OK"))
	if err != nil {
		panic(err)
	}
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
	util.Schedule_UTCNowOverride = timestamp

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

	_, err = w.Write([]byte(util.Schedule_UTCNow().Format(time.RFC3339)))
	if err != nil {
		panic(err)
	}
}

func AdminTest_TravelBack(w http.ResponseWriter, r *http.Request) {
	util.Schedule_UTCNowOverride = time.Time{}

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

	_, err = w.Write([]byte(util.Schedule_UTCNow().Format(time.RFC3339)))
	if err != nil {
		panic(err)
	}
}

func AdminTest_WaitForPublishPostsJob(w http.ResponseWriter, r *http.Request) {
	currentUserId := rutil.CurrentUserId(r)

	utcNow := util.Schedule_UTCNow()
	conn := rutil.DBConn(r)
	userSettings, err := models.UserSettings_Get(conn, currentUserId)
	if err != nil {
		panic(err)
	}
	localTime := utcNow.In(tzdata.LocationByName[userSettings.Timezone])
	localDate := util.Schedule_Date(localTime)

	for pollCount := 0; pollCount < 10; pollCount++ {
		isScheduledForDate, err := jobs.PublishPostsJob_IsScheduledForDate(conn, currentUserId, localDate)
		if err != nil {
			panic(err)
		}
		if !isScheduledForDate {
			_, err := w.Write([]byte("OK"))
			if err != nil {
				panic(err)
			}
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
		_, err = w.Write([]byte(*result))
		if err != nil {
			panic(err)
		}
	}
}

func adminTest_CompareTimestamps(conn *pgw.Conn, commandId int64) error {
	commandIdStr := fmt.Sprint(commandId)
	for pollCount := 0; pollCount < 30; pollCount++ {
		workerCommandIdStr, err := models.TestSingleton_GetValue(conn, "time_travel_command_id")
		if err != nil {
			return err
		}
		if workerCommandIdStr == commandIdStr {
			return nil
		}

		time.Sleep(100 * time.Millisecond)
	}
	return oops.Newf("Worker didn't time travel (command id %s)", commandIdStr)
}
