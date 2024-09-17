package jobs

import (
	"context"
	"feedrewind/db/pgw"
	"feedrewind/models"
	"feedrewind/oops"
	"feedrewind/util/schedule"
	"time"

	"github.com/mrz1836/postmark"
)

func init() {
	registerJobNameFunc("PollPostmarkBouncesJob",
		func(ctx context.Context, id JobId, pool *pgw.Pool, args []any) error {
			if len(args) != 0 {
				return oops.Newf("Expected 0 args, got %d: %v", len(args), args)
			}

			return PollPostmarkBouncesJob_Perform(ctx, pool)
		},
	)
}

func PollPostmarkBouncesJob_PerformAt(qu pgw.Queryable, runAt schedule.Time) error {
	return performAt(qu, runAt, "PollPostmarkBouncesJob", defaultQueue)
}

func PollPostmarkBouncesJob_Perform(ctx context.Context, pool *pgw.Pool) error {
	logger := pool.Logger()
	client, _ := GetPostmarkClientAndMaybeMetadata(pool)
	bounces, _, err := client.GetBounces(ctx, 100, 0, nil)
	if err != nil {
		return oops.Wrap(err)
	}
	logger.Info().Msgf("Queried %d bounces", len(bounces))

	var fullBounces []postmark.Bounce
	for _, bounce := range bounces {
		if err := ctx.Err(); err != nil {
			logger.Info().Err(err).Send()
			return err
		}

		exists, err := models.PostmarkBounce_Exists(pool, bounce.ID)
		if err != nil {
			return err
		}
		if exists {
			continue
		}

		fullBounce, err := client.GetBounce(ctx, bounce.ID)
		if err != nil {
			return oops.Wrap(err)
		}
		fullBounces = append(fullBounces, fullBounce)
	}
	logger.Info().Msgf("New bounces: %d", len(fullBounces))

	if len(fullBounces) > 0 && len(fullBounces) == len(bounces) {
		logger.Warn().Msg("All bounces are new, likely missed some on the second page")
	}

	tx, err := pool.Begin()
	if err != nil {
		return err
	}

	for _, fullBounce := range fullBounces {
		if err := ctx.Err(); err != nil {
			return err
		}

		logger.Warn().Any("bounce", fullBounce).Msg("Inserting Postmark bounce")
		err = models.PostmarkBounce_CreateIfNotExists(tx, fullBounce)
		if err != nil {
			rollbackErr := tx.Rollback()
			if rollbackErr != nil {
				logger.Error().Err(rollbackErr).Msgf("Rollback error")
			}
			return err
		}

		err = ProcessPostmarkBounceJob_PerformNow(tx, fullBounce.ID)
		if err != nil {
			rollbackErr := tx.Rollback()
			if rollbackErr != nil {
				logger.Error().Err(rollbackErr).Msgf("Rollback error")
			}
			return err
		}
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	hourFromNow := schedule.UTCNow().Add(time.Hour)
	runAt := hourFromNow.BeginningOfHour()
	err = PollPostmarkBouncesJob_PerformAt(pool, runAt)
	if err != nil {
		return err
	}

	return nil
}
