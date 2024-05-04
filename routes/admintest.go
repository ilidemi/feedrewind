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
	"os/exec"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pkg/errors"
	"github.com/stripe/stripe-go/v78"
	"github.com/stripe/stripe-go/v78/customer"
	"github.com/stripe/stripe-go/v78/subscription"
	"github.com/stripe/stripe-go/v78/testhelpers/testclock"
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
			handler like concat('%- ', $1::text, E'\n%')
	`, fmt.Sprint(currentUserId))
	if err != nil {
		panic(err)
	}

	userSettings, err := models.UserSettings_Get(conn, currentUserId)
	if err != nil {
		panic(err)
	}

	err = jobs.PublishPostsJob_ScheduleInitial(conn, currentUserId, userSettings, false)
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
	row := conn.QueryRow(`
		select stripe_subscription_id from users_without_discarded
		where email = $1 and stripe_subscription_id is not null
	`, email)
	var stripeSubscriptionId string
	err := row.Scan(&stripeSubscriptionId)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		panic(err)
	}
	if err == nil {
		_, err := subscription.Cancel(stripeSubscriptionId, nil)
		if stripeErr, ok := err.(*stripe.Error); ok && stripeErr.Code == stripe.ErrorCodeResourceMissing {
			// already deleted
		} else if err != nil {
			panic(err)
		}
	}

	logger := rutil.Logger(r)
	rows, err := conn.Query(`delete from users_with_discarded where email = $1 returning id`, email)
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
		err := jobs.PublishPostsJob_Delete(conn, userId, logger)
		if err != nil {
			panic(err)
		}
	}
	util.MustWrite(w, "OK")
}

func AdminTest_SetTestSingleton(w http.ResponseWriter, r *http.Request) {
	key := util.EnsureParamStr(r, "key")
	value := util.EnsureParamStr(r, "value")
	conn := rutil.DBConn(r)
	_, err := conn.Exec(`update test_singletons set value = $1 where key = $2`, value, key)
	if err != nil {
		panic(err)
	}
	util.MustWrite(w, "OK")
}

func AdminTest_DeleteTestSingleton(w http.ResponseWriter, r *http.Request) {
	key := util.EnsureParamStr(r, "key")
	conn := rutil.DBConn(r)
	_, err := conn.Exec(`update test_singletons set value = null where key = $1`, key)
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

func AdminTest_EnsureStripeListen(w http.ResponseWriter, r *http.Request) {
	queryCmd := exec.Command(
		"pwsh", "-command",
		`Get-CimInstance win32_process -Filter "name='stripe.exe'" | select -expandproperty CommandLine`,
	)
	queryOutput, err := queryCmd.Output()
	if err != nil {
		panic(err)
	}
	queryOutputTokens := strings.Fields(string(queryOutput))
	if len(queryOutputTokens) == 4 &&
		(queryOutputTokens[0] == "stripe" || queryOutputTokens[0] == "stripe.exe") &&
		queryOutputTokens[1] == "listen" &&
		queryOutputTokens[2] == "--forward-to" &&
		queryOutputTokens[3] == "localhost:3000/stripe/webhook" {

		// Found it
		util.MustWrite(w, "OK")
		return
	} else if len(queryOutputTokens) > 0 {
		panic(fmt.Errorf("Found unknown stripe process: %s", string(queryOutput)))
	}

	// Need to start it ourselves
	stripeCmd := exec.Command(
		"cmd", "/c", "start", "stripe", "listen", "--forward-to", "localhost:3000/stripe/webhook",
	)
	err = stripeCmd.Start()
	if err != nil {
		panic(err)
	}
	time.Sleep(3 * time.Second)
	util.MustWrite(w, "OK")
}

func AdminTest_DeleteStripeSubscription(w http.ResponseWriter, r *http.Request) {
	email := util.EnsureParamStr(r, "email")
	conn := rutil.DBConn(r)
	row := conn.QueryRow(`
		select stripe_subscription_id from users_without_discarded
		where email = $1 and stripe_subscription_id is not null
	`, email)
	var stripeSubscriptionId string
	err := row.Scan(&stripeSubscriptionId)
	if err != nil {
		panic(err)
	}

	_, err = subscription.Cancel(stripeSubscriptionId, nil)
	if stripeErr, ok := err.(*stripe.Error); ok && stripeErr.Code == stripe.ErrorCodeResourceMissing {
		// already deleted
	} else if err != nil {
		panic(err)
	}

	util.MustWrite(w, "OK")
}

func AdminTest_ForwardStripeCustomer45Days(w http.ResponseWriter, r *http.Request) {
	email := util.EnsureParamStr(r, "email")
	conn := rutil.DBConn(r)
	row := conn.QueryRow(`select stripe_customer_id from users_without_discarded where email = $1`, email)
	var stripeCustomerId string
	err := row.Scan(&stripeCustomerId)
	if err != nil {
		panic(err)
	}
	cus, err := customer.Get(stripeCustomerId, nil)
	if err != nil {
		panic(err)
	}
	//nolint:exhaustruct
	_, err = testclock.Advance(cus.TestClock.ID, &stripe.TestHelpersTestClockAdvanceParams{
		FrozenTime: stripe.Int64(time.Now().AddDate(0, 0, 45).Unix()),
	})
	if err != nil {
		panic(err)
	}
	util.MustWrite(w, "OK")
}

func AdminTest_DeleteStripeClocks(w http.ResponseWriter, r *http.Request) {
	it := testclock.List(nil)
	for it.Next() {
		clock := it.TestHelpersTestClock()
		_, err := testclock.Del(clock.ID, nil)
		if err != nil {
			panic(err)
		}
	}
	if err := it.Err(); err != nil {
		panic(err)
	}

	util.MustWrite(w, "OK")
}
