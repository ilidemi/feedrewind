package util

import (
	"fmt"
	"net/http"
	"net/url"
)

const LoginPath = "/login"
const SignUpPath = "/signup"

func LoginPathWithRedirect(r *http.Request) string {
	redirect := url.QueryEscape(r.URL.Path)
	return fmt.Sprintf("%s?redirect=%s", LoginPath, redirect)
}
