package routes

import (
	"fmt"
	"html/template"
	"net/http"
	"time"

	"feedrewind.com/jobs"
	"feedrewind.com/models"
	"feedrewind.com/routes/rutil"
	"feedrewind.com/templates"
	"feedrewind.com/third_party/tzdata"
	"feedrewind.com/util"
)

func UserSettings_Page(w http.ResponseWriter, r *http.Request) {
	logger := rutil.Logger(r)
	pool := rutil.DBPool(r)
	currentUser := rutil.CurrentUser(r)
	userSettings, err := models.UserSettings_Get(pool, currentUser.Id)
	if err != nil {
		panic(err)
	}
	userGroupId, userGroupFound := util.GroupIdByTimezoneId[userSettings.Timezone]

	type TimezoneOption struct {
		Value      string
		Label      string
		IsSelected bool
	}
	var timezoneOptions []TimezoneOption
	for _, friendlyTimezone := range util.FriendlyTimezones {
		timezoneOptions = append(timezoneOptions, TimezoneOption{
			Value:      friendlyTimezone.GroupId,
			Label:      friendlyTimezone.FriendlyName,
			IsSelected: friendlyTimezone.GroupId == userGroupId,
		})
	}
	if !userGroupFound {
		if !util.UnfriendlyGroupIds[userSettings.Timezone] {
			logger.Error().Msgf("User timezone not found in tzdb: %s", userSettings.Timezone)
		}
		timezoneOptions = append(timezoneOptions, TimezoneOption{
			Value:      userSettings.Timezone,
			Label:      userSettings.Timezone,
			IsSelected: true,
		})
	}

	type SettingsResult struct {
		Title                                string
		Session                              *util.Session
		TimezoneOptions                      []TimezoneOption
		Version                              int
		ShortFriendlyPrefixNameByGroupIdJson template.JS
		GroupIdByTimezoneIdJson              template.JS
	}
	templates.MustWrite(w, "settings/settings", SettingsResult{
		Title:                                util.DecorateTitle("Settings"),
		Session:                              rutil.Session(r),
		TimezoneOptions:                      timezoneOptions,
		Version:                              userSettings.Version,
		ShortFriendlyPrefixNameByGroupIdJson: util.ShortFriendlyPrefixNameByGroupIdJson,
		GroupIdByTimezoneIdJson:              util.GroupIdByTimezoneIdJson,
	})
}

func UserSettings_SaveTimezone(w http.ResponseWriter, r *http.Request) {
	logger := rutil.Logger(r)
	pool := rutil.DBPool(r)
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
		tx, err := pool.Begin()
		if err != nil {
			panic(err)
		}
		defer util.CommitOrRollbackMsg(tx, &result, "Unlocked PublishPostsJob")

		logger.Info().Msg("Locking PublishPostsJob")
		lockedJobs, err := jobs.PublishPostsJob_Lock(tx, currentUser.Id)
		if err != nil {
			panic(err)
		}
		logger.Info().Msgf("Locked PublishPostsJob %d", len(lockedJobs))

		for _, job := range lockedJobs {
			if job.LockedBy != "" {
				logger.Info().Msgf("Some jobs are running, unlocking %d", len(lockedJobs))
				return false
			}
		}

		oldUserSettings, err := models.UserSettings_Get(tx, currentUser.Id)
		if err != nil {
			panic(err)
		}
		if !((oldUserSettings.MaybeDeliveryChannel != nil && len(lockedJobs) == 1) ||
			(oldUserSettings.MaybeDeliveryChannel == nil && len(lockedJobs) == 0)) {
			logger.Warn().Msgf("Unexpected amount of job rows for the user: %d", len(lockedJobs))
			return false
		}

		if oldUserSettings.Version >= newVersion {
			logger.Info().Msgf("Version conflict: existing %d, new %d", oldUserSettings.Version, newVersion)
			rutil.MustWriteJson(w, http.StatusConflict, map[string]any{
				"version": oldUserSettings.Version,
			})
			return true
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
			jobTime, err := jobDate.TimeIn(newLocation)
			if err != nil {
				panic(err)
			}
			newHour := jobs.PublishPostsJob_GetHourOfDay(*oldUserSettings.MaybeDeliveryChannel)
			newRunAt := jobTime.Add(time.Duration(newHour) * time.Hour).UTC()
			err = jobs.PublishPostsJob_UpdateRunAt(tx, job.Id, newRunAt)
			if err != nil {
				panic(err)
			}
			logger.Info().Msgf("Rescheduled PublishPostsJob for %s", newRunAt)
		}

		pc := models.NewProductEventContext(tx, r, rutil.CurrentProductUserId(r))
		models.ProductEvent_MustEmitFromRequest(pc, "update timezone", map[string]any{
			"old_timezone": oldTimezone,
			"new_timezone": newTimezone,
		}, nil)

		w.WriteHeader(http.StatusOK)
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
			break
		} else {
			failedLockAttempts++
		}
	}
}

