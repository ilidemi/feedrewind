package config

import (
	"fmt"
	"os"
)

type Config struct {
	Env                       Env
	Dyno                      string
	DB                        DBConfig
	IsHeroku                  bool
	RootUrl                   string
	SessionHashKey            []byte
	SessionBlockKey           []byte
	AmplitudeApiKey           string
	AwsAccessKey              string
	AwsSecretAccessKey        string
	PostmarkApiSandboxToken   string
	PostmarkApiToken          string
	PostmarkWebhookSecret     string
	SlackWebhook              string
	StripeApiKey              string
	StripeWebhookSecret       string
	StripeSupporterConfigId   string
	StripePatronConfigId      string
	StripeCustomBlogProductId string
	StripeCustomBlogPriceId   string
	StripeCustomBlogPrice     string
	TumblrApiKey              string
	AdminUserIds              map[int64]bool
}

type Env int

const (
	EnvDevelopment Env = iota
	EnvTesting
	EnvProduction
)

func (e Env) IsDevOrTest() bool {
	return e == EnvDevelopment || e == EnvTesting
}

type DBConfig struct {
	User          string
	MaybePassword *string
	Host          string
	Port          int
	DBName        string
}

func (c DBConfig) DSN() string {
	password := ""
	if c.MaybePassword != nil {
		password = fmt.Sprintf(" password=%s", *c.MaybePassword)
	}
	return fmt.Sprintf("user=%s%s host=%s port=%d dbname=%s", c.User, password, c.Host, c.Port, c.DBName)
}

const AuthTokenLength = 16

var Cfg Config

func init() {
	if isTesting {
		Cfg = testingConfig()
		return
	}

	_, ok := os.LookupEnv("FEEDREWIND_ENV")
	if !ok {
		Cfg = developmentConfig()
		return
	}

	Cfg = productionConfig()
}
