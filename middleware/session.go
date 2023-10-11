package middleware

import (
	"context"
	"feedrewind/config"
	"feedrewind/log"
	"feedrewind/models"
	"net/http"
	"time"

	"github.com/gorilla/securecookie"
)

var secureCookie *securecookie.SecureCookie

func init() {
	secureCookie = securecookie.New(config.Cfg.SessionHashKey, config.Cfg.SessionBlockKey)
}

type sessionData struct {
	AuthToken     string
	ProductUserId models.ProductUserId
}

const cookieName = "feedrewind_session"

func Session(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		var session sessionData
		cookie, err := r.Cookie(cookieName)
		if err == nil {
			err := secureCookie.Decode(cookieName, cookie.Value, &session)
			if err != nil {
				log.Info(r).Err(err).Msg("Couldn't decode session cookie")
			}
		}

		next.ServeHTTP(w, withSession(r, &session))
	}
	return http.HandlerFunc(fn)
}

type sessionKeyType struct{}

var sessionKey = &sessionKeyType{}

func withSession(r *http.Request, session *sessionData) *http.Request {
	r = r.WithContext(context.WithValue(r.Context(), sessionKey, session))
	return r
}

func GetSessionAuthToken(r *http.Request) string {
	return r.Context().Value(sessionKey).(*sessionData).AuthToken
}

func GetSessionProductUserId(r *http.Request) models.ProductUserId {
	return r.Context().Value(sessionKey).(*sessionData).ProductUserId
}

func MustSetSessionAuthToken(w http.ResponseWriter, r *http.Request, authToken string) {
	session := r.Context().Value(sessionKey).(*sessionData)
	session.AuthToken = authToken
	mustSetCookie(w, session)
}

func MustSetSessionProductUserId(w http.ResponseWriter, r *http.Request, productUserId models.ProductUserId) {
	session := r.Context().Value(sessionKey).(*sessionData)
	session.ProductUserId = productUserId
	mustSetCookie(w, session)
}

func mustSetCookie(w http.ResponseWriter, session *sessionData) {
	encoded, err := secureCookie.Encode(cookieName, session)
	if err != nil {
		panic(err)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    encoded,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().UTC().AddDate(1, 0, 0),
	})
}
