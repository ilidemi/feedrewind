package main

import (
	"context"
	"feedrewind/db"
	"feedrewind/log"
	frmiddleware "feedrewind/middleware"
	"feedrewind/models"
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
	conn, err := db.Pool.Acquire(context.Background())
	if err != nil {
		panic(err)
	}
	models.MustInit(conn)
	conn.Release()

	staticR := chi.NewRouter()
	staticR.Use(frmiddleware.Logger)
	staticR.Use(middleware.Compress(5))
	staticR.Use(frmiddleware.Recoverer)
	staticR.Use(frmiddleware.DefaultHeaders)
	staticR.Use(middleware.GetHead)

	staticR.Group(func(r chi.Router) {
		r.Use(frmiddleware.DB)
		r.Use(frmiddleware.Session)
		r.Use(frmiddleware.CurrentUser)
		r.Use(frmiddleware.CSRF)

		r.Get("/", routes.LandingIndex)
		r.Get(util.LoginPath, routes.LoginPage)
		r.Post(util.LoginPath, routes.Login)
		r.Get("/logout", routes.Logout)
		r.Get(util.SignUpPath, routes.SignUpPage)
		r.Post(util.SignUpPath, routes.SignUp)

		r.Group(func(authorized chi.Router) {
			authorized.Use(frmiddleware.Authorize)

			authorized.Get("/subscriptions", routes.SubscriptionsIndex)
			authorized.Get("/subscriptions/{id}", routes.SubscriptionsShow)
			authorized.Post("/subscriptions/{id}", routes.SubscriptionsUpdate)
			authorized.Post("/subscriptions/{id}/pause", routes.SubscriptionsPause)
			authorized.Post("/subscriptions/{id}/unpause", routes.SubscriptionsUnpause)

			authorized.Get("/settings", routes.SettingsPage)
			authorized.Post("/settings/save_timezone", routes.SettingsSaveTimezone)
			authorized.Post("/settings/save_delivery_channel", routes.SettingsSaveDeliveryChannel)
		})
		r.Post("/subscriptions/{id:\\d+}/delete", routes.SubscriptionsDelete)
	})

	staticR.Get(util.StaticRouteTemplate, routes.StaticFile)

	log.Info().Msg("Started")
	if err := http.ListenAndServe(":3000", staticR); err != nil {
		panic(err)
	}
}
