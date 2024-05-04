package config

import (
	"encoding/hex"
	"fmt"
	"os"

	"github.com/stripe/stripe-go/v78"
)

func DevelopmentDBConfig() DBConfig {
findRoot:
	for i := 0; ; i++ {
		if i > 100 {
			panic("Something went wrong when looking for the feedrewind root dir")
		}
		cwd, err := os.Getwd()
		if err != nil {
			panic(err)
		}
		entries, err := os.ReadDir(cwd)
		if err != nil {
			panic(err)
		}
		for _, entry := range entries {
			if (entry.Type()&os.ModeDir == 0) && entry.Name() == "Procfile" {
				break findRoot
			}
		}
		err = os.Chdir("..")
		if err != nil {
			panic(err)
		}
	}
	wslIp, err := os.ReadFile("config/wsl_ip.txt")
	if err != nil {
		panic(err)
	}
	if len(wslIp) == 0 {
		panic("wsl ip is empty")
	}

	return DBConfig{
		User:          "postgres",
		MaybePassword: nil,
		Host:          string(wslIp),
		Port:          5432,
		DBName:        "rss_catchup_rails_development",
	}
}

func developmentConfig() Config {
	pid := os.Getpid()
	dyno := fmt.Sprintf("local.%d", pid)
	dbConfig := DevelopmentDBConfig()
	sessionHashKey, err := hex.DecodeString("REDACTED_DEV_SESSION_HASH_KEY")
	if err != nil {
		panic(err)
	}
	sessionBlockKey, err := hex.DecodeString("REDACTED_DEV_SESSION_BLOCK_KEY")
	if err != nil {
		panic(err)
	}

	stripe.Key = "REDACTED_DEV_STRIPE_API_KEY"
	return Config{
		Env:                     EnvDevelopment,
		Dyno:                    dyno,
		DB:                      dbConfig,
		IsHeroku:                false,
		RootUrl:                 "http://localhost:3000",
		SessionHashKey:          sessionHashKey,
		SessionBlockKey:         sessionBlockKey,
		AmplitudeApiKey:         "REDACTED_DEV_AMPLITUDE_API_KEY",
		PostmarkApiSandboxToken: "REDACTED_DEV_POSTMARK_API_SANDBOX_TOKEN",
		PostmarkApiToken:        "REDACTED_DEV_POSTMARK_API_TOKEN", // FeedRewindDevelopment
		PostmarkWebhookSecret:   "REDACTED_DEV_POSTMARK_WEBHOOK_SECRET",
		SlackWebhook:            "REDACTED_DEV_SLACK_WEBHOOK",
		StripeWebhookSecret:     "REDACTED_DEV_STRIPE_WEBHOOK_SECRET",
		AdminUserIds:            nil,
	}
}
