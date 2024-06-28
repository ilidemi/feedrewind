package jobs

import (
	"context"
	"feedrewind/config"
	"feedrewind/db/migrations"
	"feedrewind/db/pgw"
	"feedrewind/oops"
	"feedrewind/third_party/tzdata"
	"feedrewind/util"
	"feedrewind/util/schedule"
	"os"
	"time"
)

func init() {
	registerJobNameFunc(
		"CodeMaintenanceJob", func(ctx context.Context, conn *pgw.Conn, args []any) error {
			if len(args) != 0 {
				return oops.Newf("Expected 0 args, got %d: %v", len(args), args)
			}

			return CodeMaintenanceJob_Perform(ctx, conn)
		},
	)
	migrations.CodeMaintenanceJob_PerformAtFunc = CodeMaintenanceJob_PerformAt
}

func CodeMaintenanceJob_PerformAt(tx pgw.Queryable, runAt schedule.Time) error {
	return performAt(tx, runAt, "CodeMaintenanceJob", defaultQueue)
}

func CodeMaintenanceJob_Perform(ctx context.Context, conn *pgw.Conn) error {
	logger := conn.Logger()
	utcNow := schedule.UTCNow()

	tzDateCutoff := utcNow.AddDate(0, -6, 0).Format("2006-01-02")
	if tzdata.UpdatedDate < tzDateCutoff {
		logger.Warn().Msgf("tzdata is from %s, please update", tzdata.UpdatedDate)
	} else {
		logger.Info().Msgf("tzdata is from %s, still fresh", tzdata.UpdatedDate)
	}
	if util.TimezonesUpdatedDate < tzDateCutoff {
		logger.Warn().Msgf("timezones are from %s, please update", util.TimezonesUpdatedDate)
	} else {
		logger.Info().Msgf("timezones are from %s, still fresh", util.TimezonesUpdatedDate)
	}

	if config.Cfg.IsHeroku {
		chromeDateCutoff := utcNow.AddDate(0, -2, 0)
		chromeFileInfo, err := os.Stat("chrome")
		if err != nil {
			return oops.Wrap(err)
		}
		if chromeFileInfo.ModTime().Before(time.Time(chromeDateCutoff)) {
			logger.Warn().Msgf("chrome is from %s, please update", chromeFileInfo.ModTime())
		} else {
			logger.Info().Msgf("chrome is from %s, still fresh", chromeFileInfo.ModTime())
		}
	}

	pst := tzdata.LocationByName["America/Los_Angeles"]
	runAt := utcNow.BeginningOfDayIn(pst).Add(10 * time.Hour).UTC()
	if runAt.Sub(utcNow) < 0 {
		runAt = runAt.AddDate(0, 0, 1)
	}
	err := CodeMaintenanceJob_PerformAt(conn, runAt)
	if err != nil {
		return err
	}

	return nil
}
