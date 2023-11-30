package db

import (
	"context"
	"feedrewind/config"
	"feedrewind/db/migrations"
	"feedrewind/db/pgw"
	"feedrewind/oops"
)

var Pool *pgw.Pool

func init() {
	var err error
	Pool, err = pgw.NewPool(context.Background(), config.Cfg.DB.DSN())
	if err != nil {
		panic(err)
	}
}

func EnsureLatestMigration() error {
	conn, err := Pool.AcquireBackground()
	if err != nil {
		return err
	}
	defer conn.Release()

	row := conn.QueryRow("select version from schema_migrations order by version desc limit 1")
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
