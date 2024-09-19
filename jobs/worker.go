package jobs

import (
	"context"
	"errors"
	"feedrewind/config"
	"feedrewind/crawler"
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
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "net/http/pprof"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"github.com/stripe/stripe-go/v78"
	"gopkg.in/yaml.v3"
)

var Worker *cobra.Command

func init() {
	Worker = &cobra.Command{
		Use: "worker",
		Run: func(_ *cobra.Command, _ []string) {
			if config.Cfg.Env.IsDevOrTest() {
				go func() {
					fmt.Println(http.ListenAndServe("localhost:6061", nil))
				}()
			}

			dynoIdIdx := strings.LastIndex(config.Cfg.Dyno, ".") + 1
			dynoId, err := strconv.Atoi(config.Cfg.Dyno[dynoIdIdx:])
			if err != nil {
				log.NewBackgroundLogger().Error().Err(err).Msg("Couldn't parse dyno id")
				os.Exit(1)
			}
			workerNamePrefix := fmt.Sprintf("%s-%d", workerNameBase, dynoId)
			logger := &WorkerLogger{WorkerName: workerNamePrefix}

			conn, err := db.RootPool.AcquireBackground()
			if err != nil {
				logger.Error().Err(err).Msg("Couldn't connect to db")
				os.Exit(1)
			}

			availableWorkers := make([]bool, totalWorkerCount)
			for i := range availableWorkers {
				availableWorkers[i] = true
			}
			finishedJobs := make(chan jobResult, totalWorkerCount)

			stripe.Key = config.Cfg.StripeApiKey
			stripe.DefaultLeveledLogger = &log.StripeLogger{Logger: logger}

			err = startWorker(conn, dynoId, workerNamePrefix, availableWorkers, finishedJobs, logger)
			if errors.Is(err, context.Canceled) {
				logger.Info().Msg("Context canceled, shutting down")
				waitForJobs(conn, availableWorkers, finishedJobs, logger)
				os.Exit(0)
			} else if err != nil {
				logger.Error().Err(err).Msg("Error occurred in the worker, shutting down")
				waitForJobs(conn, availableWorkers, finishedJobs, logger)
				os.Exit(1)
			}
		},
	}
}

type jobFunc func(ctx context.Context, id JobId, pool *pgw.Pool, args []any) error

type jobDesc struct {
	ClassName string
	Func      jobFunc
}

var jobDescs []jobDesc

func registerJobNameFunc(className string, f jobFunc) {
	jobDescs = append(jobDescs, jobDesc{
		ClassName: className,
		Func:      f,
	})
}

const stripeWebhookWorkerCount = 100
const defaultWorkerCount = 100
const guidedCrawlingWorkerCount = 100
const totalWorkerCount = stripeWebhookWorkerCount + defaultWorkerCount + guidedCrawlingWorkerCount
const stripeWebhookQueue = "stripe_webhook"
const defaultQueue = "default"
const guidedCrawlingQueue = "guided_crawling"
const maxBrowserCount = 20

const workerNameBase = "go-worker"
const sleepDelay = 100 * time.Millisecond
const maxPollFailures = 600 // One minute of sleeps with sleepDelay
const maxAttempts = 25
const maxRunTimeDeadline = 4 * time.Hour
const maxRunTimeTimeout = maxRunTimeDeadline - 5*time.Minute
const deploymentDuration = time.Minute

type job struct {
	Id         JobId
	Attempts   int32
	RawHandler string
	Queue      string
	JobData    JobData
}

type Handler struct {
	Job_Data JobData
}

type JobData struct {
	Job_Class   string
	Arguments   []any
	Enqueued_At string
}

type jobResult struct {
	WorkerId int
	Id       JobId
	Status   jobStatus
	Err      error
}

func startWorker(
	conn *pgw.Conn, dynoId int, workerNamePrefix string, availableWorkers []bool, finishedJobs chan jobResult,
	logger log.Logger,
) error {
	jobDescsByClassName := make(map[string]jobDesc)
	for _, jobDesc := range jobDescs {
		if _, ok := jobDescsByClassName[jobDesc.ClassName]; ok {
			return oops.Newf("Duplicate job class name: %s", jobDesc.ClassName)
		}
		jobDescsByClassName[jobDesc.ClassName] = jobDesc
	}

	signalCtx, signalCancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer signalCancel()
	go func() {
		<-signalCtx.Done()
		logger.Info().Msg("Caught termination signal")
	}()

	lockFailures := 0
	for {
		row := conn.QueryRow(`select pg_try_advisory_lock($1)`, dynoId)
		var succeeded bool
		err := row.Scan(&succeeded)
		if err != nil {
			return err
		}

		row = conn.QueryRow(`
			select virtualtransaction, pid
			from pg_locks
			where locktype = 'advisory' and objid = $1
		`, dynoId)
		var virtualTransaction string
		var pid int
		err = row.Scan(&virtualTransaction, &pid)
		if err != nil {
			return err
		}

		if succeeded {
			logger.Info().Msgf("Acquired advisory lock (vt=%s pid=%d)", virtualTransaction, pid)
			break
		}

		lockFailures++
		if lockFailures >= 60 {
			return oops.Newf(
				"Couldn't acquire advisory lock after %d attempts (conflict vt=%s pid=%d)",
				lockFailures, virtualTransaction, pid,
			)
		}
		logger.Info().Msgf(
			"Acquiring advisory lock - attempt %d (conflict vt=%s pid=%d)",
			lockFailures, virtualTransaction, pid,
		)
		time.Sleep(time.Second)
	}

	crawler.SetMaxBrowserCount(maxBrowserCount)
	var lastStripeWebhookHoggedWarning time.Time
	var lastGuidedCrawlingHoggedWarning time.Time
	var defaultHoggedSince time.Time
	var lastDefaultHoggedWarning time.Time

	logger.Info().Msg("Worker started")

	pollFailures := 0
mainLoop:
	for {
		if err := signalCtx.Err(); err != nil {
			return err
		}
	checkFinished:
		for {
			select {
			case jobResult := <-finishedJobs:
				err := finishJob(conn, jobResult, availableWorkers, logger)
				if err != nil {
					return err
				}
				continue
			default:
				break checkFinished
			}
		}

		availableWorkerIdByQueue := map[string]int{}
		availableWorkerNameByQueue := map[string]string{}
		var availableWorkerNames []string
		for workerId, isAvailable := range availableWorkers {
			if isAvailable {
				var queue string
				switch {
				case workerId < stripeWebhookWorkerCount:
					queue = stripeWebhookQueue
				case workerId < stripeWebhookWorkerCount+defaultWorkerCount:
					queue = defaultQueue
				default:
					queue = guidedCrawlingQueue
				}
				workerName := fmt.Sprintf("%s-%d", workerNamePrefix, workerId)
				if _, ok := availableWorkerIdByQueue[queue]; !ok {
					availableWorkerIdByQueue[queue] = workerId
					availableWorkerNameByQueue[queue] = workerName
				}
				availableWorkerNames = append(availableWorkerNames, workerName)
			}
		}
		if _, ok := availableWorkerIdByQueue[stripeWebhookQueue]; !ok {
			if time.Since(lastStripeWebhookHoggedWarning) > 30*time.Minute {
				logger.Warn().Msgf("All %d stripe webhook workers are hogged", stripeWebhookWorkerCount)
				lastStripeWebhookHoggedWarning = time.Now()
			}
		}
		if _, ok := availableWorkerIdByQueue[defaultQueue]; !ok {
			if defaultHoggedSince.IsZero() {
				defaultHoggedSince = time.Now()
			} else if time.Since(defaultHoggedSince) > 10*time.Minute &&
				time.Since(lastDefaultHoggedWarning) > 30*time.Minute {
				logger.Warn().Msgf("All %d default workers are hogged", defaultWorkerCount)
				lastDefaultHoggedWarning = time.Now()
			}
		} else {
			defaultHoggedSince = time.Time{}
		}
		if _, ok := availableWorkerIdByQueue[guidedCrawlingQueue]; !ok {
			if time.Since(lastGuidedCrawlingHoggedWarning) > 30*time.Minute {
				logger.Warn().Msgf("All %d guided crawling workers are hogged", guidedCrawlingWorkerCount)
				lastGuidedCrawlingHoggedWarning = time.Now()
			}
		}
		if len(availableWorkerIdByQueue) == 0 {
			time.Sleep(sleepDelay)
			continue
		}

		var queuesSb strings.Builder
		var lockedBySb strings.Builder
		var shouldNotBeLockedBySb strings.Builder
		{
			fmt.Fprint(&queuesSb, "(")
			fmt.Fprint(&lockedBySb, "case")
			isFirst := true
			for queue, workerName := range availableWorkerNameByQueue {
				if !isFirst {
					fmt.Fprint(&queuesSb, ", ")
				}
				isFirst = false
				fmt.Fprintf(&queuesSb, "'%s'", queue)
				fmt.Fprintf(&lockedBySb, " when queue='%s' then '%s'", queue, workerName)
			}
			fmt.Fprint(&queuesSb, ")")
			fmt.Fprint(&lockedBySb, " end")

			fmt.Fprint(&shouldNotBeLockedBySb, "(")
			for i, workerName := range availableWorkerNames {
				if i > 0 {
					fmt.Fprint(&shouldNotBeLockedBySb, ", ")
				}
				fmt.Fprintf(&shouldNotBeLockedBySb, "'%s'", workerName)
			}
			fmt.Fprint(&shouldNotBeLockedBySb, ")")
		}

		var j job
		jobPollTime := schedule.UTCNow()
		lockExpiredTimestamp := jobPollTime.Add(-maxRunTimeDeadline)
		deploymentProbablyFinishedTimestamp := jobPollTime.Add(-deploymentDuration)
		row := conn.QueryRow(`
			update delayed_jobs
			set locked_at = $1, locked_by = `+lockedBySb.String()+`
			where id in (
				select id
				from delayed_jobs
				where (
					(run_at <= $1 and (locked_at is null or locked_at < $2)) or
					(locked_by in `+shouldNotBeLockedBySb.String()+` and locked_at < $3)
				) and failed_at is null and queue in `+queuesSb.String()+`
				order by priority asc, run_at asc
				limit 1
				for update
			)
			returning id, attempts, handler, queue, locked_by
		`, jobPollTime, lockExpiredTimestamp, deploymentProbablyFinishedTimestamp)
		var assignedWorkerName string
		err := row.Scan(&j.Id, &j.Attempts, &j.RawHandler, &j.Queue, &assignedWorkerName)
		if errors.Is(err, pgx.ErrNoRows) {
			time.Sleep(sleepDelay)
			continue mainLoop
		} else if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "25006" {
				// ERROR: cannot execute UPDATE in a read-only transaction (SQLSTATE 25006)
				return err
			}

			pollFailures++
			logger.Error().Err(err).Msgf("Poll failures: %d", pollFailures)
			if pollFailures >= maxPollFailures {
				return oops.New("Max poll failures reached")
			}
			time.Sleep(sleepDelay)
			continue mainLoop
		}
		pollFailures = 0

		assignedWorkerId, ok := availableWorkerIdByQueue[j.Queue]
		if !ok {
			panic(fmt.Errorf("Unknown queue: %s", j.Queue))
		}
		availableWorkers[assignedWorkerId] = false

		var h Handler
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

		go func() {
			assignedWorkerId := assignedWorkerId
			assignedWorkerName := assignedWorkerName
			j := j
			defer func() {
				if r := recover(); r != nil {
					err, ok := r.(error)
					if !ok {
						err = oops.Newf("%v", r)
					}
					finishedJobs <- jobResult{
						WorkerId: assignedWorkerId,
						Id:       j.Id,
						Status:   jobStatusFatal,
						Err:      err,
					}
				}
			}()
			status, err := runJob(signalCtx, j, jobDescsByClassName, assignedWorkerName)
			finishedJobs <- jobResult{
				WorkerId: assignedWorkerId,
				Id:       j.Id,
				Status:   status,
				Err:      err,
			}
		}()
	}
}

type jobStatus int

const (
	jobStatusFatal jobStatus = iota
	jobStatusFail
	jobStatusOk
)

func runJob(
	signalCtx context.Context, j job, jobDescsByClassName map[string]jobDesc, workerName string,
) (jobStatus, error) {
	jobLogger := &JobLogger{
		WorkerName: workerName,
		ClassName:  j.JobData.Job_Class,
		JobId:      j.Id,
	}
	// Context cancellation is handled manually at the job level and not at db level
	// Quick-running jobs should be able to gracefully finish and reschedule themselves
	jobPool := db.RootPool.Child(signalCtx, jobLogger)
	jobDesc, ok := jobDescsByClassName[j.JobData.Job_Class]
	if !ok {
		jobErr := oops.Newf("Couldn't find job func for class %s", j.JobData.Job_Class)
		jobLogger.Error().Err(jobErr).Send()
		err := failJob(jobPool, j, jobErr)
		if err != nil {
			return jobStatusFatal, err
		}
		return jobStatusFail, nil
	}

	jobLogger.LogPerforming(j)
	jobStart := time.Now().UTC()
	timeoutCtx, timeoutCancel := context.WithTimeout(signalCtx, maxRunTimeTimeout)
	jobErr := jobDesc.Func(timeoutCtx, j.Id, jobPool, j.JobData.Arguments)
	timeoutCancel()
	if jobErr != nil {
		if errors.Is(jobErr, context.Canceled) {
			jobLogger.LogCanceled(j, jobStart)
		} else {
			jobLogger.LogFailed(j, jobStart, jobErr)
		}
		err := failJob(jobPool, j, jobErr)
		if errors.Is(err, context.Canceled) {
			return jobStatusFatal, err
		} else if err != nil {
			err := retryAction(err, jobLogger, "fail job", func() error {
				return failJob(jobPool, j, jobErr)
			})
			if err != nil {
				return jobStatusFatal, err
			}
		}
		return jobStatusFail, nil
	}

	jobLogger.LogCompleted(j, jobStart)
	return jobStatusOk, nil
}

func failJob(qu pgw.Queryable, j job, jobErr error) error {
	utcNow := schedule.UTCNow()
	var errorStr string
	if sterr, ok := jobErr.(*oops.Error); ok {
		errorStr = sterr.FullString()
	} else {
		errorStr = jobErr.Error()
	}

	if j.Attempts+1 >= maxAttempts {
		_, err := qu.ExecWithContext(context.Background(), `
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

	_, err := qu.ExecWithContext(context.Background(), `
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

func waitForJobs(
	conn *pgw.Conn, availableWorkers []bool, finishedJobs chan jobResult, logger log.Logger,
) {
	allFinished := true
	for _, isAvailable := range availableWorkers {
		if !isAvailable {
			allFinished = false
			break
		}
	}
	if allFinished {
		return
	}

	timeout := 30 * time.Second
	timer := time.NewTimer(timeout)
	startTime := time.Now().UTC()

checkFinished:
	for {
		select {
		case jobResult := <-finishedJobs:
			err := finishJob(conn, jobResult, availableWorkers, logger)
			if err != nil {
				logger.Error().Err(err).Msgf("Error while finishing job %d", jobResult.Id)
			}
			for _, isAvailable := range availableWorkers {
				if !isAvailable {
					continue checkFinished
				}
			}
			timer.Stop()
			logger.Info().Msgf("Jobs finished in %v", time.Since(startTime))
			return
		case <-timer.C:
			logger.Error().Msgf("Jobs didn't finish within a %v timeout", timeout)
			return
		}
	}
}

func finishJob(
	conn *pgw.Conn, jobResult jobResult, availableWorkers []bool, logger log.Logger,
) error {
	switch jobResult.Status {
	case jobStatusFatal:
		return jobResult.Err
	case jobStatusOk:
		_, err := conn.ExecWithContext(context.Background(), `
			delete from delayed_jobs where id = $1
		`, jobResult.Id)
		if err != nil {
			err := retryAction(err, logger, "delete job", func() error {
				_, innerErr := conn.ExecWithContext(context.Background(), `
					delete from delayed_jobs where id = $1
				`, jobResult.Id)
				return innerErr
			})
			if err != nil {
				return err
			}
		}
		fallthrough
	case jobStatusFail:
		availableWorkers[jobResult.WorkerId] = true
	default:
		panic("unknown job status")
	}
	return nil
}

func retryAction(err error, logger log.Logger, actionName string, action func() error) error {
	failures := 0
	for {
		failures++
		logger.Error().Err(err).Msgf("%s failures: %d", actionName, failures)
		if failures >= maxPollFailures {
			return oops.Newf("Max %s failures reached", actionName)
		}
		time.Sleep(sleepDelay)
		err = action()
		if err == nil {
			return nil
		}
	}
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

func (l *JobLogger) LogPerforming(j job) {
	l.Info().
		Any("args", j.JobData.Arguments).
		Str("enqueued_at", j.JobData.Enqueued_At).
		Msg("Performing job")
}

func (l *JobLogger) LogCanceled(j job, jobStart time.Time) {
	l.Info().
		Any("args", j.JobData.Arguments).
		Msgf("Canceled job after %s (%d prior attempts)", time.Since(jobStart), j.Attempts)
}

func (l *JobLogger) LogFailed(j job, jobStart time.Time, jobErr error) {
	l.Error().
		Any("args", j.JobData.Arguments).
		Err(jobErr).
		Msgf("Failed job in %s (%d prior attempts)", time.Since(jobStart), j.Attempts)
}

func (l *JobLogger) LogCompleted(j job, jobStart time.Time) {
	l.Info().
		Any("args", j.JobData.Arguments).
		Msgf("Completed job in %s", time.Since(jobStart))
}
