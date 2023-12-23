package routes

import (
	"feedrewind/jobs"
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
	logger := rutil.Logger(r)
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
	user, err := models.FullUser_FindByEmail(conn, email)
	if errors.Is(err, models.ErrUserNotFound) {
		logger.Info().Msg("User not found")
	} else if err != nil {
		panic(err)
	} else {
		err := bcrypt.CompareHashAndPassword([]byte(user.PasswordDigest), []byte(password))
		if err == nil {
			middleware.MustSetSessionAuthToken(w, r, user.AuthToken)

			// Users visiting landing page then signing in need to be excluded from the sign up funnel
			// Track them twice: first as anonymous, then properly
			pc := models.NewProductEventContext(conn, r, rutil.CurrentProductUserId(r))
			models.ProductEvent_MustEmitFromRequest(
				pc, "log in", map[string]any{"user_is_anonymous": true}, nil,
			)
			models.ProductEvent_MustEmitFromRequest(
				pc, "log in", map[string]any{"user_is_anonymous": false}, nil,
			)

			subscriptionId := rutil.MustExtractAnonymousSubscriptionId(w, r)
			if subscriptionId != 0 {
				exists, err := models.Subscription_Exists(conn, subscriptionId)
				if err != nil {
					panic(err)
				}
				if exists {
					err := models.Subscription_SetUserId(conn, subscriptionId, user.Id)
					if err != nil {
						panic(err)
					}
					http.Redirect(w, r, rutil.SubscriptionSetupPath(subscriptionId), http.StatusFound)
				}
			}

			if redirect == "" {
				redirect = "/subscriptions"
			}
			http.Redirect(w, r, redirect, http.StatusFound)
			return
		} else {
			logger.Info().Err(err).Msg("Password doesn't match")
		}
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
	logger := rutil.Logger(r)
	if rutil.CurrentUser(r) != nil {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	email := util.EnsureParamStr(r, "email")
	password := util.EnsureParamStr(r, "new-password")
	timezone := util.EnsureParamStr(r, "timezone")
	timeOffsetStr := util.EnsureParamStr(r, "time_offset")

	tx, err := rutil.DBConn(r).Begin()
	if err != nil {
		panic(err)
	}
	defer util.CommitOrRollbackOnPanic(tx)

	const passwordTooShort = "Password is too short (minimum is 8 characters)"
	const userAlreadyExists = "We already have an account registered with that email address"
	existingUser, err := models.FullUser_FindByEmail(tx, email)
	userExists := true
	if errors.Is(err, models.ErrUserNotFound) {
		userExists = false
	} else if err != nil {
		panic(err)
	}

	var user *models.FullUser
	if userExists && existingUser.PasswordDigest == "" {
		user, err = models.FullUser_UpdatePassword(tx, existingUser.Id, password)
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
		productUserExists, err := models.User_ExistsByProductUserId(tx, productUserId)
		if err != nil {
			panic(err)
		}
		if productUserExists {
			var err error
			productUserId, err = models.ProductUserId_New()
			if err != nil {
				panic(err)
			}
		}
		user, err = models.FullUser_Create(tx, email, password, name, productUserId)
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
			logger.Warn().Msgf("Unknown timezone: %s", timezone)
			timeOffset, err := strconv.ParseInt(timeOffsetStr, 10, 32)
			if err != nil {
				logger.Warn().Msgf("Couldn't parse time offset: %s", timeOffsetStr)
				timeOffset = 0
			}
			offsetHoursInverted := int(timeOffset) / 60
			var ok bool
			timezoneOut, ok = util.UnfriendlyGroupIdByOffset[offsetHoursInverted]
			if !ok {
				logger.Warn().Msgf("Time offset too large: %s", timeOffsetStr)
				timezoneOut = util.TimezoneUTC
			}
		}
		logger.Info().Msgf("Timezone out: %s", timezoneOut)

		err = models.UserSettings_Create(tx, user.Id, timezoneOut)
		if err != nil {
			panic(err)
		}

		err = publish.CreateEmptyUserFeed(tx, user.Id)
		if err != nil {
			panic(err)
		}

		pc := models.NewProductEventContext(tx, r, user.ProductUserId)
		models.ProductEvent_MustEmitFromRequest(pc, "sign up", nil, nil)

		slackMessage := fmt.Sprintf("*%s* signed up", jobs.NotifySlackJob_Escape(user.Email))
		err = jobs.NotifySlackJob_PerformNow(tx, slackMessage)
		if err != nil {
			panic(err)
		}
	}

	middleware.MustSetSessionAuthToken(w, r, user.AuthToken)

	subscriptionId := rutil.MustExtractAnonymousSubscriptionId(w, r)
	if subscriptionId != 0 {
		subscriptionExists, err := models.Subscription_Exists(tx, subscriptionId)
		if err != nil {
			panic(err)
		}
		if subscriptionExists {
			err := models.Subscription_SetUserId(tx, subscriptionId, user.Id)
			if err != nil {
				panic(err)
			}
			http.Redirect(w, r, rutil.SubscriptionSetupPath(subscriptionId), http.StatusFound)
			return
		} else {
			http.Redirect(w, r, "/subscriptions", http.StatusFound)
			return
		}
	} else {
		http.Redirect(w, r, "/subscriptions", http.StatusFound)
		return
	}
}
