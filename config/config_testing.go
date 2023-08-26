//go:build testing

package config

const isTesting = true

func testingConfig() Config {
	developmentCfg := developmentConfig()
	developmentCfg.DB.DBName = "rss_catchup_rails_test"
	return developmentCfg
}
