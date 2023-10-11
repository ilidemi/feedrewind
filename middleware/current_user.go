package middleware

import (
	"context"
	"errors"
	"feedrewind/log"
	"feedrewind/models"
	"net/http"
)

// CurrentUser should come after Session
func CurrentUser(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		conn := GetDBConn(r)
		authToken := GetSessionAuthToken(r)
		currentUser, err := models.User_FindByAuthToken(conn, authToken)
		if errors.Is(err, models.ErrUserNotFound) {
			currentUser = nil
		} else if err != nil {
			panic(err)
		}

		var productUserId models.ProductUserId
		if currentUser != nil {
			productUserId = currentUser.ProductUserId
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
		if currentUser != nil {
			var err error
			currentUserHasBounced, err = models.PostmarkBouncedUser_Exists(conn, currentUser.Id)
			if err != nil {
				panic(err)
			}
		} else {
			currentUserHasBounced = false
		}

		setCurrentUserData(r, currentUser, productUserId, currentUserHasBounced)
		next.ServeHTTP(w, r)
	}
	return http.HandlerFunc(fn)
}

type currentUserDataKeyType struct{}

var currentUserDataKey = &currentUserDataKeyType{}

type currentUserData struct {
	IsSet         bool
	User          *models.User
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
	r *http.Request, user *models.User, productUserId models.ProductUserId, hasBounced bool,
) {
	data := r.Context().Value(currentUserDataKey).(*currentUserData)
	data.IsSet = true
	data.User = user
	data.ProductUserId = productUserId
	data.HasBounced = hasBounced
}

func GetCurrentUser(r *http.Request) *models.User {
	return r.Context().Value(currentUserDataKey).(*currentUserData).User
}

func getCurrentUserId(r *http.Request) (int64, error) {
	data := r.Context().Value(currentUserDataKey)
	if data == nil || !data.(*currentUserData).IsSet {
		return 0, log.ErrUserUnknown
	}
	currentUser := data.(*currentUserData).User
	if currentUser == nil {
		return 0, log.ErrUserAnonymous
	}
	return int64(currentUser.Id), nil
}

func init() {
	log.GetCurrentUserId = getCurrentUserId
}

func GetCurrentProductUserId(r *http.Request) models.ProductUserId {
	return r.Context().Value(currentUserDataKey).(*currentUserData).ProductUserId
}

func GetCurrentUserHasBounced(r *http.Request) bool {
	return r.Context().Value(currentUserDataKey).(*currentUserData).HasBounced
}
