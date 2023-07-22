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

		data := currentUserData{
			User:          currentUser,
			ProductUserId: productUserId,
			HasBounced:    currentUserHasBounced,
		}
		next.ServeHTTP(w, withCurrentUserData(r, &data))
	}
	return http.HandlerFunc(fn)
}

type currentUserDataKeyType struct{}

var currentUserDataKey = &currentUserDataKeyType{}

type currentUserData struct {
	User          *models.User
	ProductUserId models.ProductUserId
	HasBounced    bool
}

func withCurrentUserData(r *http.Request, data *currentUserData) *http.Request {
	r = r.WithContext(context.WithValue(r.Context(), currentUserDataKey, data))
	return r
}

func GetCurrentUser(r *http.Request) *models.User {
	return r.Context().Value(currentUserDataKey).(*currentUserData).User
}

func GetCurrentProductUserId(r *http.Request) models.ProductUserId {
	return r.Context().Value(currentUserDataKey).(*currentUserData).ProductUserId
}

func GetCurrentUserHasBounced(r *http.Request) bool {
	return r.Context().Value(currentUserDataKey).(*currentUserData).HasBounced
}
