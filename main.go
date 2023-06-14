package main

import (
	"feedrewind/helpers"
	"feedrewind/log"
	frmiddleware "feedrewind/middleware"
	"feedrewind/routes"
	"fmt"
	"net/http"

	_ "net/http/pprof"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

//go:generate go run cmd/timezones/main.go
//go:generate go run third_party/tzdata/generate_zipdata.go

func main() {
	go func() {
		fmt.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	r := chi.NewRouter()
	r.Use(frmiddleware.Logger)
	r.Use(middleware.Compress(5))
	r.Use(frmiddleware.Recoverer)
	r.Use(frmiddleware.DefaultHeaders)
	r.Use(middleware.GetHead)

	r.Get("/", routes.LandingIndex)
	r.Get(helpers.StaticRouteTemplate, routes.StaticFile)

	log.Info().Msg("Started")
	if err := http.ListenAndServe(":3000", r); err != nil {
		panic(err)
	}

}
