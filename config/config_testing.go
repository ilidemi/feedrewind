//go:build testing

package config

const isTesting = true

func testingConfig() Config {
	devCfg := developmentConfig()
	return Config{
		Env: EnvTesting,
		DB: DBConfig{
			User:     devCfg.DB.User,
			Password: devCfg.DB.Password,
			Host:     devCfg.DB.Host,
			Port:     devCfg.DB.Port,
			DBName:   "rss_catchup_rails_test",
		},
		SessionHashKey:          devCfg.SessionHashKey,
		SessionBlockKey:         devCfg.SessionBlockKey,
		PostmarkApiSandboxToken: devCfg.PostmarkApiSandboxToken,
		PostmarkWebhookSecret:   devCfg.PostmarkWebhookSecret,
		AdminUserIds:            nil,
	}
}
