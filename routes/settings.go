package routes

import (
	"feedrewind/config"
	"feedrewind/jobs"
	"feedrewind/models"
	"feedrewind/routes/rutil"
	"feedrewind/templates"
	"feedrewind/third_party/tzdata"
	"feedrewind/util"
	"fmt"
	"html/template"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pkg/errors"
	"github.com/stripe/stripe-go/v78"
	"github.com/stripe/stripe-go/v78/billingportal/session"
)

type deliveryChannel string

const (
	deliveryChannelRSS   deliveryChannel = "rss"
	deliveryChannelEmail deliveryChannel = "email"
)

type deliverySettings struct {
	IsRSSSelected   bool
	IsEmailSelected bool
	RSSValue        deliveryChannel
	EmailValue      deliveryChannel
}

func newDeliverySettings(userSettings *models.UserSettings) deliverySettings {
	isRSSSelected := false
	isEmailSelected := false
	if userSettings.MaybeDeliveryChannel != nil {
		isRSSSelected = *userSettings.MaybeDeliveryChannel == models.DeliveryChannelMultipleFeeds
		isEmailSelected = *userSettings.MaybeDeliveryChannel == models.DeliveryChannelEmail
	}
	return deliverySettings{
		IsRSSSelected:   isRSSSelected,
		IsEmailSelected: isEmailSelected,
		RSSValue:        deliveryChannelRSS,
		EmailValue:      deliveryChannelEmail,
	}
}

func UserSettings_Page(w http.ResponseWriter, r *http.Request) {
	logger := rutil.Logger(r)
	pool := rutil.DBPool(r)
	currentUser := rutil.CurrentUser(r)
	userSettings, err := models.UserSettings_Get(pool, currentUser.Id)
	if err != nil {
		panic(err)
	}
	userGroupId, userGroupFound := util.GroupIdByTimezoneId[userSettings.Timezone]
	row := pool.QueryRow(`
		select plan_id from pricing_offers
		where id = (select offer_id from users_without_discarded where id = $1)
	`, currentUser.Id)
	var planId models.PlanId
	err = row.Scan(&planId)
	if err != nil {
		panic(err)
	}
	var currentPlan string
	switch planId {
	case models.PlanIdFree:
		currentPlan = "Free"
	case models.PlanIdSupporter:
		currentPlan = "Supporter"
	case models.PlanIdPatron:
		currentPlan = "Patron"
	default:
		panic(fmt.Errorf("Unknown plan: %s", planId))
	}
	isPaid := planId != models.PlanIdFree

	cancelAtStr := ""
	renewsOnStr := ""
	if isPaid {
		row := pool.QueryRow(`
			select stripe_cancel_at, stripe_current_period_end from users_without_discarded where id = $1
		`, currentUser.Id)
		var maybeCancelAt, maybeCurrentPeriodEnd *time.Time
		err := row.Scan(&maybeCancelAt, &maybeCurrentPeriodEnd)
		if err != nil {
			panic(err)
		}
		if maybeCancelAt != nil {
			timezone := tzdata.LocationByName[userSettings.Timezone]
			cancelAtStr = maybeCancelAt.In(timezone).Format("Jan 2, 2006")
		}
		if maybeCurrentPeriodEnd != nil {
			timezone := tzdata.LocationByName[userSettings.Timezone]
			renewsOnStr = maybeCurrentPeriodEnd.In(timezone).Format("Jan 2, 2006")
		}
	}

	isPatron := planId == models.PlanIdPatron
	patronCredits := 0
	if isPatron {
		row := pool.QueryRow(`select count from patron_credits where user_id = $1`, currentUser.Id)
		err := row.Scan(&patronCredits)
		if errors.Is(err, pgx.ErrNoRows) {
			logger.Warn().Msgf("Displaying 0 credits to user %d who upgraded to paid", currentUser.Id)
		} else if err != nil {
			panic(err)
		}
	}

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
		DeliveryChannel                      deliverySettings
		Version                              int
		ShortFriendlyPrefixNameByGroupIdJson template.JS
		GroupIdByTimezoneIdJson              template.JS
		CurrentPlan                          string
		IsPatron                             bool
		PatronCredits                        int
		CancelAt                             string
		RenewsOn                             string
		IsPaid                               bool
	}
	templates.MustWrite(w, "settings/settings", SettingsResult{
		Title:                                util.DecorateTitle("Settings"),
		Session:                              rutil.Session(r),
		TimezoneOptions:                      timezoneOptions,
		DeliveryChannel:                      newDeliverySettings(userSettings),
		Version:                              userSettings.Version,
		ShortFriendlyPrefixNameByGroupIdJson: util.ShortFriendlyPrefixNameByGroupIdJson,
		GroupIdByTimezoneIdJson:              util.GroupIdByTimezoneIdJson,
		CurrentPlan:                          currentPlan,
		IsPatron:                             planId == models.PlanIdPatron,
		PatronCredits:                        patronCredits,
		CancelAt:                             cancelAtStr,
		RenewsOn:                             renewsOnStr,
		IsPaid:                               isPaid,
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

func UserSettings_SaveDeliveryChannel(w http.ResponseWriter, r *http.Request) {
	logger := rutil.Logger(r)
	pool := rutil.DBPool(r)
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

		oldDeliveryChannel := oldUserSettings.MaybeDeliveryChannel
		err = models.UserSettings_SaveDeliveryChannelVersion(
			tx, currentUser.Id, newDeliveryChannel, newVersion,
		)
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
			jobTime, err := jobDate.TimeIn(location)
			if err != nil {
				panic(err)
			}
			newHour := jobs.PublishPostsJob_GetHourOfDay(newDeliveryChannel)
			newRunAt := jobTime.Add(time.Duration(newHour) * time.Hour).UTC()
			err = jobs.PublishPostsJob_UpdateRunAt(tx, job.Id, newRunAt)
			if err != nil {
				panic(err)
			}
			logger.Info().Msgf("Rescheduled PublishPostsJob for %s", newRunAt)
		} else {
			newUserSettings, err := models.UserSettings_Get(tx, currentUser.Id)
			if err != nil {
				panic(err)
			}
			err = jobs.PublishPostsJob_ScheduleInitial(tx, currentUser.Id, newUserSettings, false)
			if err != nil {
				panic(err)
			}
			logger.Info().Msg("Did initial schedule for PublishPostsJob")
		}

		pc := models.NewProductEventContext(tx, r, rutil.CurrentProductUserId(r))
		models.ProductEvent_MustEmitFromRequest(pc, "update delivery channel", map[string]any{
			"old_delivery_channel": oldDeliveryChannel,
			"new_delivery_channel": newDeliveryChannel,
		}, map[string]any{
			"delivery_channel": newDeliveryChannel,
		})

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

		if mustSaveDeliveryChannel() {
			break
		} else {
			failedLockAttempts++
		}
	}
}

func UserSettings_Billing(w http.ResponseWriter, r *http.Request) {
	currentUserId := rutil.CurrentUserId(r)
	pool := rutil.DBPool(r)
	row := pool.QueryRow(`
		select stripe_customer_id, (select plan_id from pricing_offers where id = offer_id)
		from users_without_discarded
		where id = $1
	`, currentUserId)
	var stripeCustomerId string
	var planId models.PlanId
	err := row.Scan(&stripeCustomerId, &planId)
	if err != nil {
		panic(err)
	}

	var billingPortalConfigurationId string
	switch planId {
	case models.PlanIdFree:
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	case models.PlanIdSupporter:
		billingPortalConfigurationId = config.Cfg.StripeSupporterConfigId
	case models.PlanIdPatron:
		billingPortalConfigurationId = config.Cfg.StripePatronConfigId
	default:
		panic(fmt.Errorf("Unknown plan id: %s", planId))
	}

	//nolint:exhaustruct
	params := &stripe.BillingPortalSessionParams{
		Customer:      stripe.String(stripeCustomerId),
		ReturnURL:     stripe.String(config.Cfg.RootUrl + "/settings"),
		Configuration: stripe.String(billingPortalConfigurationId),
	}
	portalSession, err := session.New(params)
	if err != nil {
		panic(err)
	}
	http.Redirect(w, r, portalSession.URL, http.StatusSeeOther)
}

func UserSettings_BillingFull(w http.ResponseWriter, r *http.Request) {
	currentUserId := rutil.CurrentUserId(r)
	logger := rutil.Logger(r)
	pool := rutil.DBPool(r)
	row := pool.QueryRow(`
		select stripe_customer_id, (select plan_id from pricing_offers where id = offer_id)
		from users_without_discarded
		where id = $1
	`, currentUserId)
	var stripeCustomerId string
	var planId models.PlanId
	err := row.Scan(&stripeCustomerId, &planId)
	if err != nil {
		panic(err)
	}

	if planId == models.PlanIdFree {
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}

	logger.Warn().Msgf("User %d has visited billing_full portal, consider Stripe flows?", currentUserId)

	//nolint:exhaustruct
	params := &stripe.BillingPortalSessionParams{
		Customer:  stripe.String(stripeCustomerId),
		ReturnURL: stripe.String(config.Cfg.RootUrl + "/pricing"),
	}
	portalSession, err := session.New(params)
	if err != nil {
		panic(err)
	}
	http.Redirect(w, r, portalSession.URL, http.StatusSeeOther)
}
