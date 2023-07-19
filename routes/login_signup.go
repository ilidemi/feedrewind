package routes

import (
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
	"golang.org/x/crypto/bcrypt"
)

type loginResult struct {
	Session         *util.Session
	Error           string
	Redirect        string
	FormId          string
	EmailInputId    string
	EmailErrorId    string
	PasswordInputId string
	PasswordErrorId string
}

func newLoginResult(r *http.Request, error string, redirect string) loginResult {
	return loginResult{
		Session:         rutil.Session(r),
		Error:           error,
		Redirect:        redirect,
		FormId:          "login_form",
		EmailInputId:    "email",
		EmailErrorId:    "email_error",
		PasswordInputId: "current-password",
		PasswordErrorId: "password_error",
	}
}

func Login_Page(w http.ResponseWriter, r *http.Request) {
	if rutil.CurrentUser(r) != nil {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	query := r.URL.Query()
	redirect := ""
	if redirects, ok := query["redirect"]; ok {
		redirect = redirects[0]
	}

	result := newLoginResult(r, "", redirect)
	templates.MustWrite(w, "login_signup/login", result)
}

func Login(w http.ResponseWriter, r *http.Request) {
	if rutil.CurrentUser(r) != nil {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	err := r.ParseForm()
	if err != nil {
		panic(err)
	}
	email := util.EnsureParamStr(r, "email")
	password := util.EnsureParamStr(r, "current-password")
	redirect := util.EnsureParamStr(r, "redirect")

	conn := rutil.DBConn(r)
	user := models.User_MustFindByEmail(conn, email)
	if user != nil {
		err := bcrypt.CompareHashAndPassword([]byte(user.PasswordDigest), []byte(password))
		if err == nil {
			middleware.MustSetSessionAuthToken(w, r, user.AuthToken)

			// Users visiting landing page then signing in need to be excluded from the sign up funnel
			// Track them twice: first as anonymous, then properly
			currentProductUserId := rutil.CurrentProductUserId(r)
			models.ProductEvent_MustEmitFromRequest(models.ProductEventRequestArgs{
				Tx:            conn,
				Request:       r,
				ProductUserId: currentProductUserId,
				EventType:     "log in",
				EventProperties: map[string]any{
					"user_is_anonymous": true,
				},
				UserProperties: nil,
			})
			models.ProductEvent_MustEmitFromRequest(models.ProductEventRequestArgs{
				Tx:            conn,
				Request:       r,
				ProductUserId: user.ProductUserId,
				EventType:     "log in",
				EventProperties: map[string]any{
					"user_is_anonymous": false,
				},
				UserProperties: nil,
			})

			subscriptionId := rutil.MustExtractAnonymousSubscriptionId(w, r)
			if subscriptionId != 0 && models.Subscription_MustExists(conn, subscriptionId) {
				models.Subscription_MustSetUserId(conn, subscriptionId, user.Id)
				http.Redirect(w, r, rutil.SubscriptionSetupPath(subscriptionId), http.StatusFound)
			}

			if redirect == "" {
				redirect = "/subscriptions"
			}
			http.Redirect(w, r, redirect, http.StatusFound)
			return
		} else {
			log.Info().Err(err).Msg("Password doesn't match")
		}
	} else {
		log.Info().Msg("User not found")
	}

	result := newLoginResult(r, "Email or password is invalid", redirect)
	templates.MustWrite(w, "login_signup/login", result)
}

func Logout(w http.ResponseWriter, r *http.Request) {
	middleware.MustSetSessionAuthToken(w, r, "")
	http.Redirect(w, r, "/", http.StatusFound)
}

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

func SignUp_Page(w http.ResponseWriter, r *http.Request) {
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

	email := util.EnsureParamStr(r, "email")
	password := util.EnsureParamStr(r, "new-password")
	timezone := util.EnsureParamStr(r, "timezone")
	timeOffsetStr := util.EnsureParamStr(r, "time_offset")

	conn := rutil.DBConn(r)
	tx := conn.MustBegin()
	defer func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			panic(errors.Wrap(err, "rollback error"))
		}
	}()
	defer func() {
		if err := tx.Commit(); err != nil {
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
			offsetHoursInverted := int(timeOffset) / 60
			var ok bool
			timezoneOut, ok = util.UnfriendlyGroupIdByOffset[offsetHoursInverted]
			if !ok {
				log.Warn().Msgf("Time offset too large: %s", timeOffsetStr)
				timezoneOut = util.TimezoneUTC
			}
		}
		log.Info().Msgf("Timezone out: %s", timezoneOut)

		models.UserSettings_MustCreate(tx, user.Id, timezoneOut)

		publish.MustCreateEmptyUserFeed(tx, user.Id)

		models.ProductEvent_MustEmitFromRequest(models.ProductEventRequestArgs{
			Tx:              tx,
			Request:         r,
			ProductUserId:   user.ProductUserId,
			EventType:       "sign up",
			EventProperties: nil,
			UserProperties:  nil,
		})

		slackMessage := fmt.Sprintf("*%s* signed up", jobs.NotifySlackJob_Escape(user.Email))
		jobs.NotifySlackJob_MustPerformNow(tx, slackMessage)
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
