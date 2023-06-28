package util

import "html/template"

type Session struct {
	CSRFToken      string
	CSRFField      template.HTML
	IsLoggedIn     bool
	UserHasBounced bool
	UserEmail      string
	UserName       string
}
