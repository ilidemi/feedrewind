package routes

import (
	"feedrewind/routes/rutil"
	"feedrewind/templates"
	"feedrewind/util"
	"net/http"
)

type miscResult struct {
	Session *util.Session
}

func Misc_Terms(w http.ResponseWriter, r *http.Request) {
	templates.MustWrite(w, "misc/terms", miscResult{
		Session: rutil.Session(r),
	})
}

func Misc_Privacy(w http.ResponseWriter, r *http.Request) {
	templates.MustWrite(w, "misc/privacy", miscResult{
		Session: rutil.Session(r),
	})
}

func Misc_About(w http.ResponseWriter, r *http.Request) {
	templates.MustWrite(w, "misc/about", miscResult{
		Session: rutil.Session(r),
	})
}
