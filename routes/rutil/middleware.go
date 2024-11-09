package rutil

import (
	"fmt"
	"html/template"
	"net/http"

	"feedrewind.com/db/pgw"
	"feedrewind.com/log"
	"feedrewind.com/middleware"
	"feedrewind.com/models"
	"feedrewind.com/util"
)

// This file wraps calls to the middleware package so that the routes don't have to reference it

func Logger(r *http.Request) log.Logger {
	return middleware.GetLogger(r)
}

func DBPool(r *http.Request) *pgw.Pool {
	return middleware.GetDBPool(r)
}

func CurrentUser(r *http.Request) *models.User {
	return middleware.GetCurrentUser(r)
}

func CurrentUserId(r *http.Request) models.UserId {
	currentUser := middleware.GetCurrentUser(r)
	if currentUser == nil {
		return 0
	}
	return currentUser.Id
}

func CurrentProductUserId(r *http.Request) models.ProductUserId {
	return middleware.GetCurrentProductUserId(r)
}

func Session(r *http.Request) *util.Session {
	csrfToken := middleware.GetCSRFToken(r)
	csrfField := template.HTML(fmt.Sprintf(
		"<input type=\"hidden\" name=\"authenticity_token\" value=\"%s\">", csrfToken,
	))

	currentUser := CurrentUser(r)
	isLoggedIn := false
	userHasBounced := false
	userEmail := ""
	userName := ""
	if currentUser != nil {
		isLoggedIn = true
		userHasBounced = middleware.GetCurrentUserHasBounced(r)
		userEmail = currentUser.Email
		userName = currentUser.Name
	}

	return &util.Session{
		CSRFToken:      csrfToken,
		CSRFField:      csrfField,
		IsLoggedIn:     isLoggedIn,
		UserHasBounced: userHasBounced,
		UserEmail:      userEmail,
		UserName:       userName,
	}
}
