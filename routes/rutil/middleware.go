package rutil

import (
	"feedrewind/db/pgw"
	"feedrewind/middleware"
	"feedrewind/models"
	"feedrewind/util"
	"fmt"
	"html/template"
	"net/http"
)

// This file wraps calls to the middleware package so that the routes don't have to reference it

func DBConn(r *http.Request) *pgw.Conn {
	return middleware.GetDBConn(r)
}

func CurrentUser(r *http.Request) *models.User {
	return middleware.GetCurrentUser(r)
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
