package middleware

import (
	"context"
	"errors"
	"feedrewind/models"
	"net/http"
)

// CurrentUser should come after Session
func CurrentUser(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		pool := GetDBPool(r)
		authToken := GetSessionAuthToken(r)
		maybeCurrentUser, err := models.User_FindByAuthToken(pool, authToken)
		if errors.Is(err, models.ErrUserNotFound) {
			maybeCurrentUser = nil
		} else if err != nil {
			panic(err)
		}

		var productUserId models.ProductUserId
		if maybeCurrentUser != nil {
			productUserId = maybeCurrentUser.ProductUserId
		} else if sessionProductUserId := GetSessionProductUserId(r); sessionProductUserId != "" {
			productUserId = sessionProductUserId
		} else {
			var err error
			productUserId, err = models.ProductUserId_New()
			if err != nil {
				panic(err)
			}
			MustSetSessionProductUserId(w, r, productUserId)
		}

		var currentUserHasBounced bool
		if maybeCurrentUser != nil {
			var err error
			currentUserHasBounced, err = models.PostmarkBouncedUser_Exists(pool, maybeCurrentUser.Id)
			if err != nil {
				panic(err)
			}
		} else {
			currentUserHasBounced = false
		}

		if maybeCurrentUser != nil {
			setLoggerUserId(r, maybeCurrentUser.Id)
		} else {
			setLoggerUserId(r, 0)
		}

		setCurrentUserData(r, maybeCurrentUser, productUserId, currentUserHasBounced)
		next.ServeHTTP(w, r)
	}
	return http.HandlerFunc(fn)
}

type currentUserDataKeyType struct{}

var currentUserDataKey = &currentUserDataKeyType{}

type currentUserData struct {
	IsSet         bool
	MaybeUser     *models.User
	ProductUserId models.ProductUserId
	HasBounced    bool
}

// To be called by Logger middleware so that the context persists till request completion
func withCurrentUserData(r *http.Request) *http.Request {
	data := &currentUserData{} //nolint:exhaustruct
	r = r.WithContext(context.WithValue(r.Context(), currentUserDataKey, data))
	return r
}

// To be called by CurrentUser middleware
func setCurrentUserData(
	r *http.Request, maybeUser *models.User, productUserId models.ProductUserId, hasBounced bool,
) {
	data := r.Context().Value(currentUserDataKey).(*currentUserData)
	data.IsSet = true
	data.MaybeUser = maybeUser
	data.ProductUserId = productUserId
	data.HasBounced = hasBounced
}

func GetCurrentUser(r *http.Request) *models.User {
	return r.Context().Value(currentUserDataKey).(*currentUserData).MaybeUser
}

func GetCurrentProductUserId(r *http.Request) models.ProductUserId {
	return r.Context().Value(currentUserDataKey).(*currentUserData).ProductUserId
}

func GetCurrentUserHasBounced(r *http.Request) bool {
	return r.Context().Value(currentUserDataKey).(*currentUserData).HasBounced
}
