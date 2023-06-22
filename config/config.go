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
}

type Env int

const (
	EnvDevelopment Env = iota
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

var Cfg Config

func init() {
	env, ok := os.LookupEnv("FEEDREWIND_ENV")
	if !ok {
		Cfg = developmentConfig()
		return
	}

	_ = env
	panic("Production config not supported yet")
}
