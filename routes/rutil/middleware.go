package rutil

import (
	"feedrewind/middleware"
	"feedrewind/models"
	"fmt"
	"html/template"
	"net/http"
)

// This file wraps calls to the middleware package so that the routes don't have to reference it

func CurrentUser(r *http.Request) *models.User {
	return middleware.GetCurrentUser(r)
}

func CSRFField(r *http.Request) template.HTML {
	return template.HTML(fmt.Sprintf(
		"<input type=\"hidden\" name=\"authenticity_token\" value=\"%s\">", middleware.GetCSRFToken(r),
	))
}
