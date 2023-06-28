package routes

import (
	"context"
	"feedrewind/db"
	"feedrewind/jobs"
	"feedrewind/log"
	"feedrewind/middleware"
	"feedrewind/models"
	"feedrewind/publish"
	"feedrewind/routes/rutil"
	"feedrewind/templates"
	"feedrewind/util"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/pkg/errors"
)

type signUpResult struct {
	Session         *util.Session
	Error           string
	FormId          string
	EmailInputId    string
	EmailErrorId    string
	PasswordInputId string
	PasswordErrorId string
}

func newSignUpResult(r *http.Request, errorMsg string) signUpResult {
	return signUpResult{
		Session:         rutil.Session(r),
		Error:           errorMsg,
		FormId:          "signup_form",
		EmailInputId:    "email",
		EmailErrorId:    "email_error",
		PasswordInputId: "new-password",
		PasswordErrorId: "password_error",
	}
}

func SignUpPage(w http.ResponseWriter, r *http.Request) {
	if rutil.CurrentUser(r) != nil {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	result := newSignUpResult(r, "")
	templates.MustWrite(w, "login_signup/signup", result)
}

func SignUp(w http.ResponseWriter, r *http.Request) {
	if rutil.CurrentUser(r) != nil {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	email := util.EnsureParam(r, "email")
	password := util.EnsureParam(r, "new-password")
	timezone := util.EnsureParam(r, "timezone")
	timeOffsetStr := util.EnsureParam(r, "time_offset")

	ctx := context.Background()
	tx, err := db.Conn.Begin(ctx)
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			panic(errors.Wrap(err, "rollback error"))
		}
	}()
	defer func() {
		if err := tx.Commit(ctx); err != nil {
			panic(err)
		}
	}()

	const passwordTooShort = "Password is too short (minimum is 8 characters)"
	const userAlreadyExists = "We already have an account registered with that email address"
	existingUser := models.User_MustFindByEmail(tx, email)
	var user *models.User
	if existingUser != nil && existingUser.PasswordDigest == "" {
		var err error
		user, err = models.User_UpdatePassword(tx, existingUser.Id, password)
		if errors.Is(err, models.ErrPasswordTooShort) {
			result := newSignUpResult(r, passwordTooShort)
			templates.MustWrite(w, "login_signup/signup", result)
			return
		} else if err != nil {
			panic(err)
		}
	} else {
		name := email[:strings.Index(email, "@")]
		productUserId := rutil.CurrentProductUserId(r)
		if productUserId != "" && models.User_MustExistsByProductUserId(tx, productUserId) {
			productUserId = models.ProductUserId_MustNew()
		}
		var err error
		user, err = models.User_Create(tx, email, password, name, productUserId)
		if errors.Is(err, models.ErrUserAlreadyExists) {
			result := newSignUpResult(r, userAlreadyExists)
			templates.MustWrite(w, "login_signup/signup", result)
			return
		} else if errors.Is(err, models.ErrPasswordTooShort) {
			result := newSignUpResult(r, passwordTooShort)
			templates.MustWrite(w, "login_signup/signup", result)
			return
		} else if err != nil {
			panic(err)
		}

		var timezoneOut string
		if _, ok := util.GroupIdByTimezoneId[timezone]; ok {
			timezoneOut = timezone
		} else {
			log.Warn().Msgf("Unknown timezone: %s", timezone)
			timeOffset, err := strconv.ParseInt(timeOffsetStr, 10, 32)
			if err != nil {
				log.Warn().Msgf("Couldn't parse time offset: %s", timeOffsetStr)
				timeOffset = 0
			}
			offsetHoursInverted := timeOffset / 60
			if -14 <= offsetHoursInverted && offsetHoursInverted <= 12 {
				offsetStr := fmt.Sprint(offsetHoursInverted)
				if offsetHoursInverted >= 0 {
					offsetStr = "+" + offsetStr
				}
				timezoneOut = fmt.Sprintf("Etc/GMT%s", offsetStr)
			} else {
				log.Warn().Msgf("Time offset too large: %s", timeOffsetStr)
				timezoneOut = "UTC"
			}
		}
		log.Info().Msgf("Timezone out: %s", timezoneOut)

		models.UserSettings_MustCreate(tx, user.Id, timezoneOut)

		publish.MustCreateEmptyUserFeed(tx, user.Id)

		models.ProductEvent_MustEmitFromRequest(tx, r, user.ProductUserId, "sign up", nil)

		slackMessage := fmt.Sprintf("*%s* signed up", jobs.NotifySlackJob_Escape(user.Email))
		jobs.NotifySlackJob_MustPerformLater(tx, slackMessage)
	}

	middleware.MustSetSessionAuthToken(w, r, user.AuthToken)

	subscriptionId := rutil.MustExtractAnonymousSubscriptionId(w, r)
	if subscriptionId != 0 && models.Subscription_MustExists(tx, subscriptionId) {
		models.Subscription_MustSetUserId(tx, subscriptionId, user.Id)
		http.Redirect(w, r, rutil.SubscriptionSetupPath(subscriptionId), http.StatusFound)
		return
	} else {
		http.Redirect(w, r, "/subscriptions", http.StatusFound)
		return
	}
}
