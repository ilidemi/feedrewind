package util

import (
	"crypto/rand"
	"encoding/binary"
	"feedrewind/db/pgw"
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
			tx.Logger().Error().Err(err).Msg("Rollback error")
		}
		panic(rvr)
	} else if *isSuccess {
		if err := tx.Commit(); err != nil {
			panic(err)
		}
		if successMsg != "" {
			tx.Logger().Info().Msg(successMsg)
		}
	} else {
		if err := tx.Rollback(); err != nil {
			panic(err)
		}
	}
}

func CommitOrRollbackErr(tx *pgw.Tx, err *error) {
	isSuccess := *err == nil
	CommitOrRollbackMsg(tx, &isSuccess, "")
}

func CommitOrRollbackOnPanic(tx *pgw.Tx) {
	isSuccess := true
	CommitOrRollbackMsg(tx, &isSuccess, "")
}

// Helps the caller of Tx to clobber the variable that's passed as parentTx, preventing accidental use
type Clobber struct{}

func Tx(parentTx pgw.Queryable, f func(*pgw.Tx, Clobber) error) error {
	tx, err := parentTx.Begin()
	if err != nil {
		return err
	}

	err = f(tx, Clobber{})
	if err != nil {
		rollbackErr := tx.Rollback()
		if rollbackErr != nil {
			parentTx.Logger().Error().Err(rollbackErr).Msg("Rollback error")
		}
		return err
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	return nil
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
