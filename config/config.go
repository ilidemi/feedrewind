package config

import (
	"fmt"
	"os"
)

type Config struct {
	Env             Env
	DB              DBConfig
	SessionHashKey  []byte
	SessionBlockKey []byte
	AdminUserIds    map[int64]bool
}

type Env int

const (
	EnvDevelopment Env = iota
	EnvTesting
	EnvProduction
)

type DBConfig struct {
	User     string
	Password *string
	Host     string
	Port     int
	DBName   string
}

func (c DBConfig) DSN() string {
	password := ""
	if c.Password != nil {
		password = fmt.Sprintf(" password=%s", *c.Password)
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

	env, ok := os.LookupEnv("FEEDREWIND_ENV")
	if !ok {
		Cfg = developmentConfig()
		return
	}

	_ = env
	panic("Production config not supported yet")
}
