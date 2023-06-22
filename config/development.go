package config

import (
	"encoding/hex"
	"os"
)

func developmentConfig() Config {
	wslIp, err := os.ReadFile("config/wsl_ip.txt")
	if err != nil {
		panic(err)
	}
	if len(wslIp) == 0 {
		panic("wsl ip is empty")
	}

	sessionHashKey, err := hex.DecodeString("REDACTED_DEV_SESSION_HASH_KEY")
	if err != nil {
		panic(err)
	}
	sessionBlockKey, err := hex.DecodeString("REDACTED_DEV_SESSION_BLOCK_KEY")
	if err != nil {
		panic(err)
	}

	return Config{
		Env: EnvDevelopment,
		DB: DBConfig{
			User:     "postgres",
			Password: nil,
			Host:     string(wslIp),
			Port:     5432,
			DBName:   "rss_catchup_rails_development",
		},
		SessionHashKey:  sessionHashKey,
		SessionBlockKey: sessionBlockKey,
	}
}
