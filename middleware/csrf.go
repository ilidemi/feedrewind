package middleware

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"feedrewind.com/config"
	"feedrewind.com/util"
)

const CSRFFormKey = "authenticity_token"

var errCsrfValidationFailed = errors.New("CSRF validation failed")

func CSRF(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		authToken := GetSessionAuthToken(r)
		var rawAuthToken []byte
		if authToken != "" {
			var err error
			urlAuthToken := strings.ReplaceAll(strings.ReplaceAll(authToken, "+", "-"), "/", "_")
			rawAuthToken, err = base64.RawURLEncoding.DecodeString(urlAuthToken)
			if err != nil {
				panic(err)
			}
			if len(rawAuthToken) != config.AuthTokenLength {
				panic(fmt.Errorf("Unexpected auth token length: %d", len(rawAuthToken)))
			}
		}

		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			var incomingToken string
			err := r.ParseForm()
			if err == nil {
				incomingToken = r.PostFormValue(CSRFFormKey)
			}
			if incomingToken == "" {
				incomingToken = r.Header.Get("X-CSRF-Token")
			}

			if !mustValidateCSRFToken(incomingToken, rawAuthToken) {
				if len(rawAuthToken) != 0 &&
					(r.URL.Path == util.LoginPath || r.URL.Path == util.SignUpPath) {
					// Trying to log in or sign up when already logged in
					http.Redirect(w, r, "/", http.StatusSeeOther)
					return
				}

				panic(util.HttpError{
					Status: http.StatusForbidden,
					Inner:  errCsrfValidationFailed,
				})
			}
		}

		csrfToken := mustMaskCSRFToken(rawAuthToken)
		next.ServeHTTP(w, withCSRFToken(r, csrfToken))
	}
	return http.HandlerFunc(fn)
}

func mustValidateCSRFToken(csrfToken string, authToken []byte) bool {
	maskedCSRFToken, err := base64.RawStdEncoding.DecodeString(csrfToken)
	if err != nil {
		return false
	}

	if len(maskedCSRFToken) != config.AuthTokenLength*2 {
		return false
	}

	if len(authToken) == 0 {
		return true
	}

	oneTimePad := maskedCSRFToken[:config.AuthTokenLength]
	encryptedCSRFToken := maskedCSRFToken[config.AuthTokenLength:]
	decodedCSRFToken := make([]byte, config.AuthTokenLength)
	for i := 0; i < config.AuthTokenLength; i++ {
		decodedCSRFToken[i] = oneTimePad[i] ^ encryptedCSRFToken[i]
	}

	return subtle.ConstantTimeCompare(decodedCSRFToken, []byte(authToken)) == 1
}

func mustMaskCSRFToken(authToken []byte) string {
	oneTimePad := make([]byte, config.AuthTokenLength)
	_, err := rand.Read(oneTimePad)
	if err != nil {
		panic(err)
	}

	decodedCSRFToken := []byte(authToken)
	if len(decodedCSRFToken) == 0 {
		decodedCSRFToken = make([]byte, config.AuthTokenLength)
		_, err := rand.Read(decodedCSRFToken)
		if err != nil {
			panic(err)
		}
	}

	encryptedCSRFToken := make([]byte, config.AuthTokenLength)
	for i := 0; i < config.AuthTokenLength; i++ {
		encryptedCSRFToken[i] = oneTimePad[i] ^ decodedCSRFToken[i]
	}
	maskedCSRFToken := slices.Concat(oneTimePad, encryptedCSRFToken)

	return base64.RawStdEncoding.EncodeToString(maskedCSRFToken)
}

type csrfTokenKeyType struct{}

var csrfTokenKey = &csrfTokenKeyType{}

func withCSRFToken(r *http.Request, csrfToken string) *http.Request {
	r = r.WithContext(context.WithValue(r.Context(), csrfTokenKey, csrfToken))
	return r
}

func GetCSRFToken(r *http.Request) string {
	return r.Context().Value(csrfTokenKey).(string)
}
