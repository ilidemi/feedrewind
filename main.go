package main

import (
	"context"
	"feedrewind/cmd"
	"feedrewind/cmd/crawl"
	"feedrewind/config"
	"feedrewind/db"
	"feedrewind/jobs"
	"feedrewind/log"
	frmiddleware "feedrewind/middleware"
	"feedrewind/models"
	"feedrewind/routes"
	"feedrewind/util"
	"feedrewind/util/schedule"
	"fmt"
	"net/http"
	"time"

	_ "net/http/pprof"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/heroku/x/hmetrics"
	"github.com/spf13/cobra"
)

//go:generate go run cmd/timezones/main.go
//go:generate go run third_party/tzdata/generate_zipdata.go

func main() {
	rootCmd := &cobra.Command{
		Use: "feedrewind",
	}

	var port *int
	webCmd := &cobra.Command{
		Use: "web",
		Run: func(_ *cobra.Command, _ []string) {
			runServer(*port)
		},
	}
	port = webCmd.PersistentFlags().Int("port", 3000, "Web Port")

	rootCmd.AddCommand(webCmd)
	rootCmd.AddCommand(jobs.Worker)
	rootCmd.AddCommand(cmd.Db)
	rootCmd.AddCommand(cmd.Tailwind)
	rootCmd.AddCommand(cmd.WslStartup)
	rootCmd.AddCommand(crawl.Crawl)
	rootCmd.AddCommand(&cobra.Command{
		Use: "log-stalled-jobs",
		Run: func(_ *cobra.Command, _ []string) {
			logStalledJobs()
		},
	})

	if err := rootCmd.Execute(); err != nil {
		panic(err)
	}
}

func runServer(port int) {
	if config.Cfg.Env.IsDevOrTest() {
		go func() {
			fmt.Println(http.ListenAndServe("localhost:6060", nil))
		}()
	}

	logger := &log.BackgroundLogger{}
	if config.Cfg.IsHeroku {
		// Adapted from https://github.com/heroku/x/blob/v0.1.0/hmetrics/onload/init.go
		go func() {
			var errorHandler hmetrics.ErrHandler = func(err error) error {
				logger.Error().Err(err).Msg("Error sending heroku metrics")
				return nil
			}
			for backoff := int64(1); ; backoff++ {
				start := time.Now()
				err := hmetrics.Report(context.Background(), hmetrics.DefaultEndpoint, errorHandler)
				if time.Since(start) > 5*time.Minute {
					backoff = 1
				}
				if err != nil {
					_ = errorHandler(err)
				}
				time.Sleep(time.Duration(backoff*10) * time.Second)
			}
		}()
	}

	conn, err := db.Pool.AcquireBackground()
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
	staticR.Use(frmiddleware.RedirectHttpToHttps)
	staticR.Use(middleware.GetHead)
	staticR.Use(frmiddleware.DB)

	staticR.Group(func(r chi.Router) {
		r.Use(frmiddleware.Session)
		r.Use(frmiddleware.CurrentUser)
		r.Use(frmiddleware.CSRF)
		r.Use(frmiddleware.EmitVisit)

		r.Get("/", routes.Landing_Index)

		r.Get(util.LoginPath, routes.Login_Page)
		r.Post(util.LoginPath, routes.Login)
		r.Get("/logout", routes.Logout)
		r.Get(util.SignUpPath, routes.SignUp_Page)
		r.Post(util.SignUpPath, routes.SignUp)

		r.Get("/subscriptions/add", routes.Onboarding_Add)
		r.Post("/subscriptions/add/{start_url}", routes.Onboarding_Add)
		r.Post("/subscriptions/add", routes.Onboarding_AddLanding)
		r.Post("/subscriptions/discover_feeds", routes.Onboarding_DiscoverFeeds)
		r.Get("/preview/{slug}", routes.Onboarding_Preview)

		r.Get("/subscriptions/{id:\\d+}/setup", routes.Subscriptions_Setup)
		r.Post("/subscriptions", routes.Subscriptions_Create)
		r.Post("/subscriptions/{id:\\d+}/progress", routes.Subscriptions_Progress)
		r.Post("/subscriptions/{id:\\d+}/submit_progress_times", routes.Subscriptions_SubmitProgressTimes)
		r.Post("/subscriptions/{id:\\d+}/select_posts", routes.Subscriptions_SelectPosts)
		r.Post("/subscriptions/{id:\\d+}/mark_wrong", routes.Subscriptions_MarkWrong)
		r.Post("/subscriptions/{id:\\d+}/schedule", routes.Subscriptions_Schedule)
		r.Post("/subscriptions/{id:\\d+}/delete", routes.Subscriptions_Delete)
		r.Get("/subscriptions/{id:\\d+}/progress_stream", routes.Subscriptions_ProgressStream)

		r.Get("/blogs/{id}/unsupported", routes.Blogs_Unsupported)

		r.Get("/terms", routes.Misc_Terms)
		r.Get("/privacy", routes.Misc_Privacy)
		r.Get("/about", routes.Misc_About)

		r.Group(func(authorized chi.Router) {
			authorized.Use(frmiddleware.Authorize)

			authorized.Get("/subscriptions", routes.Subscriptions_Index)
			authorized.Get("/subscriptions/{id:\\d+}", routes.Subscriptions_Show)
			authorized.Post("/subscriptions/{id:\\d+}", routes.Subscriptions_Update)
			authorized.Post("/subscriptions/{id:\\d+}/pause", routes.Subscriptions_Pause)
			authorized.Post("/subscriptions/{id:\\d+}/unpause", routes.Subscriptions_Unpause)

			authorized.Get("/settings", routes.UserSettings_Page)
			authorized.Post("/settings/save_timezone", routes.UserSettings_SaveTimezone)
			authorized.Post("/settings/save_delivery_channel", routes.UserSettings_SaveDeliveryChannel)
		})

		r.Group(func(admin chi.Router) {
			admin.Use(frmiddleware.AuthorizeAdmin)

			admin.Get("/admin/add_blog", routes.Admin_AddBlog)
			admin.Post("/admin/post_blog", routes.Admin_PostBlog)

			admin.Get("/admin/dashboard", routes.Admin_Dashboard)
		})

		if config.Cfg.Env.IsDevOrTest() {
			r.Get("/test/reschedule_user_job", routes.AdminTest_RescheduleUserJob)
			r.Get("/test/run_reset_failed_blogs_job", routes.AdminTest_RunResetFailedBlogsJob)
			r.Get("/test/destroy_user_subscriptions", routes.AdminTest_DestroyUserSubscriptions)
			r.Get("/test/destroy_user", routes.AdminTest_DestroyUser)
			r.Get("/test/set_email_metadata", routes.AdminTest_SetEmailMetadata)
			r.Get("/test/assert_email_count_with_metadata", routes.AdminTest_AssertEmailCountWithMetadata)
			r.Get("/test/delete_email_metadata", routes.AdminTest_DeleteEmailMetadata)
			r.Get("/test/travel_to", routes.AdminTest_TravelTo)
			r.Get("/test/travel_back", routes.AdminTest_TravelBack)
			r.Get("/test/wait_for_publish_posts_job", routes.AdminTest_WaitForPublishPostsJob)
			r.Get("/test/execute_sql", routes.AdminTest_ExecuteSql)
		}
	})

	staticR.Group(func(anonR chi.Router) {
		anonR.Use(frmiddleware.DB)

		anonR.Get("/subscriptions/{id}/feed", routes.Rss_SubscriptionFeed) // Legacy
		anonR.Get("/feeds/single/{id}", routes.Rss_UserFeed)
		anonR.Get("/feeds/{id}", routes.Rss_SubscriptionFeed)

		anonR.Get("/posts/{slug}/{random_id:[A-Za-z0-9_-]+}/", routes.Posts_Post)

		anonR.Post("/postmark/report_bounce", routes.Postmark_ReportBounce)
	})

	staticR.Get(util.StaticRouteTemplate, routes.Static_File)
	staticR.NotFound(routes.Misc_NotFound)

	logger.Info().Msgf("Started on port %d", port)
	addr := fmt.Sprintf(":%d", port)
	if err := http.ListenAndServe(addr, staticR); err != nil {
		panic(err)
	}
}

func logStalledJobs() {
	logger := &log.BackgroundLogger{}
	logger.Info().Msg("Checking for stalled jobs")
	conn, err := db.Pool.AcquireBackground()
	if err != nil {
		panic(err)
	}
	defer conn.Release()

	hourAgo := schedule.UTCNow().Add(-1 * time.Hour)
	rows, err := conn.Query(`
		select id, handler from delayed_jobs where locked_at < $1
	`, hourAgo)
	if err != nil {
		panic(err)
	}
	for rows.Next() {
		var id int64
		var handler string
		err := rows.Scan(&id, &handler)
		if err != nil {
			panic(err)
		}
		logger.Warn().Msgf("Stalled job (%d): %s", id, handler)
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}
}
