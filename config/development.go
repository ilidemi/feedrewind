package config

import (
	"encoding/hex"
	"os"
	"strings"
)

func DevelopmentDBConfig() DBConfig {
	for i := 0; ; i++ {
		if i > 100 {
			panic("Something went wrong when looking for the feedrewind root dir")
		}
		cwd, err := os.Getwd()
		if err != nil {
			panic(err)
		}
		if strings.HasSuffix(cwd, "/feedrewind") || strings.HasSuffix(cwd, "\\feedrewind") {
			break
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
		User:     "postgres",
		Password: nil,
		Host:     string(wslIp),
		Port:     5432,
		DBName:   "rss_catchup_rails_development",
	}
}

func developmentConfig() Config {
	dbConfig := DevelopmentDBConfig()
	sessionHashKey, err := hex.DecodeString("REDACTED_DEV_SESSION_HASH_KEY")
	if err != nil {
		panic(err)
	}
	sessionBlockKey, err := hex.DecodeString("REDACTED_DEV_SESSION_BLOCK_KEY")
	if err != nil {
		panic(err)
	}

	return Config{
		Env:                     EnvDevelopment,
		DB:                      dbConfig,
		IsHeroku:                false,
		SessionHashKey:          sessionHashKey,
		SessionBlockKey:         sessionBlockKey,
		AmplitudeApiKey:         "REDACTED_DEV_AMPLITUDE_API_KEY",
		PostmarkApiSandboxToken: "REDACTED_DEV_POSTMARK_API_SANDBOX_TOKEN",
		PostmarkApiToken:        "REDACTED_DEV_POSTMARK_API_TOKEN", // FeedRewindDevelopment
		PostmarkWebhookSecret:   "REDACTED_DEV_POSTMARK_WEBHOOK_SECRET",
		SlackWebhook:            "REDACTED_DEV_SLACK_WEBHOOK",
		AdminUserIds:            nil,
	}
}
