package main

import (
	"feedrewind/log"
	frmiddleware "feedrewind/middleware"
	"feedrewind/routes"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
)

func main() {
	r := chi.NewRouter()
	r.Use(frmiddleware.Logger)
	r.Use(frmiddleware.Recoverer)

	r.Get("/", routes.LandingIndex)

	workdir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	staticDir := http.Dir(filepath.Join(workdir, "web"))
	r.Get("/static/*", func(w http.ResponseWriter, r *http.Request) {
		rctx := chi.RouteContext(r.Context())
		pathPrefix := strings.TrimSuffix(rctx.RoutePattern(), "/*")
		fs := http.StripPrefix(pathPrefix, http.FileServer(staticDir))
		fs.ServeHTTP(w, r)
	})

	log.Info().Msg("Started")
	if err := http.ListenAndServe(":3000", r); err != nil {
		panic(err)
	}

}
