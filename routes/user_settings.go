package routes

import (
	"feedrewind/jobs"
	"feedrewind/log"
	"feedrewind/models"
	"feedrewind/routes/rutil"
	"feedrewind/templates"
	"feedrewind/third_party/tzdata"
	"feedrewind/util"
	"fmt"
	"html/template"
	"net/http"
	"time"
)

type deliveryChannel string

const (
	deliveryChannelRSS   deliveryChannel = "rss"
	deliveryChannelEmail deliveryChannel = "email"
)

func UserSettings_Page(w http.ResponseWriter, r *http.Request) {
	conn := rutil.DBConn(r)
	currentUser := rutil.CurrentUser(r)
	userSettings, err := models.UserSettings_GetById(conn, currentUser.Id)
	if err != nil {
		panic(err)
	}
	userGroupId, userGroupFound := util.GroupIdByTimezoneId[userSettings.Timezone]

	type timezoneOption struct {
		Value      string
		Label      string
		IsSelected bool
	}
	var timezoneOptions []timezoneOption
	for _, friendlyTimezone := range util.FriendlyTimezones {
		timezoneOptions = append(timezoneOptions, timezoneOption{
			Value:      friendlyTimezone.GroupId,
			Label:      friendlyTimezone.FriendlyName,
			IsSelected: friendlyTimezone.GroupId == userGroupId,
		})
	}
	if !userGroupFound {
		if !util.UnfriendlyGroupIds[userSettings.Timezone] {
			log.Error().
				Str("timezone", userSettings.Timezone).
				Msg("User timezone not found in tzdb")
		}
		timezoneOptions = append(timezoneOptions, timezoneOption{
			Value:      userSettings.Timezone,
			Label:      userSettings.Timezone,
			IsSelected: true,
		})
	}

	type deliverySettings struct {
		IsRSSSelected   bool
		IsEmailSelected bool
		RSSValue        deliveryChannel
		EmailValue      deliveryChannel
	}

	type settingsPageResult struct {
		Session                              *util.Session
		TimezoneOptions                      []timezoneOption
		DeliveryChannel                      deliverySettings
		Version                              int
		ShortFriendlyPrefixNameByGroupIdJson template.JS
		ShortFriendlyNameByGroupIdJson       template.JS
		GroupIdByTimezoneIdJson              template.JS
	}
	isRSSSelected := false
	isEmailSelected := false
	if userSettings.DeliveryChannel != nil {
		isRSSSelected = *userSettings.DeliveryChannel == models.DeliveryChannelMultipleFeeds
		isEmailSelected = *userSettings.DeliveryChannel == models.DeliveryChannelEmail
	}
	result := settingsPageResult{
		Session:         rutil.Session(r),
		TimezoneOptions: timezoneOptions,
		DeliveryChannel: deliverySettings{
			IsRSSSelected:   isRSSSelected,
			IsEmailSelected: isEmailSelected,
			RSSValue:        deliveryChannelRSS,
			EmailValue:      deliveryChannelEmail,
		},
		Version:                              userSettings.Version,
		ShortFriendlyPrefixNameByGroupIdJson: util.ShortFriendlyPrefixNameByGroupIdJson,
		ShortFriendlyNameByGroupIdJson:       util.ShortFriendlyNameByGroupIdJson,
		GroupIdByTimezoneIdJson:              util.GroupIdByTimezoneIdJson,
	}

	templates.MustWrite(w, "users/settings", result)
}

func UserSettings_SaveTimezone(w http.ResponseWriter, r *http.Request) {
	conn := rutil.DBConn(r)
	newTimezone := util.EnsureParamStr(r, "timezone")
	newVersion := util.EnsureParamInt(r, "version")
	newLocation, ok := tzdata.LocationByName[newTimezone]
	if !ok {
		panic(fmt.Errorf("Unknown timezone: %s", newTimezone))
	}
	currentUser := rutil.CurrentUser(r)

	// Saving timezone may race with user's update rss job.
	// If the job is already running, wait till it finishes, otherwise lock the row so it doesn't start
	mustSaveTimezone := func() (result bool) {
		tx, err := conn.Begin()
		if err != nil {
			panic(err)
		}
		defer util.CommitOrRollbackMsg(tx, result, "Unlocked PublishPostsJob")

		log.Info().Msg("Locking PublishPostsJob")
		lockedJobs, err := jobs.PublishPostsJob_Lock(tx, currentUser.Id)
		if err != nil {
			panic(err)
		}
		log.Info().Int("count", len(lockedJobs)).Msg("Locked PublishPostsJob")

		for _, job := range lockedJobs {
			if job.LockedBy != "" {
				log.Info().Int("count", len(lockedJobs)).Msg("Some jobs are running, unlocking")
				return false
			}
		}

		oldUserSettings, err := models.UserSettings_GetById(tx, currentUser.Id)
		if err != nil {
			panic(err)
		}
		if !((oldUserSettings.DeliveryChannel != nil && len(lockedJobs) == 1) ||
			(oldUserSettings.DeliveryChannel == nil && len(lockedJobs) == 0)) {
			log.Warn().Int("count", len(lockedJobs)).Msg("Unexpected amount of job rows for the user")
			return false
		}

		if oldUserSettings.Version >= newVersion {
			log.Info().
				Int("existing_version", oldUserSettings.Version).
				Int("new_version", newVersion).
				Msg("Version conflict")
			rutil.MustWriteJson(w, http.StatusConflict, map[string]any{
				"version": oldUserSettings.Version,
			})
		}

		oldTimezone := oldUserSettings.Timezone
		err = models.UserSettings_SaveTimezone(tx, currentUser.Id, newTimezone, newVersion)
		if err != nil {
			panic(err)
		}

		if len(lockedJobs) == 1 {
			job := lockedJobs[0]
			jobDate, err := jobs.PublishPostsJob_GetNextScheduledDate(tx, currentUser.Id)
			if err != nil {
				panic(err)
			}
			jobTime, err := util.Schedule_DateInLocation(jobDate, newLocation)
			if err != nil {
				panic(err)
			}
			newHour := jobs.PublishPostsJob_GetHourOfDay(*oldUserSettings.DeliveryChannel)
			newRunAt := jobTime.Add(time.Duration(newHour) * time.Hour).UTC()
			err = jobs.PublishPostsJob_UpdateRunAt(tx, job.Id, newRunAt)
			if err != nil {
				panic(err)
			}
			log.Info().Time("run_at", newRunAt).Msg("Rescheduled PublishPostsJob")
		}

		pc := models.NewProductEventContext(conn, r, rutil.CurrentProductUserId(r))
		models.ProductEvent_MustEmitFromRequest(pc, "update timezone", map[string]any{
			"old_timezone": oldTimezone,
			"new_timezone": newTimezone,
		}, nil)

		return true
	}

	failedLockAttempts := 0
	for {
		if failedLockAttempts >= 3 {
			panic("Couldn't lock the job rows")
		} else if failedLockAttempts > 0 {
			time.Sleep(time.Second)
		}

		if mustSaveTimezone() {
			w.WriteHeader(http.StatusOK)
			break
		} else {
			failedLockAttempts++
		}
	}
}

func UserSettings_SaveDeliveryChannel(w http.ResponseWriter, r *http.Request) {
	conn := rutil.DBConn(r)
	currentUser := rutil.CurrentUser(r)
	deliveryChannelStr := util.EnsureParamStr(r, "delivery_channel")
	newVersion := util.EnsureParamInt(r, "version")

	var newDeliveryChannel models.DeliveryChannel
	switch deliveryChannel(deliveryChannelStr) {
	case deliveryChannelRSS:
		newDeliveryChannel = models.DeliveryChannelMultipleFeeds
	case deliveryChannelEmail:
		newDeliveryChannel = models.DeliveryChannelEmail
	default:
		panic(util.HttpError{
			Status: http.StatusBadRequest,
			Inner:  fmt.Errorf("unknown delivery channel: %s", deliveryChannelStr),
		})
	}

	// Saving delivery channel may race with user's update rss job.
	// If the job is already running, wait till it finishes, otherwise lock the row so it doesn't start
	mustSaveDeliveryChannel := func() (result bool) {
		tx, err := conn.Begin()
		if err != nil {
			panic(err)
		}
		defer util.CommitOrRollbackMsg(tx, result, "Unlocked PublishPostsJob")

		log.Info().Msg("Locking PublishPostsJob")
		lockedJobs, err := jobs.PublishPostsJob_Lock(tx, currentUser.Id)
		if err != nil {
			panic(err)
		}
		log.Info().Int("count", len(lockedJobs)).Msg("Locked PublishPostsJob")

		for _, job := range lockedJobs {
			if job.LockedBy != "" {
				log.Info().Int("count", len(lockedJobs)).Msg("Some jobs are running, unlocking")
				return false
			}
		}

		oldUserSettings, err := models.UserSettings_GetById(tx, currentUser.Id)
		if err != nil {
			panic(err)
		}
		if !((oldUserSettings.DeliveryChannel != nil && len(lockedJobs) == 1) ||
			(oldUserSettings.DeliveryChannel == nil && len(lockedJobs) == 0)) {
			log.Warn().Int("count", len(lockedJobs)).Msg("Unexpected amount of job rows for the user")
			return false
		}

		if oldUserSettings.Version >= newVersion {
			log.Info().
				Int("existing_version", oldUserSettings.Version).
				Int("new_version", newVersion).
				Msg("Version conflict")
			rutil.MustWriteJson(w, http.StatusConflict, map[string]any{
				"version": oldUserSettings.Version,
			})
		}

		oldDeliveryChannel := oldUserSettings.DeliveryChannel
		err = models.UserSettings_SaveDeliveryChannel(tx, currentUser.Id, newDeliveryChannel, newVersion)
		if err != nil {
			panic(err)
		}

		if len(lockedJobs) > 0 {
			job := lockedJobs[0]
			jobDate, err := jobs.PublishPostsJob_GetNextScheduledDate(tx, currentUser.Id)
			if err != nil {
				panic(err)
			}
			location := tzdata.LocationByName[oldUserSettings.Timezone]
			jobTime, err := util.Schedule_DateInLocation(jobDate, location)
			if err != nil {
				panic(err)
			}
			newHour := jobs.PublishPostsJob_GetHourOfDay(newDeliveryChannel)
			newRunAt := jobTime.Add(time.Duration(newHour) * time.Hour).UTC()
			err = jobs.PublishPostsJob_UpdateRunAt(tx, job.Id, newRunAt)
			if err != nil {
				panic(err)
			}
			log.Info().Time("run_at", newRunAt).Msg("Rescheduled PublishPostsJob")
		} else {
			err := jobs.PublishPostsJob_ScheduleInitial(tx, currentUser.Id, oldUserSettings)
			if err != nil {
				panic(err)
			}
			log.Info().Msg("Did initial schedule for PublishPostsJob")
		}

		pc := models.NewProductEventContext(conn, r, rutil.CurrentProductUserId(r))
		models.ProductEvent_MustEmitFromRequest(pc, "update delivery channel", map[string]any{
			"old_delivery_channel": oldDeliveryChannel,
			"new_delivery_channel": newDeliveryChannel,
		}, map[string]any{
			"delivery_channel": newDeliveryChannel,
		})

		return true
	}

	failedLockAttempts := 0
	for {
		if failedLockAttempts >= 3 {
			panic("Couldn't lock the job rows")
		} else if failedLockAttempts > 0 {
			time.Sleep(time.Second)
		}

		if mustSaveDeliveryChannel() {
			w.WriteHeader(http.StatusOK)
			break
		} else {
			failedLockAttempts++
		}
	}
}
