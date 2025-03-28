package db

import (
	"context"

	"feedrewind.com/config"
	"feedrewind.com/db/migrations"
	"feedrewind.com/db/pgw"
	"feedrewind.com/log"
	"feedrewind.com/oops"
)

var RootPool *pgw.Pool

func init() {
	var err error
	RootPool, err = pgw.NewPool(context.Background(), log.NewBackgroundLogger(), config.Cfg.DB.DSN())
	if err != nil {
		panic(err)
	}
}

func EnsureLatestMigration() error {
	conn, err := RootPool.AcquireBackground()
	if err != nil {
		return err
	}
	defer conn.Release()

	row := RootPool.QueryRow("select version from schema_migrations order by version desc limit 1")
	var latestDbVersion string
	err = row.Scan(&latestDbVersion)
	if err != nil {
		return err
	}

	for _, migration := range migrations.All {
		version := migration.Version()
		if version > latestDbVersion {
			return oops.Newf("Migration is not in db: %s", version)
		}
	}

	return nil
}
