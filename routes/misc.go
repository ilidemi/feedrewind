package routes

import (
	"feedrewind/models"
	"feedrewind/routes/rutil"
	"feedrewind/templates"
	"feedrewind/util"
	"net/http"
)

type miscResult struct {
	Title   string
	Session *util.Session
}

func Misc_Terms(w http.ResponseWriter, r *http.Request) {
	templates.MustWrite(w, "misc/terms", miscResult{
		Title:   util.DecorateTitle("Terms"),
		Session: rutil.Session(r),
	})
}

func Misc_Privacy(w http.ResponseWriter, r *http.Request) {
	templates.MustWrite(w, "misc/privacy", miscResult{
		Title:   util.DecorateTitle("Privacy"),
		Session: rutil.Session(r),
	})
}

func Misc_About(w http.ResponseWriter, r *http.Request) {
	templates.MustWrite(w, "misc/about", miscResult{
		Title:   util.DecorateTitle("About"),
		Session: rutil.Session(r),
	})
}

func Misc_NotFound(w http.ResponseWriter, r *http.Request) {
	logger := rutil.Logger(r)
	conn := rutil.DBConn(r)
	models.ProductEvent_DummyEmitOrLog(conn, r, false, "404", map[string]any{
		"path":    r.URL.Path,
		"method":  r.Method,
		"referer": util.CollapseReferer(r),
	}, logger)
	w.WriteHeader(http.StatusNotFound)
	type Result struct {
		Title string
	}
	templates.MustWrite(w, "misc/404", Result{
		Title: util.DecorateTitle("Page not found"),
	})
}
