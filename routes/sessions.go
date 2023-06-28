package routes

import (
	"feedrewind/db"
	"feedrewind/log"
	"feedrewind/middleware"
	"feedrewind/models"
	"feedrewind/routes/rutil"
	"feedrewind/templates"
	"feedrewind/util"
	"net/http"

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

func LoginPage(w http.ResponseWriter, r *http.Request) {
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
	email := util.EnsureParam(r, "email")
	password := util.EnsureParam(r, "current-password")
	redirect := util.EnsureParam(r, "redirect")

	user := models.User_MustFindByEmail(db.Conn, email)
	if user != nil {
		err := bcrypt.CompareHashAndPassword([]byte(user.PasswordDigest), []byte(password))
		if err == nil {
			middleware.MustSetSessionAuthToken(w, r, user.AuthToken)

			// Users visiting landing page then signing in need to be excluded from the sign up funnel
			// Track them twice: first as anonymous, then properly
			currentProductUserId := rutil.CurrentProductUserId(r)
			models.ProductEvent_MustEmitFromRequest(
				db.Conn, r, currentProductUserId, "log in", map[string]any{
					"user_is_anonymous": true,
				},
			)
			models.ProductEvent_MustEmitFromRequest(
				db.Conn, r, user.ProductUserId, "log in", map[string]any{
					"user_is_anonymous": false,
				},
			)

			subscriptionId := rutil.MustExtractAnonymousSubscriptionId(w, r)
			if subscriptionId != 0 && models.Subscription_MustExists(db.Conn, subscriptionId) {
				models.Subscription_MustSetUserId(db.Conn, subscriptionId, user.Id)
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
