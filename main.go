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
	staticR.Use(middleware.Compress(3))
	staticR.Use(frmiddleware.Recoverer)
	staticR.Use(frmiddleware.DefaultHeaders)
	staticR.Use(middleware.GetHead)

	staticR.Group(func(r chi.Router) {
		r.Use(frmiddleware.DB)
		r.Use(frmiddleware.Session)
		r.Use(frmiddleware.CurrentUser)
		r.Use(frmiddleware.CSRF)

		r.Get("/", routes.Landing_Index)

		r.Get(util.LoginPath, routes.Login_Page)
		r.Post(util.LoginPath, routes.Login)
		r.Get("/logout", routes.Logout)
		r.Get(util.SignUpPath, routes.SignUp_Page)
		r.Post(util.SignUpPath, routes.SignUp)

		r.Get("/subscriptions/add", routes.Onboarding_Add)
		r.Get("/subscriptions/add/{start_url}", routes.Onboarding_Add)
		r.Post("/subscriptions/add", routes.Onboarding_AddLanding)
		r.Post("/subscriptions/discover_feeds", routes.Onboarding_DiscoverFeeds)
		r.Get("/preview/{slug}", routes.Onboarding_Preview)

		r.Get("/subscriptions/{id}/setup", routes.Subscriptions_Setup)
		r.Post("/subscriptions", routes.Subscriptions_Create)
		r.Post("/subscriptions/{id}/progress", routes.Subscriptions_Progress)
		r.Post("/subscriptions/{id}/submit_progress_times", routes.Subscriptions_SubmitProgressTimes)
		r.Post("/subscriptions/{id}/select_posts", routes.Subscriptions_SelectPosts)
		r.Post("/subscriptions/{id}/mark_wrong", routes.Subscriptions_MarkWrong)
		r.Post("/subscriptions/{id}/schedule", routes.Subscriptions_Schedule)

		r.Get("/subscriptions/{id}/feed", routes.Rss_SubscriptionFeed) // Legacy
		r.Get("/feeds/single/{id}", routes.Rss_UserFeed)
		r.Get("/feeds/{id}", routes.Rss_SubscriptionFeed)

		r.Get("/blogs/{id}/unsupported", routes.Blogs_Unsupported)

		r.Group(func(authorized chi.Router) {
			authorized.Use(frmiddleware.Authorize)

			authorized.Get("/subscriptions", routes.Subscriptions_Index)
			authorized.Get("/subscriptions/{id}", routes.Subscriptions_Show)
			authorized.Post("/subscriptions/{id}", routes.Subscriptions_Update)
			authorized.Post("/subscriptions/{id}/pause", routes.Subscriptions_Pause)
			authorized.Post("/subscriptions/{id}/unpause", routes.Subscriptions_Unpause)

			authorized.Get("/settings", routes.UserSettings_Page)
			authorized.Post("/settings/save_timezone", routes.UserSettings_SaveTimezone)
			authorized.Post("/settings/save_delivery_channel", routes.UserSettings_SaveDeliveryChannel)
		})
		r.Post("/subscriptions/{id:\\d+}/delete", routes.Subscriptions_Delete)
	})

	staticR.Get(util.StaticRouteTemplate, routes.Static_File)

	log.Info().Msg("Started")
	if err := http.ListenAndServe(":3000", staticR); err != nil {
		panic(err)
	}
}
