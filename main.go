package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"

	_ "net/http/pprof"

	"feedrewind.com/cmd"
	"feedrewind.com/cmd/crawl"
	"feedrewind.com/config"
	"feedrewind.com/db"
	"feedrewind.com/jobs"
	"feedrewind.com/log"
	frmiddleware "feedrewind.com/middleware"
	"feedrewind.com/models"
	"feedrewind.com/routes"
	"feedrewind.com/util"
	"feedrewind.com/util/schedule"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/spf13/cobra"
	"github.com/stripe/stripe-go/v78"
)

//go:generate go run cmd/timezones/main.go
//go:generate go run third_party/tzdata/date_zipdata.go

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
	rootCmd.AddCommand(crawl.CrawlRobots)
	rootCmd.AddCommand(crawl.PuppeteerScaleTest)
	rootCmd.AddCommand(crawl.HN1000ScaleTest)
	rootCmd.AddCommand(&cobra.Command{
		Use: "log-stalled-jobs",
		Run: func(_ *cobra.Command, _ []string) {
			logStalledJobs()
		},
	})
	rootCmd.AddCommand(&cobra.Command{
		Use: "stripe-listen",
		Run: func(_ *cobra.Command, _ []string) {
			stripeCmd := exec.Command("stripe", "listen", "--forward-to", "localhost:3000/stripe/webhook")
			stripeCmd.Stdout = os.Stdout
			stripeCmd.Stderr = os.Stderr
			if err := stripeCmd.Run(); err != nil {
				panic(err)
			}
		},
	})
	rootCmd.AddCommand(&cobra.Command{
		Use: "demo-seed-db",
		Run: func(_ *cobra.Command, _ []string) {
			conn, err := db.RootPool.AcquireBackground()
			if err != nil {
				panic(err)
			}
			defer conn.Release()

			tx, err := conn.Begin()
			if err != nil {
				panic(err)
			}
			_, err = tx.Exec(`
				insert into pricing_plans (id, default_offer_id)
				values ('free', 'free_demo'), ('supporter', 'supporter_demo'), ('patron', 'patron_demo')
			`)
			if err != nil {
				panic(err)
			}
			_, err = tx.Exec(`
				insert into pricing_offers (id, monthly_rate, yearly_rate, plan_id)
				values ('free_demo', '$0.00', '$0.00', 'free'),
					('supporter_demo', '$1.00', '$10.00', 'supporter'),
					('patron_demo', '$10.00', '$100.00', 'patron')
			`)
			if err != nil {
				panic(err)
			}
			err = tx.Commit()
			if err != nil {
				panic(err)
			}
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

	logger := log.NewBackgroundLogger()
	if config.Cfg.IsHeroku {
		go util.ReportHerokuMetrics(logger)
	}

	models.MustInit(db.RootPool)
	frmiddleware.MustInitBannedIps()
	routes.Subscriptions_MustStartListeningForNotifications()
	parentCtx := context.Background()
	signalCtx, signalCancel := signal.NotifyContext(parentCtx, syscall.SIGINT, syscall.SIGTERM)
	defer signalCancel()
	var wg sync.WaitGroup
	wg.Add(1)
	models.ProductEvent_StartDummyEventsSync(signalCtx, &wg)

	stripe.Key = config.Cfg.StripeApiKey
	stripe.DefaultLeveledLogger = &log.StripeLogger{Logger: logger}

	staticR := chi.NewRouter()
	staticR.Use(frmiddleware.Logger)
	staticR.Use(frmiddleware.IpBan)
	staticR.Use(middleware.Compress(3))
	staticR.Use(frmiddleware.Recoverer)
	staticR.Use(frmiddleware.DefaultHeaders)
	staticR.Use(frmiddleware.RedirectHttpToHttps)
	staticR.Use(middleware.GetHead)
	staticR.Use(frmiddleware.DB)
	staticR.Use(frmiddleware.RedirectSlashes("/subscriptions/add/"))

	staticR.Group(func(r chi.Router) {
		r.Use(frmiddleware.Session)
		r.Use(frmiddleware.CurrentUser)
		r.Use(frmiddleware.CSRF)
		r.Use(frmiddleware.EmitVisit)

		r.Get("/", routes.Landing_Index)
		r.Get("/hmn", routes.Landing_HMN)

		r.Get(util.LoginPath, routes.Users_LoginPage)
		r.Post(util.LoginPath, routes.Users_Login)
		r.Get("/logout", routes.Users_Logout)
		r.Get(util.SignUpPath, routes.Users_SignUpPage)
		r.Post(util.SignUpPath, routes.Users_SignUp)

		r.Get("/subscriptions/add", routes.Onboarding_Add)
		r.Post("/subscriptions/add", routes.Onboarding_AddLanding)
		// For the convenience of testing and sharing links
		r.Get("/subscriptions/add/{start_url}", routes.Onboarding_Add)
		r.Post("/subscriptions/add/{start_url}", routes.Onboarding_Add)
		r.Post("/subscriptions/discover_feeds", routes.Onboarding_DiscoverFeeds)
		r.Get("/preview/{slug}", routes.Onboarding_Preview)
		r.Get("/pricing", routes.Onboarding_Pricing)
		r.Post("/checkout", routes.Onboarding_Checkout)

		r.Get("/subscriptions/{id:\\d+}/setup", routes.Subscriptions_Setup)
		r.Post("/subscriptions", routes.Subscriptions_Create)
		r.Post("/subscriptions/{id:\\d+}/progress", routes.Subscriptions_Progress)
		r.Post("/subscriptions/{id:\\d+}/submit_progress_times", routes.Subscriptions_SubmitProgressTimes)
		r.Post("/subscriptions/{id:\\d+}/select_posts", routes.Subscriptions_SelectPosts)
		r.Post("/subscriptions/{id:\\d+}/mark_wrong", routes.Subscriptions_MarkWrong)
		r.Post("/subscriptions/{id:\\d+}/schedule", routes.Subscriptions_Schedule)
		r.Post("/subscriptions/{id:\\d+}/delete", routes.Subscriptions_Delete)
		r.Get("/subscriptions/{id:\\d+}/progress_stream", routes.Subscriptions_ProgressStream)
		r.Post("/subscriptions/{id:\\d+}/notify_when_supported", routes.Subscriptions_NotifyWhenSupported)

		r.Get("/terms", routes.Misc_Terms)
		r.Get("/privacy", routes.Misc_Privacy)
		r.Get("/subprocessors", routes.Misc_Subprocessors)
		r.Get("/about", routes.Misc_About)
		r.Get("/bot", routes.Misc_Bot)

		r.Group(func(authorized chi.Router) {
			authorized.Use(frmiddleware.Authorize)

			authorized.Get("/subscriptions", routes.Subscriptions_Index)
			authorized.Get("/subscriptions/{id:\\d+}", routes.Subscriptions_Show)
			authorized.Post("/subscriptions/{id:\\d+}", routes.Subscriptions_Update)
			authorized.Post("/subscriptions/{id:\\d+}/pause", routes.Subscriptions_Pause)
			authorized.Post("/subscriptions/{id:\\d+}/unpause", routes.Subscriptions_Unpause)
			authorized.Get("/subscriptions/{id:\\d+}/request", routes.Subscriptions_RequestCustomBlogPage)
			authorized.Post(
				"/subscriptions/{id:\\d+}/checkout", routes.Subscriptions_CheckoutCustomBlogRequest,
			)
			authorized.Get(
				"/subscriptions/{id:\\d+}/submit_request", routes.Subscriptions_SubmitCustomBlogRequest,
			)
			authorized.Post(
				"/subscriptions/{id:\\d+}/submit_request", routes.Subscriptions_SubmitCustomBlogRequest,
			)

			authorized.Get("/settings", routes.UserSettings_Page)
			authorized.Post("/settings/save_timezone", routes.UserSettings_SaveTimezone)
			authorized.Post("/settings/save_delivery_channel", routes.UserSettings_SaveDeliveryChannel)
			authorized.Get("/billing", routes.UserSettings_Billing)
			authorized.Get("/billing_full", routes.UserSettings_BillingFull)
			authorized.Get("/upgrade", routes.Users_Upgrade)
			authorized.Post("/delete_account", routes.Users_DeleteAccount)
		})

		r.Group(func(admin chi.Router) {
			admin.Use(frmiddleware.AuthorizeAdmin)

			admin.Get("/admin/add_blog", routes.Admin_AddBlog)
			admin.Post("/admin/post_blog", routes.Admin_PostBlog)

			admin.Get("/admin/dashboard", routes.Admin_Dashboard)
			admin.Post("/admin/job/{id:\\d+}/delete", routes.Admin_DeleteJob)
		})

		if config.Cfg.Env.IsDevOrTest() {
			r.Get("/test/reschedule_user_job", routes.AdminTest_RescheduleUserJob)
			r.Get("/test/run_reset_failed_blogs_job", routes.AdminTest_RunResetFailedBlogsJob)
			r.Get("/test/destroy_user_subscriptions", routes.AdminTest_DestroyUserSubscriptions)
			r.Get("/test/destroy_user", routes.AdminTest_DestroyUser)
			r.Get("/test/get_test_singleton", routes.AdminTest_GetTestSingleton)
			r.Get("/test/set_test_singleton", routes.AdminTest_SetTestSingleton)
			r.Get("/test/delete_test_singleton", routes.AdminTest_DeleteTestSingleton)
			r.Get("/test/assert_email_count_with_metadata", routes.AdminTest_AssertEmailCountWithMetadata)
			r.Get("/test/travel_to", routes.AdminTest_TravelTo)
			r.Get("/test/travel_back", routes.AdminTest_TravelBack)
			r.Get("/test/wait_for_publish_posts_job", routes.AdminTest_WaitForPublishPostsJob)
			r.Get("/test/execute_sql", routes.AdminTest_ExecuteSql)
			r.Get("/test/ensure_stripe_listen", routes.AdminTest_EnsureStripeListen)
			r.Get("/test/delete_stripe_subscription", routes.AdminTest_DeleteStripeSubscription)
			r.Get("/test/forward_stripe_customer", routes.AdminTest_ForwardStripeCustomer)
			r.Get("/test/delete_stripe_clocks", routes.AdminTest_DeleteStripeClocks)
		}
	})

	staticR.Group(func(anonR chi.Router) {
		anonR.Get("/subscriptions/{id}/feed", routes.Rss_SubscriptionFeed) // Legacy
		anonR.Get("/feeds/single/{id}", routes.Rss_UserFeed)
		anonR.Get("/feeds/{id}", routes.Rss_SubscriptionFeed)

		anonR.Get("/posts/{slug}/{random_id:[A-Za-z0-9_-]+}", routes.Posts_Post)

		anonR.Post("/postmark/report_bounce", routes.Webhooks_PostmarkReportBounce)
		anonR.Post("/stripe/webhook", routes.Webhooks_Stripe)
	})

	staticR.Get(util.StaticRouteTemplate, routes.Static_File)
	staticR.Get("/robots.txt", routes.Static_RobotsTxt)
	staticR.NotFound(routes.Misc_NotFound)

	logger.Info().Msgf("Started on port %d", port)
	server := http.Server{ //nolint:exhaustruct
		Addr:        fmt.Sprintf(":%d", port),
		BaseContext: func(net.Listener) context.Context { return parentCtx },
		Handler:     staticR,
	}
	go server.ListenAndServe() //nolint:errcheck
	<-signalCtx.Done()
	shutdownCtx, shutdownCancel := context.WithTimeout(parentCtx, 5*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		panic(err)
	}
	wg.Wait()
}

func logStalledJobs() {
	logger := log.TaskLogger{TaskName: "log_stalled_jobs"}
	logger.Info().Msg("Checking for stalled jobs")

	hourAgo := schedule.UTCNow().Add(-1 * time.Hour)
	rows, err := db.RootPool.Query(`
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
