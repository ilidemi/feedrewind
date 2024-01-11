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
		IsHeroku:                false,
		SessionHashKey:          devCfg.SessionHashKey,
		SessionBlockKey:         devCfg.SessionBlockKey,
		AmplitudeApiKey:         devCfg.AmplitudeApiKey,
		PostmarkApiSandboxToken: devCfg.PostmarkApiSandboxToken,
		PostmarkApiToken:        devCfg.PostmarkApiToken,
		PostmarkWebhookSecret:   devCfg.PostmarkWebhookSecret,
		SlackWebhook:            devCfg.SlackWebhook,
		AdminUserIds:            nil,
	}
}
