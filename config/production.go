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
		IsHeroku:                true,
		SessionHashKey:          sessionHashKey,
		SessionBlockKey:         sessionBlockKey,
		AmplitudeApiKey:         "REDACTED_AMPLITUDE_API_KEY",
		PostmarkApiSandboxToken: "",
		PostmarkApiToken:        mustLookupEnv("POSTMARK_API_TOKEN"),
		PostmarkWebhookSecret:   mustLookupEnv("POSTMARK_WEBHOOK_SECRET"),
		SlackWebhook:            mustLookupEnv("SLACK_WEBHOOK"),
		AdminUserIds: map[int64]bool{
			6835322936850076956: true, // belk94@gmail.com
			6862710086337347875: true, // test@test.com
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
