package jobs

import (
	"context"
	"feedrewind/db/pgw"
	"feedrewind/oops"
	"feedrewind/util/schedule"
	"time"
)

func init() {
	registerJobNameFunc(
		"LogPostgresRowCountJob",
		func(ctx context.Context, id JobId, conn *pgw.Conn, args []any) error {
			if len(args) != 0 {
				return oops.Newf("Expected 0 args, got %d: %v", len(args), args)
			}

			return LogPostgresRowCountJob_Perform(ctx, conn)
		},
	)
}

func LogPostgresRowCountJob_PerformAt(tx pgw.Queryable, runAt schedule.Time) error {
	return performAt(tx, runAt, "LogPostgresRowCountJob", defaultQueue)
}

func LogPostgresRowCountJob_Perform(ctx context.Context, conn *pgw.Conn) error {
	logger := conn.Logger()

	// https://stackoverflow.com/a/28668340
	row := conn.QueryRow(`
		SELECT
			SUM(pgClass.reltuples) AS totalRowCount
		FROM
			pg_class pgClass
		LEFT JOIN
			pg_namespace pgNamespace ON (pgNamespace.oid = pgClass.relnamespace)
		WHERE
			pgNamespace.nspname NOT IN ('pg_catalog', 'information_schema') AND
			pgClass.relkind='r'
	`)

	var rowCount int64
	err := row.Scan(&rowCount)
	if err != nil {
		return err
	}

	if rowCount > 5000000 {
		logger.Warn().Msgf("DB total row count: %d (over 50%%)", rowCount)
	} else {
		logger.Info().Msgf("DB total row count: %d", rowCount)
	}

	hourFromNow := schedule.UTCNow().Add(time.Hour)
	runAt := hourFromNow.BeginningOfHour()
	err = LogPostgresRowCountJob_PerformAt(conn, runAt)
	if err != nil {
		return err
	}

	return nil
}
