package jobs

import (
	"context"
	"feedrewind/crawler"
	"feedrewind/db/pgw"
	"feedrewind/oops"
	"feedrewind/third_party/tzdata"
	"feedrewind/util/schedule"
	"time"
)

func init() {
	registerJobNameFunc("TestSubstackJob",
		func(ctx context.Context, conn *pgw.Conn, args []any) error {
			if len(args) != 0 {
				return oops.Newf("Expected 0 args, got %d: %v", len(args), args)
			}

			return TestSubstackJob_Perform(ctx, conn)
		},
	)
}

func TestSubstackJob_PerformAt(tx pgw.Queryable, runAt schedule.Time) error {
	return performAt(tx, runAt, "TestSubstackJob", defaultQueue)
}

func TestSubstackJob_Perform(ctx context.Context, conn *pgw.Conn) error {
	logger := conn.Logger()

	httpClient := crawler.NewHttpClientImpl(ctx, false)
	dlogger := crawler.NewDummyLogger()
	progressLogger := crawler.NewMockProgressLogger(dlogger)
	crawlCtx := crawler.NewCrawlContext(httpClient, nil, &progressLogger)

	acxPublic, _, err := crawler.ExtractSubstackPublicAndTotalCounts(
		"https://www.astralcodexten.com", &crawlCtx, dlogger,
	)
	if err != nil {
		dlogger.Replay(logger)
		return err
	}
	if acxPublic == 0 {
		logger.Error().Msgf("ACX has zero public posts, check if paid post detection logic should be changed")
	}
	logger.Info().Msg("Public on ACX are good")

	pePublic, peTotal, err := crawler.ExtractSubstackPublicAndTotalCounts(
		"https://newsletter.pragmaticengineer.com/", &crawlCtx, dlogger,
	)
	if err != nil {
		dlogger.Replay(logger)
		return err
	}
	if pePublic == peTotal {
		logger.Error().Msgf(
			"Pragmatic Engineer has zero paid posts, check if paid post detection logic should be changed",
		)
	}
	logger.Info().Msg("Paid on Pragmatic Engineer are good")

	pst := tzdata.LocationByName["America/Los_Angeles"]
	utcNow := schedule.UTCNow()
	runAt := utcNow.BeginningOfDayIn(pst).Add(10 * time.Hour).UTC()
	if runAt.Sub(utcNow) < 0 {
		runAt = runAt.AddDate(0, 0, 1)
	}
	err = TestSubstackJob_PerformAt(conn, runAt)
	if err != nil {
		return err
	}

	return nil
}
