//go:build testing

package config

const isTesting = true

func testingConfig() Config {
	devCfg := developmentConfig()
	return Config{
		Env:  EnvTesting,
		Dyno: devCfg.Dyno,
		DB: DBConfig{
			User:          devCfg.DB.User,
			MaybePassword: devCfg.DB.MaybePassword,
			Host:          devCfg.DB.Host,
			Port:          devCfg.DB.Port,
			DBName:        "rss_catchup_rails_test",
		},
		IsHeroku:                  false,
		RootUrl:                   devCfg.RootUrl,
		SessionHashKey:            devCfg.SessionHashKey,
		SessionBlockKey:           devCfg.SessionBlockKey,
		AmplitudeApiKey:           devCfg.AmplitudeApiKey,
		AwsAccessKey:              devCfg.AwsAccessKey,
		AwsSecretAccessKey:        devCfg.AwsSecretAccessKey,
		PostmarkApiSandboxToken:   devCfg.PostmarkApiSandboxToken,
		PostmarkApiToken:          devCfg.PostmarkApiToken,
		PostmarkWebhookSecret:     devCfg.PostmarkWebhookSecret,
		SlackWebhook:              devCfg.SlackWebhook,
		StripeApiKey:              devCfg.StripeApiKey,
		StripeWebhookSecret:       devCfg.StripeWebhookSecret,
		StripeSupporterConfigId:   devCfg.StripeSupporterConfigId,
		StripePatronConfigId:      devCfg.StripePatronConfigId,
		StripeCustomBlogProductId: devCfg.StripeCustomBlogProductId,
		StripeCustomBlogPriceId:   devCfg.StripeCustomBlogPriceId,
		StripeCustomBlogPrice:     devCfg.StripeCustomBlogPrice,
		AdminUserIds:              nil,
	}
}
