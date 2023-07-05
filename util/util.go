package util

import (
	"feedrewind/db/pgw"
	"feedrewind/log"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
)

const LoginPath = "/login"
const SignUpPath = "/signup"

func LoginPathWithRedirect(r *http.Request) string {
	redirect := url.QueryEscape(r.URL.Path)
	return fmt.Sprintf("%s?redirect=%s", LoginPath, redirect)
}

type Session struct {
	CSRFToken      string
	CSRFField      template.HTML
	IsLoggedIn     bool
	UserHasBounced bool
	UserEmail      string
	UserName       string
}

func CommitOrRollback(tx *pgw.Tx, isSuccess bool, successMsg string) {
	if rvr := recover(); rvr != nil {
		if err := tx.Rollback(); err != nil {
			log.Error().
				Err(err).
				Msg("Rollback error")
		}
		panic(rvr)
	} else if isSuccess {
		if err := tx.Commit(); err != nil {
			panic(err)
		}
		if successMsg != "" {
			log.Info().Msg(successMsg)
		}
	} else {
		if err := tx.Rollback(); err != nil {
			panic(err)
		}
	}
}
