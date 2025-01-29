package config

// Note: config management is clunky because the code was structured to run on a specific dev machine with
// the least friction, then pushed to GitHub much later on

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"

	"github.com/goccy/go-json"
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
	var wslIp string
	wslIpBytes, err := os.ReadFile("config/wsl_ip.txt")
	if errors.Is(err, os.ErrNotExist) {
		// no-op
	} else if err != nil {
		panic(err)
	} else if len(wslIpBytes) == 0 {
		panic("wsl ip is empty")
	} else {
		wslIp = string(wslIpBytes)
	}

	jsonConfig := mustReadJsonConfig()
	var maybePassword *string
	password := jsonConfig["db_password"]
	if password != nil {
		passwordStr := password.(string)
		maybePassword = &passwordStr
	}
	host := jsonConfig["db_host"].(string)
	if wslIp != "" {
		host = wslIp
	}

	return DBConfig{
		User:          jsonConfig["db_user"].(string),
		MaybePassword: maybePassword,
		Host:          host,
		Port:          int(jsonConfig["db_port"].(float64)),
		DBName:        jsonConfig["db_name"].(string),
	}
}

const DemoValue = "DEMO"

func developmentConfig() Config {
	pid := os.Getpid()
	dyno := fmt.Sprintf("local.%d", pid)
	dbConfig := DevelopmentDBConfig()
	jsonConfig := mustReadJsonConfig()
	sessionHashKey, err := hex.DecodeString(jsonConfig["session_hash_key"].(string))
	if err != nil {
		panic(err)
	}
	sessionBlockKey, err := hex.DecodeString(jsonConfig["session_block_key"].(string))
	if err != nil {
		panic(err)
	}

	getStringOrDemo := func(key string) string {
		if value, ok := jsonConfig[key]; ok {
			return value.(string)
		}
		return DemoValue
	}

	return Config{
		Env:                       EnvDevelopment,
		Dyno:                      dyno,
		DB:                        dbConfig,
		IsHeroku:                  false,
		RootUrl:                   "http://localhost:3000",
		SessionHashKey:            sessionHashKey,
		SessionBlockKey:           sessionBlockKey,
		AmplitudeApiKey:           getStringOrDemo("amplitude_api_key"),
		AwsAccessKey:              getStringOrDemo("aws_access_key"),
		AwsSecretAccessKey:        getStringOrDemo("aws_secret_access_key"),
		PostmarkApiSandboxToken:   getStringOrDemo("postmark_api_sandbox_token"),
		PostmarkApiToken:          getStringOrDemo("postmark_api_token"),
		PostmarkWebhookSecret:     getStringOrDemo("postmark_webhook_secret"),
		SlackWebhook:              getStringOrDemo("slack_webhook"),
		StripeApiKey:              getStringOrDemo("stripe_api_key"),
		StripeWebhookSecret:       getStringOrDemo("stripe_webhook_secret"),
		StripeSupporterConfigId:   getStringOrDemo("stripe_supporter_config_id"),
		StripePatronConfigId:      getStringOrDemo("stripe_patron_config_id"),
		StripeCustomBlogProductId: getStringOrDemo("stripe_custom_blog_product_id"),
		StripeCustomBlogPriceId:   getStringOrDemo("stripe_custom_blog_price_id"),
		StripeCustomBlogPrice:     getStringOrDemo("stripe_custom_blog_price"),
		TumblrApiKey:              getStringOrDemo("tumblr_api_key"),
		AdminUserIds:              nil,
	}
}

func mustReadJsonConfig() map[string]any {
	configBytes, err := os.ReadFile("config/devbox.json")
	if errors.Is(err, os.ErrNotExist) {
		configBytes, err = os.ReadFile("config/demo.json")
		if err != nil {
			panic(err)
		}
	} else if err != nil {
		panic(err)
	}

	var jsonConfig map[string]any
	err = json.Unmarshal(configBytes, &jsonConfig)
	if err != nil {
		panic(err)
	}
	return jsonConfig
}
