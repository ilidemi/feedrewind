package config

import "os"

func developmentConfig() Config {
	wslIp, err := os.ReadFile("config/wsl_ip.txt")
	if err != nil {
		panic(err)
	}
	if len(wslIp) == 0 {
		panic("wsl ip is empty")
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
	}
}
