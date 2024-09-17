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
	"os/exec"
	"time"
)

func init() {
	registerJobNameFunc(
		"CodeMaintenanceJob",
		func(ctx context.Context, id JobId, pool *pgw.Pool, args []any) error {
			if len(args) != 0 {
				return oops.Newf("Expected 0 args, got %d: %v", len(args), args)
			}

			return CodeMaintenanceJob_Perform(ctx, pool)
		},
	)
	migrations.CodeMaintenanceJob_PerformAtFunc = CodeMaintenanceJob_PerformAt
}

func CodeMaintenanceJob_PerformAt(qu pgw.Queryable, runAt schedule.Time) error {
	return performAt(qu, runAt, "CodeMaintenanceJob", defaultQueue)
}

func CodeMaintenanceJob_Perform(ctx context.Context, pool *pgw.Pool) error {
	logger := pool.Logger()
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
		chromeDateCutoff := utcNow.AddDate(0, -2, 0).Format("2006-01-02")
		chromePath, err := exec.LookPath("chrome")
		if err != nil {
			return oops.Wrap(err)
		}
		chromeFileInfo, err := os.Stat(chromePath)
		if err != nil {
			return oops.Wrap(err)
		}
		chromeModDate := chromeFileInfo.ModTime().Format("2006-01-02")
		if chromeModDate < chromeDateCutoff {
			logger.Warn().Msgf("chrome is from %s, please update", chromeModDate)
		} else {
			logger.Info().Msgf("chrome is from %s, still fresh", chromeModDate)
		}
	}

	pst := tzdata.LocationByName["America/Los_Angeles"]
	runAt := utcNow.BeginningOfDayIn(pst).Add(10 * time.Hour).UTC()
	if runAt.Sub(utcNow) < 0 {
		runAt = runAt.AddDate(0, 0, 1)
	}
	err := CodeMaintenanceJob_PerformAt(pool, runAt)
	if err != nil {
		return err
	}

	return nil
}
