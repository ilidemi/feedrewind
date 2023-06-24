package main

import (
	"feedrewind/db"
	"feedrewind/log"
	frmiddleware "feedrewind/middleware"
	"feedrewind/routes"
	"feedrewind/util"
	"fmt"
	"net/http"

	_ "net/http/pprof"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/spf13/cobra"
)

//go:generate go run cmd/timezones/main.go
//go:generate go run third_party/tzdata/generate_zipdata.go

func main() {
	// pprof
	go func() {
		fmt.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	rootCmd := &cobra.Command{
		Use: "feedrewind",
		Run: func(_ *cobra.Command, _ []string) {
			runServer()
		},
	}
	rootCmd.AddCommand(db.DbCmd)

	if err := rootCmd.Execute(); err != nil {
		panic(err)
	}
}

func runServer() {
	r := chi.NewRouter()
	r.Use(frmiddleware.Logger)
	r.Use(middleware.Compress(5))
	r.Use(frmiddleware.Recoverer)
	r.Use(frmiddleware.DefaultHeaders)
	r.Use(middleware.GetHead)
	r.Use(frmiddleware.Session)
	r.Use(frmiddleware.CurrentUser)
	r.Use(frmiddleware.CSRF)

	r.Get("/", routes.LandingIndex)
	r.Get(util.LoginPath, routes.LoginPage)
	r.Post(util.LoginPath, routes.Login)
	r.Get("/logout", routes.Logout)
	r.Get(util.SignUpPath, routes.SignUpPage)
	r.Post(util.SignUpPath, routes.SignUp)
	r.Get("/subscriptions", routes.Dashboard)
	r.Get(util.StaticRouteTemplate, routes.StaticFile)

	log.Info().Msg("Started")
	if err := http.ListenAndServe(":3000", r); err != nil {
		panic(err)
	}
}
