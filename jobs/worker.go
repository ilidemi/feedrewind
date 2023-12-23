package jobs

import (
	"context"
	"errors"
	"feedrewind/db"
	"feedrewind/db/pgw"
	"feedrewind/log"
	"feedrewind/oops"
	"feedrewind/util/schedule"
	"fmt"
	"math"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var Worker *cobra.Command

func init() {
	Worker = &cobra.Command{
		Use: "worker",
		Run: func(_ *cobra.Command, _ []string) {
			go func() {
				fmt.Println(http.ListenAndServe("localhost:6061", nil))
			}()
			err := startWorker()
			logger := &log.BackgroundLogger{}
			if errors.Is(err, context.Canceled) {
				logger.Info().Msg("Context canceled, shutting down")
				os.Exit(0)
			} else if err != nil {
				logger.
					Error().
					Err(err).
					Msg("Error occurred in the worker")
				os.Exit(1)
			}
		},
	}
}

type jobFunc func(ctx context.Context, conn *pgw.Conn, args []any) error

type jobNameFunc struct {
	ClassName string
	Func      jobFunc
}

var jobNameFuncs []jobNameFunc

func registerJobNameFunc(className string, f jobFunc) {
	jobNameFuncs = append(jobNameFuncs, jobNameFunc{
		ClassName: className,
		Func:      f,
	})
}

const workerName = "go-worker"
const sleepDelay = 100 * time.Millisecond
const maxPollFailures = 600 // One minute of sleeps with sleepDelay
const maxAttempts = 25
const maxRunTimeDeadline = 4 * time.Hour
const maxRunTimeTimeout = maxRunTimeDeadline - 5*time.Minute

type job struct {
	Id         JobId
	Attempts   int32
	RawHandler string
	JobData    jobData
}

type handler struct {
	Job_Data jobData
}

type jobData struct {
	Job_Class   string
	Arguments   []any
	Enqueued_At string
}

func startWorker() error {
	jobFuncsByClassName := make(map[string]jobFunc)
	for _, jobNameFunc := range jobNameFuncs {
		if _, ok := jobFuncsByClassName[jobNameFunc.ClassName]; ok {
			return oops.Newf("Duplicate job class name: %s", jobNameFunc.ClassName)
		}
		jobFuncsByClassName[jobNameFunc.ClassName] = jobNameFunc.Func
	}

	conn, err := db.Pool.AcquireBackground()
	logger := &WorkerLogger{WorkerName: workerName}
	if err != nil {
		return err
	}

	signalCtx, signalCancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer signalCancel()
	go func() {
		<-signalCtx.Done()
		logger.Info().Msg("Caught termination signal")
	}()

	logger.Info().Msg("Worker started")

	pollFailures := 0
	for {
		if err := signalCtx.Err(); err != nil {
			return err
		}

		jobPollTime := schedule.UTCNow()
		lockExpiredTimestamp := jobPollTime.Add(-maxRunTimeDeadline)
		row := conn.QueryRow(`
			update delayed_jobs
			set locked_at = $1, locked_by = $3
			where id in (
				select id
				from delayed_jobs
				where
					(
						(run_at <= $1 and (locked_at is null or locked_at < $2)) or
						locked_by = $3
					) and
					failed_at is null
				order by priority asc, run_at asc
				limit 1
				for update
			)
			returning id, attempts, handler
		`, jobPollTime, lockExpiredTimestamp, workerName)

		var j job
		err := row.Scan(&j.Id, &j.Attempts, &j.RawHandler)
		if errors.Is(err, pgx.ErrNoRows) {
			time.Sleep(sleepDelay)
			continue
		} else if err != nil {
			pollFailures++
			logger.Error().Err(err).Msgf("Poll failures: %d", pollFailures)
			if pollFailures >= maxPollFailures {
				return oops.New("Max poll failures reached")
			}
			time.Sleep(sleepDelay)
			continue
		}

		pollFailures = 0

		var h handler
		err = yaml.Unmarshal([]byte(j.RawHandler), &h)
		if err != nil {
			jobErr := oops.Wrapf(err, "YAML deserialization error")
			logger.Error().Err(jobErr).Send()
			err := failJob(conn, j, jobErr)
			if err != nil {
				return err
			}
			continue
		}
		j.JobData = h.Job_Data

		jobFunc, ok := jobFuncsByClassName[j.JobData.Job_Class]
		if !ok {
			jobErr := oops.Newf("Couldn't find job func for class %s", j.JobData.Job_Class)
			logger.Error().Err(jobErr).Send()
			err := failJob(conn, j, jobErr)
			if err != nil {
				return err
			}
			continue
		}

		jobLogger := &JobLogger{
			WorkerName: workerName,
			ClassName:  j.JobData.Job_Class,
			JobId:      j.Id,
		}
		timeoutCtx, timeoutCancel := context.WithTimeout(signalCtx, maxRunTimeTimeout)
		jobConn, err := db.Pool.Acquire(timeoutCtx, jobLogger)
		if signalErr := signalCtx.Err(); signalErr != nil {
			timeoutCancel()
			return signalErr
		} else if err != nil {
			connFailures := 0
			for {
				connFailures++
				logger.Error().Err(err).Msgf("Job DB connection failures: %d", connFailures)
				if connFailures >= maxPollFailures {
					timeoutCancel()
					return oops.New("Max DB connection failures reached")
				}
				time.Sleep(sleepDelay)
				jobConn, err = db.Pool.Acquire(timeoutCtx, jobLogger)
				if err == nil {
					break
				}
			}
		}

		jobLogger.
			Info().
			Any("args", j.JobData.Arguments).
			Str("enqueued_at", j.JobData.Enqueued_At).
			Msg("Performing job")
		jobStart := time.Now().UTC()
		jobErr := jobFunc(timeoutCtx, jobConn, j.JobData.Arguments)
		timeoutCancel()
		jobConn.Release()
		if jobErr != nil {
			if errors.Is(jobErr, context.Canceled) {
				jobLogger.
					Info().
					Any("args", j.JobData.Arguments).
					Msgf("Canceled job after %s (%d prior attempts)", time.Since(jobStart), j.Attempts)
			} else {
				jobLogger.
					Error().
					Any("args", j.JobData.Arguments).
					Err(jobErr).
					Msgf("Failed job in %s (%d prior attempts)", time.Since(jobStart), j.Attempts)
			}
			err := failJob(conn, j, jobErr)
			if err != nil {
				failFailures := 0
				for {
					failFailures++
					jobLogger.
						Error().
						Err(err).
						Msgf("Fail job failures: %d", failFailures)
					if failFailures >= maxPollFailures {
						return oops.New("Max fail failures reached")
					}
					time.Sleep(sleepDelay)
					err = failJob(conn, j, jobErr)
					if err == nil {
						break
					}
				}
			}
			continue
		}

		jobLogger.
			Info().
			Any("args", j.JobData.Arguments).
			Msgf("Completed job in %s", time.Since(jobStart))

		_, err = conn.Exec(`delete from delayed_jobs where id = $1`, j.Id)
		if err != nil {
			deleteFailures := 0
			for {
				deleteFailures++
				jobLogger.
					Error().
					Err(err).
					Msgf("Delete failures: %d", deleteFailures)
				if deleteFailures >= maxPollFailures {
					return oops.New("Max delete failures reached")
				}
				time.Sleep(sleepDelay)
				_, err = conn.Exec(`delete from delayed_jobs where id = $1`, j.Id)
				if err == nil {
					break
				}
			}
		}
	}
}

func failJob(conn *pgw.Conn, j job, jobErr error) error {
	utcNow := schedule.UTCNow()
	var errorStr string
	if sterr, ok := jobErr.(*oops.Error); ok {
		errorStr = sterr.FullString()
	} else {
		errorStr = jobErr.Error()
	}

	if j.Attempts+1 >= maxAttempts {
		_, err := conn.Exec(`
			update delayed_jobs
			set locked_at = null,
				locked_by = null,
				attempts = $1,
				last_error = $2,
				failed_at = $3
			where id = $4
		`, j.Attempts+1, errorStr, utcNow, j.Id)
		return err
	}

	retryInSeconds := math.Pow(float64(j.Attempts), 4) + 5
	nextRunAt := utcNow.Add(time.Duration(retryInSeconds) * time.Second)

	_, err := conn.Exec(`
		update delayed_jobs
		set locked_at = null,
			locked_by = null,
			attempts = $1,
			last_error = $2,
			run_at = $3
		where id = $4
	`, j.Attempts+1, errorStr, nextRunAt, j.Id)
	return err
}

type WorkerLogger struct {
	WorkerName string
}

func (l *WorkerLogger) Info() *zerolog.Event {
	event := log.Base.Info()
	event = l.logWorkerCommon(event)
	return event
}

func (l *WorkerLogger) Warn() *zerolog.Event {
	event := log.Base.Warn()
	event = l.logWorkerCommon(event)
	return event
}

func (l *WorkerLogger) Error() *zerolog.Event {
	event := log.Base.Error()
	event = l.logWorkerCommon(event)
	return event
}

func (l *WorkerLogger) logWorkerCommon(event *zerolog.Event) *zerolog.Event {
	event = event.Timestamp()
	if schedule.IsSetUTCNowOverride() {
		event = event.Time("time_override", time.Time(schedule.UTCNow()))
	}
	event = event.Str("worker", l.WorkerName)
	return event
}

type JobLogger struct {
	WorkerName string
	ClassName  string
	JobId      JobId
}

func (l *JobLogger) Info() *zerolog.Event {
	event := log.Base.Info()
	event = l.logJobCommon(event)
	return event
}

func (l *JobLogger) Warn() *zerolog.Event {
	event := log.Base.Warn()
	event = l.logJobCommon(event)
	return event
}

func (l *JobLogger) Error() *zerolog.Event {
	event := log.Base.Error()
	event = l.logJobCommon(event)
	return event
}

func (l *JobLogger) logJobCommon(event *zerolog.Event) *zerolog.Event {
	event = event.Timestamp()
	if schedule.IsSetUTCNowOverride() {
		event = event.Time("time_override", time.Time(schedule.UTCNow()))
	}
	event = event.
		Str("worker", l.WorkerName).
		Int64("job_id", int64(l.JobId)).
		Str("class_name", l.ClassName)
	return event
}
