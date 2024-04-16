package routes

import (
	"feedrewind/db/pgw"
	"feedrewind/jobs"
	"feedrewind/middleware"
	"feedrewind/models"
	"feedrewind/oops"
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
	templates.MustWrite(w, "users/login", result)
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
	switch {
	case errors.Is(err, models.ErrUserNotFound):
		logger.Info().Msg("User not found")
	case err != nil:
		panic(err)
	default:
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
	templates.MustWrite(w, "users/login", result)
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
	templates.MustWrite(w, "users/signup", result)
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
			templates.MustWrite(w, "users/signup", result)
			return
		} else if err != nil {
			panic(err)
		}
	} else {
		atIdx := strings.Index(email, "@")
		if atIdx == -1 {
			panic(oops.Newf("Email is expected to contain an @: %s", email))
		}
		name := email[:atIdx]
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
		switch {
		case errors.Is(err, models.ErrUserAlreadyExists):
			result := newSignUpResult(r, userAlreadyExists)
			templates.MustWrite(w, "users/signup", result)
			return
		case errors.Is(err, models.ErrPasswordTooShort):
			result := newSignUpResult(r, passwordTooShort)
			templates.MustWrite(w, "users/signup", result)
			return
		case err != nil:
			panic(err)
		}

		var timezoneOut string
		if _, ok := util.GroupIdByTimezoneId[timezone]; ok {
			timezoneOut = timezone
		} else {
			if timezone == "1" || timezone == "" { // Dummy timezone bots use
				logger.Info().Msgf("Unknown timezone: %s", timezone)
			} else {
				logger.Warn().Msgf("Unknown timezone: %s", timezone)
			}

			timeOffset := int64(0)
			if timeOffsetStr == "" {
				logger.Info().Msg("Empty time offset")
			} else {
				var err error
				timeOffset, err = strconv.ParseInt(timeOffsetStr, 10, 32)
				if err != nil {
					logger.Warn().Msgf("Couldn't parse time offset: %s", timeOffsetStr)
					timeOffset = 0
				}
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

		slackMessage := "New user signed up"
		if atIndex := strings.LastIndex(user.Email, "@"); atIndex >= 0 {
			emailHost := strings.ToLower(user.Email[atIndex+1:])
			if popularEmailHosts[emailHost] {
				slackMessage = fmt.Sprintf("New user @%s signed up", emailHost)
			}
		}
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

func DeleteAccount(w http.ResponseWriter, r *http.Request) {
	logger := rutil.Logger(r)
	user := rutil.CurrentUser(r)
	conn := rutil.DBConn(r)
	err := util.Tx(conn, func(tx *pgw.Tx, conn util.Clobber) error {
		_, err := tx.Exec(`update users_with_discarded set discarded_at = utc_now() where id = $1`, user.Id)
		if err != nil {
			return err
		}
		result, err := tx.Exec(`
			update subscriptions_without_discarded
			set is_paused = true
			where user_id = $1 and not is_paused
		`, user.Id)
		if err != nil {
			return err
		}
		logger.Info().Msgf("Paused %d subscription(s)", result.RowsAffected())
		return nil
	})
	if err != nil {
		panic(err)
	}

	http.Redirect(w, r, "/", http.StatusFound)
}

var popularEmailHosts map[string]bool

func init() {
	popularEmailHosts = map[string]bool{}
	for _, host := range []string{
		"aol.com", "att.net", "comcast.net", "facebook.com", "gmail.com", "gmx.com", "googlemail.com",
		"google.com", "hotmail.com", "hotmail.co.uk", "mac.com", "me.com", "mail.com", "msn.com", "live.com",
		"sbcglobal.net", "verizon.net", "yahoo.com", "yahoo.co.uk", "email.com", "fastmail.fm", "games.com",
		"gmx.net", "hush.com", "hushmail.com", "icloud.com", "iname.com", "inbox.com", "lavabit.com",
		"love.com", "outlook.com", "pobox.com", "protonmail.ch", "protonmail.com", "tutanota.de",
		"tutanota.com", "tutamail.com", "tuta.io", "keemail.me", "rocketmail.com", "safe-mail.net",
		"wow.com", "ygm.com", "ymail.com", "zoho.com", "yandex.com", "bellsouth.net", "charter.net",
		"cox.net", "earthlink.net", "juno.com", "btinternet.com", "virginmedia.com", "blueyonder.co.uk",
		"freeserve.co.uk", "live.co.uk", "ntlworld.com", "o2.co.uk", "orange.net", "sky.com",
		"talktalk.co.uk", "tiscali.co.uk", "virgin.net", "wanadoo.co.uk", "bt.com", "sina.com", "sina.cn",
		"qq.com", "naver.com", "hanmail.net", "daum.net", "nate.com", "yahoo.co.jp", "yahoo.co.kr",
		"yahoo.co.id", "yahoo.co.in", "yahoo.com.sg", "yahoo.com.ph", "163.com", "yeah.net", "126.com",
		"21cn.com", "aliyun.com", "foxmail.com", "hotmail.fr", "live.fr", "laposte.net", "yahoo.fr",
		"wanadoo.fr", "orange.fr", "gmx.fr", "sfr.fr", "neuf.fr", "free.fr", "gmx.de", "hotmail.de",
		"live.de", "online.de", "t-online.de", "web.de", "yahoo.de", "libero.it", "virgilio.it", "hotmail.it",
		"aol.it", "tiscali.it", "alice.it", "live.it", "yahoo.it", "email.it", "tin.it", "poste.it",
		"teletu.it", "mail.ru", "rambler.ru", "yandex.ru", "ya.ru", "list.ru", "hotmail.be", "live.be",
		"skynet.be", "voo.be", "tvcablenet.be", "telenet.be", "hotmail.com.ar", "live.com.ar", "yahoo.com.ar",
		"fibertel.com.ar", "speedy.com.ar", "arnet.com.ar", "yahoo.com.mx", "live.com.mx", "hotmail.es",
		"hotmail.com.mx", "prodigy.net.mx", "yahoo.ca", "hotmail.ca", "bell.net", "shaw.ca", "sympatico.ca",
		"rogers.com", "yahoo.com.br", "hotmail.com.br", "outlook.com.br", "uol.com.br", "bol.com.br",
		"terra.com.br", "ig.com.br", "itelefonica.com.br", "r7.com", "zipmail.com.br", "globo.com",
		"globomail.com", "oi.com.br", "hey.com",
	} {
		popularEmailHosts[host] = true
	}
}
