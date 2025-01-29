package config

import (
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"strconv"
)

func productionConfig() Config {
	databaseUrl := mustLookupEnv("DATABASE_URL")
	dbUri, err := url.Parse(databaseUrl)
	if err != nil {
		panic(err)
	}
	dbPassword, ok := dbUri.User.Password()
	if !ok {
		panic("Password is not in the database url")
	}
	dbPort, err := strconv.Atoi(dbUri.Port())
	if err != nil {
		panic(err)
	}
	dbName := dbUri.Path[1:]

	sessionHashKeyHex := mustLookupEnv("SESSION_HASH_KEY")
	sessionHashKey, err := hex.DecodeString(sessionHashKeyHex)
	if err != nil {
		panic(err)
	}

	sessionBlockKeyHex := mustLookupEnv("SESSION_BLOCK_KEY")
	sessionBlockKey, err := hex.DecodeString(sessionBlockKeyHex)
	if err != nil {
		panic(err)
	}

	adminUserIdStr := mustLookupEnv("ADMIN_USER_ID")
	adminUserId, err := strconv.ParseInt(adminUserIdStr, 10, 64)
	if err != nil {
		panic(err)
	}

	return Config{
		Env:  EnvProduction,
		Dyno: mustLookupEnv("DYNO"),
		DB: DBConfig{
			User:          dbUri.User.Username(),
			MaybePassword: &dbPassword,
			Host:          dbUri.Hostname(),
			Port:          dbPort,
			DBName:        dbName,
		},
		IsHeroku:                  true,
		RootUrl:                   "https://feedrewind.com",
		SessionHashKey:            sessionHashKey,
		SessionBlockKey:           sessionBlockKey,
		AmplitudeApiKey:           mustLookupEnv("AMPLITUDE_API_KEY"),
		AwsAccessKey:              mustLookupEnv("AWS_ACCESS_KEY"),
		AwsSecretAccessKey:        mustLookupEnv("AWS_SECRET_ACCESS_KEY"),
		PostmarkApiSandboxToken:   "",
		PostmarkApiToken:          mustLookupEnv("POSTMARK_API_TOKEN"),
		PostmarkWebhookSecret:     mustLookupEnv("POSTMARK_WEBHOOK_SECRET"),
		SlackWebhook:              mustLookupEnv("SLACK_WEBHOOK"),
		StripeApiKey:              mustLookupEnv("STRIPE_KEY"),
		StripeWebhookSecret:       mustLookupEnv("STRIPE_WEBHOOK_SECRET"),
		StripeSupporterConfigId:   mustLookupEnv("STRIPE_SUPPORTER_CONFIG_ID"),
		StripePatronConfigId:      mustLookupEnv("STRIPE_PATRON_CONFIG_ID"),
		StripeCustomBlogProductId: mustLookupEnv("STRIPE_CUSTOM_BLOG_PRODUCT_ID"),
		StripeCustomBlogPriceId:   mustLookupEnv("STRIPE_CUSTOM_BLOG_PRICE_ID"),
		StripeCustomBlogPrice:     mustLookupEnv("STRIPE_CUSTOM_BLOG_PRICE"),
		TumblrApiKey:              mustLookupEnv("TUMBLR_API_KEY"),
		AdminUserIds: map[int64]bool{
			adminUserId: true,
		},
	}
}

func mustLookupEnv(key string) string {
	value, ok := os.LookupEnv(key)
	if !ok {
		panic(fmt.Errorf("%s environment variable not set", key))
	}
	return value
}
