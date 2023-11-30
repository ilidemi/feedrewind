package crawl

import (
	"context"
	"errors"
	"feedrewind/config"
	"feedrewind/db/pgw"
	"fmt"
	"os"
	"os/exec"
	"runtime/debug"
	"runtime/pprof"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var Crawl *cobra.Command

func init() {
	Crawl = &cobra.Command{
		Use:  "crawl",
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			crawl(args)
		},
	}
	Crawl.Flags().IntVar(&threads, "threads", 16, "(only used when crawling all)")
	Crawl.Flags().BoolVar(&allowJS, "allow-js", false, "")
}

var defaultStartLinkId = 224
var threads int
var allowJS bool

func crawl(args []string) {
	cpuFile, err := os.Create("cpuprofile")
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := cpuFile.Close(); err != nil {
			panic(err)
		}
	}()
	err = pprof.StartCPUProfile(cpuFile)
	if err != nil {
		panic(err)
	}
	defer pprof.StopCPUProfile()

	debug.SetGCPercent(-1)
	debug.SetMemoryLimit(8 * 1024 * 1024 * 1024)

	if len(args) == 0 {
		runSingle(defaultStartLinkId)
		return
	}

	arg := args[0]
	if arg == "all" {
		runAll()
		return
	}

	startLinkId, err := strconv.ParseInt(arg, 10, 64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Expected start link id, got: %s", arg)
		os.Exit(1)
	}

	runSingle(int(startLinkId))
}

func connectDB() *pgw.Pool {
	dbConfig := config.DevelopmentDBConfig()
	dbConfig.DBName = "rss_catchup_analysis"
	dsn := dbConfig.DSN() + " pool_max_conns=" + fmt.Sprint(threads+1)
	var err error
	pool, err := pgw.NewPool(context.Background(), dsn) //nolint:gocritic
	if err != nil {
		panic(err)
	}
	return pool
}

func runSingle(startLinkId int) {
	pool := connectDB()
	conn, err := pool.AcquireBackground()
	if err != nil {
		panic(err)
	}
	logger := &FileLogger{File: os.Stdout}

	result, err := runGuidedCrawl(startLinkId, false, allowJS, conn, logger)
	var gErr *GuidedCrawlingError
	var ok bool
	if gErr, ok = err.(*GuidedCrawlingError); ok {
		result = &gErr.Result
	} else if err != nil {
		panic(err)
	}

	columnValues := result.ColumnValues()
	columnStatuses := result.ColumnStatuses()
	for i := 0; i < len(GuidedCrawlingColumnNames); i++ {
		if GuidedCrawlingColumnNames[i] == "extra" {
			fmt.Println("extra:")
			lines := columnValues[i].([]string)
			for _, line := range lines {
				fmt.Printf("\t%s\n", line)
			}
		} else {
			fmt.Printf("%s\t%v\t%s\n", GuidedCrawlingColumnNames[i], columnValues[i], columnStatuses[i])
		}
	}
	if gErr != nil {
		fmt.Println()
		fmt.Print(gErr.Inner.FullString())
		os.Exit(1)
	}
}

type Result struct {
	StartLinkId int
	Result      *GuidedCrawlingResult
	Error       error
}

func runAll() {
	startTime := time.Now()
	reportFilename := fmt.Sprintf(
		"cmd/crawl/report/mp_guided_crawl_%s.html",
		startTime.Format("2006-01-02_15-04-05"),
	)
	pool := connectDB()
	conn, err := pool.AcquireBackground()
	if err != nil {
		panic(err)
	}

	rows, err := conn.Query(`
		select id from start_links
		where id not in (select start_link_id from known_issues where severity = 'discard')
		order by (id = 603) desc, id asc
	`) // 603 is slow and starts late otherwise
	if err != nil {
		panic(err)
	}

	var startLinkIds []int
	for rows.Next() {
		var id int
		err := rows.Scan(&id)
		if err != nil {
			panic(err)
		}
		startLinkIds = append(startLinkIds, id)
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}

	if len(startLinkIds) < threads {
		threads = len(startLinkIds)
	}

	inputChan := make(chan int)
	go func() {
		for _, startLinkId := range startLinkIds {
			inputChan <- startLinkId
		}
		close(inputChan)
	}()

	resultChan := make(chan Result, len(startLinkIds))

	type Progress struct {
		ThreadIdx   int
		StartLinkId int
	}
	progressChan := make(chan Progress, threads)

	logDir := "cmd/crawl/guided_crawl_log"
	_, err = os.Stat(logDir)
	if err == nil {
		os.RemoveAll(logDir)
	} else if !errors.Is(err, os.ErrNotExist) {
		panic(err)
	}
	err = os.Mkdir(logDir, 0666)
	if err != nil {
		panic(err)
	}

	runSingleThreaded := func(threadConn *pgw.Conn, threadId, startLinkId int) {
		progressChan <- Progress{
			ThreadIdx:   threadId,
			StartLinkId: startLinkId,
		}

		defer func() {
			if rvr := recover(); rvr != nil {
				fmt.Printf("Start link id %d panicked\n", startLinkId)
				panic(rvr)
			}
		}()

		logFilename := fmt.Sprintf("%s/log%d.txt", logDir, startLinkId)
		logFile, err := os.Create(logFilename)
		if err != nil {
			panic(err)
		}
		defer func() {
			err := logFile.Close()
			if err != nil {
				panic(err)
			}
		}()

		logger := FileLogger{File: logFile}
		result, err := runGuidedCrawl(startLinkId, true, allowJS, threadConn, &logger)
		if gErr, ok := err.(*GuidedCrawlingError); ok {
			errorFilename := fmt.Sprintf("%s/error%d.txt", logDir, startLinkId)
			errorFile, err := os.Create(errorFilename)
			if err != nil {
				panic(err)
			}
			fmt.Fprint(errorFile, gErr.Inner.FullString())
			err = errorFile.Close()
			if err != nil {
				panic(err)
			}

			resultChan <- Result{
				StartLinkId: startLinkId,
				Result:      result,
				Error:       gErr.Inner,
			}
		} else if err != nil {
			resultChan <- Result{
				StartLinkId: startLinkId,
				Result:      nil,
				Error:       err,
			}
		} else {
			resultChan <- Result{
				StartLinkId: startLinkId,
				Result:      result,
				Error:       nil,
			}
		}

	}

	for threadIdx := 0; threadIdx < threads; threadIdx++ {
		threadIdx := threadIdx
		go func() {
			threadConn, err := pool.AcquireBackground()
			if err != nil {
				panic(err)
			}

			for startLinkId := range inputChan {
				runSingleThreaded(threadConn, threadIdx, startLinkId)
			}
			progressChan <- Progress{
				ThreadIdx:   threadIdx,
				StartLinkId: 0,
			}
		}()
	}

	freeVersionCmd := exec.Command("free", "--version")
	_, err = freeVersionCmd.Output()
	isFreeAvailable := err == nil

	progresses := make([]int, threads)
	var threadsRunning int
	var results []Result
	outputTicker := time.NewTicker(time.Second)

Loop:
	for {
		select {
		case progress := <-progressChan:
			progresses[progress.ThreadIdx] = progress.StartLinkId
			threadsRunning = 0
			for _, startLinkId := range progresses {
				if startLinkId != 0 {
					threadsRunning++
				}
			}
		case result := <-resultChan:
			results = append(results, result)
			if len(results) == len(startLinkIds) {
				break Loop
			}
		case <-outputTicker.C:
			err := outputReport(reportFilename, results, len(startLinkIds))
			if err != nil {
				panic(err)
			}

			currentTime := time.Now()
			elapsedSeconds := int(currentTime.Sub(startTime).Seconds())
			var elapsedStr string
			if elapsedSeconds < 60 {
				elapsedStr = fmt.Sprintf("%ds", elapsedSeconds)
			} else if elapsedSeconds < 3600 {
				elapsedStr = fmt.Sprintf("%dm%02ds", elapsedSeconds/60, elapsedSeconds%60)
			} else {
				elapsedStr = fmt.Sprintf(
					"%dh%02dm%02ds", elapsedSeconds/3600, (elapsedSeconds%3600)/60, elapsedSeconds%60,
				)
			}

			memoryLog := ""
			if isFreeAvailable {
				freeCmd := exec.Command("free", "-m")
				freeOutput, err := freeCmd.Output()
				if err != nil {
					memoryLog = fmt.Sprintf(" %s %s", string(freeOutput), err.Error())
				} else {
					memLine := strings.Split(string(freeOutput), "\n")[1]
					tokens := strings.Split(memLine, " ")
					memoryLog = fmt.Sprintf(
						" memory total:%s used:%s shared:%s cache:%s free:%s",
						tokens[1], tokens[2], tokens[4], tokens[5], tokens[3],
					)
				}
			}

			fmt.Printf(
				"%s elapsed:%s total:%d to_dispatch:%d to_process:%d running:%d %v%s\n",
				currentTime.Format("2006-01-02 15:04:05"), elapsedStr, len(startLinkIds), len(inputChan),
				len(startLinkIds)-len(results), threadsRunning, progresses, memoryLog,
			)
		}
	}

	err = outputReport(reportFilename, results, len(startLinkIds))
	if err != nil {
		panic(err)
	}
}
