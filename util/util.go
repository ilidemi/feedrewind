package util

import (
	"crypto/rand"
	"encoding/binary"
	"feedrewind/db/pgw"
	"feedrewind/log"
	"feedrewind/oops"
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

func CommitOrRollbackMsg(tx *pgw.Tx, isSuccess *bool, successMsg string) {
	if rvr := recover(); rvr != nil {
		if err := tx.Rollback(); err != nil {
			log.Error().Err(err).Msg("Rollback error")
		}
		panic(rvr)
	} else if *isSuccess {
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

func CommitOrRollback(tx *pgw.Tx, isSuccess *bool) {
	CommitOrRollbackMsg(tx, isSuccess, "")
}

func CommitOrRollbackErr(tx *pgw.Tx, err *error) {
	isSuccess := *err == nil
	CommitOrRollbackMsg(tx, &isSuccess, "")
}

func CommitOrRollbackOnPanic(tx *pgw.Tx) {
	isSuccess := true
	CommitOrRollbackMsg(tx, &isSuccess, "")
}

func Ordinal(number int) string {
	number = number % 100
	if number == 11 || number == 12 || number == 13 {
		return "th"
	}

	switch number % 10 {
	case 1:
		return "st"
	case 2:
		return "nd"
	case 3:
		return "rd"
	default:
		return "th"
	}
}

func RandomInt63() (int64, error) {
	buf := make([]byte, 8)
	for {
		_, err := rand.Read(buf)
		if err != nil {
			return 0, oops.Wrap(err)
		}
		uVal := binary.LittleEndian.Uint64(buf)
		val := int64(uVal & ((1 << 63) - 1))
		if val == 0 {
			continue
		}
		return val, nil
	}
}

func DecorateTitle(title string) string {
	return title + " · FeedRewind"
}
