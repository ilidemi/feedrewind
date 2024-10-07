package crawl

import (
	"context"
	"errors"
	"feedrewind/config"
	"feedrewind/crawler"
	"feedrewind/db/pgw"
	"feedrewind/log"
	"feedrewind/oops"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	neturl "net/url"
	"os"
	"os/exec"
	"runtime/debug"
	"runtime/pprof"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/launcher/flags"
	"github.com/spf13/cobra"
)

var Crawl *cobra.Command
var CrawlRobots *cobra.Command
var PuppeteerScaleTest *cobra.Command

func init() {
	Crawl = &cobra.Command{
		Use:  "crawl",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return crawl(args)
		},
	}
	Crawl.Flags().IntVar(&threads, "threads", 16, "(only used when crawling all)")
	Crawl.Flags().BoolVar(&allowJS, "allow-js", false, "")

	CrawlRobots = &cobra.Command{
		Use: "crawl-robots",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return crawlRobots()
		},
	}

	PuppeteerScaleTest = &cobra.Command{
		Use: "puppeteer-scale-test",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return puppeteerScaleTest()
		},
	}
}

var defaultStartLinkId = 1357
var threads int
var allowJS bool

func crawl(args []string) error {
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
		crawler.SetMaxBrowserCount(1)
		runSingle(defaultStartLinkId)
		return nil
	}

	arg := args[0]
	if arg == "all" {
		runAll()
		return nil
	}

	startLinkId, err := strconv.ParseInt(arg, 10, 64)
	if err != nil {
		return oops.Newf("Expected start link id, got: %s", arg)
	}

	runSingle(int(startLinkId))
	return nil
}

func connectDB() *pgw.Pool {
	dbConfig := config.DevelopmentDBConfig()
	dbConfig.DBName = "rss_catchup_analysis"
	dsn := dbConfig.DSN() + " pool_max_conns=" + fmt.Sprint(threads+1)
	var err error
	pool, err := pgw.NewPool(context.Background(), log.NewBackgroundLogger(), dsn) //nolint:gocritic
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

	crawler.SetMaxBrowserCount(threads)
	for threadIdx := range threads {
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
			switch {
			case elapsedSeconds < 60:
				elapsedStr = fmt.Sprintf("%ds", elapsedSeconds)
			case elapsedSeconds < 3600:
				elapsedStr = fmt.Sprintf("%dm%02ds", elapsedSeconds/60, elapsedSeconds%60)
			default:
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

func crawlRobots() error {
	pool := connectDB()

	startUrlsById := map[int]string{}
	rows, err := pool.Query(`select id, coalesce(url, rss_url) from start_links`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id int
		var url string
		err := rows.Scan(&id, &url)
		if err != nil {
			return err
		}
		startUrlsById[id] = url
	}
	if err := rows.Err(); err != nil {
		return err
	}

	const workerCount = 32
	var wg sync.WaitGroup
	wg.Add(workerCount)
	idCh := make(chan int, len(startUrlsById))
	err = os.Chdir("cmd/crawl")
	if err != nil {
		return oops.Wrap(err)
	}
	for id := range startUrlsById {
		idCh <- id
	}
	close(idCh)
	for range workerCount {
		go func() {
			defer wg.Done()
			for id := range idCh {
				url := startUrlsById[id]
				uri, err := neturl.Parse(url)
				if err != nil {
					panic(err)
				}
				uri.Path = "/robots.txt"
				resp, err := http.Get(uri.String())
				if err != nil {
					fmt.Println(id, "error:", err)
					continue
				}
				if resp.StatusCode == http.StatusNotFound {
					continue
				} else if resp.StatusCode != http.StatusOK {
					fmt.Printf("%d error: http %d (%d bytes)\n", id, resp.StatusCode, resp.ContentLength)
					resp.Body.Close()
					continue
				}
				filename := fmt.Sprintf("robots/%04d_%s.txt", id, uri.Hostname())
				outFile, err := os.Create(filename)
				if err != nil {
					panic(err)
				}
				_, err = io.Copy(outFile, resp.Body)
				if err != nil {
					panic(err)
				}
				resp.Body.Close()
				if err := outFile.Close(); err != nil {
					panic(err)
				}
			}
		}()
	}
	wg.Wait()
	fmt.Println("Done")
	return nil
}

func puppeteerScaleTest() error {
	puppeteerIds := []int64{
		1007, 1025, 1031, 1034, 1044, 1046, 1047, 1069, 1071, 1098, 1107, 1112, 1115, 1124, 1126, 1132, 1154,
		1156, 1157, 1158, 1167, 1170, 1175, 1177, 1180, 1187, 1195, 1196, 1204, 1242, 1267, 1268, 1270,
		1276, 1277, 1283, 1293, 1301, 1309, 1314, 1318, 1323, 1328, 1338, 1342, 1343, 1346, 1350, 1352, 1357,
		1363, 1365, 1372, 1379, 1398, 1405, 1437, 1444, 1455, 1457, 1490, 1497, 1499, 1504, 1520, 1522, 1529,
		1531, 1534, 1535, 1536, 1543, 1545, 1548, 1552, 1555, 1562, 1564, 1572, 1576, 1578, 1582, 1589, 1594,
		1596, 1615, 1616, 1618, 1623, 1624, 1625, 1633, 1634, 1636, 1640, 1645, 1672, 1675, 1682, 1691, 1693,
		1701, 1704, 1705, 1717, 1723, 1724, 1725, 1741, 1742, 1745, 1752, 1764, 1765, 772, 774, 776, 787, 789,
		797, 801, 813, 826, 844, 845, 847, 855, 873, 878, 881, 884, 896, 917, 919, 941, 946, 948, 952, 986,
		989, 995,
	}
	successfulIds := []int64{
		1007, 1025, 1031, 1034, 1044, 1046, 1069, 1071, 1098, 1107, 1124, 1126, 1154,
		1156, 1157, 1158, 1167, 1170, 1175, 1177, 1180, 1187, 1195, 1242, 1268, 1270,
		1276, 1277, 1283, 1293, 1301, 1309, 1314, 1318, 1323, 1342, 1343, 1346, 1350, 1352,
		1365, 1372, 1379, 1398, 1405, 1437, 1444, 1455, 1457, 1490, 1499, 1504, 1520, 1522, 1529,
		1535, 1536, 1543, 1545, 1548, 1552, 1555, 1562, 1564, 1572, 1576, 1578, 1582, 1589, 1594,
		1596, 1615, 1616, 1618, 1623, 1624, 1625, 1633, 1634, 1636, 1672, 1675, 1691, 1693,
		1701, 1704, 1705, 1717, 1723, 1724, 1725, 1741, 1742, 1745, 1764, 1765, 772, 774, 776, 787, 789,
		797, 813, 826, 844, 847, 855, 873, 878, 881, 884, 896, 917, 919, 941, 946, 948, 952,
		989, 995,
	}
	successfulIdsSet := map[int64]bool{}
	for _, id := range successfulIds {
		successfulIdsSet[id] = true
	}
	email := ""
	password := ""
	if email == "" || password == "" {
		return errors.New("missing credentials")
	}

	const windowCount = 4
	rand.Shuffle(len(puppeteerIds), func(i, j int) {
		puppeteerIds[i], puppeteerIds[j] = puppeteerIds[j], puppeteerIds[i]
	})
	idCount := len(puppeteerIds)
	idCh := make(chan int64, idCount)
	for _, id := range puppeteerIds[:idCount] {
		idCh <- id
	}
	close(idCh)
	var wg sync.WaitGroup
	wg.Add(windowCount)
	pool := connectDB()
	successes := 0
	var falsePositives []int64
	var falseNegatives []int64
	var outputLock sync.Mutex
	for windowIdx := range windowCount {
		go func() {
			defer wg.Done()

			width := 548
			height := 230
			left := (windowIdx % 7) * width
			var top int
			if windowIdx/7 < 9 {
				top = (windowIdx / 7) * height
			} else {
				top = 2160 + ((windowIdx/7 - 9) * height)
			}
			l := launcher.New().Append(flags.Arguments, "--force-device-scale-factor").Headless(false)
			browserUrl := l.MustLaunch()
			browser := rod.New().ControlURL(browserUrl).MustConnect()
			page := browser.MustPage("https://feedrewind.com/login")
			page.MustSetWindow(left, top, width, height)
			page.MustElement("#email").MustInput(email)
			page.MustElement("#current-password").MustInput(password)
			page.MustElementR("input", "Sign in").MustClick()
			page.MustWaitLoad()
			isFirst := true

			for startLinkId := range idCh {
				row := pool.QueryRow(`
					select coalesce(url, rss_url) from start_links where id = $1
				`, startLinkId)
				var startUrl string
				err := row.Scan(&startUrl)
				if err != nil {
					panic(err)
				}
				if !isFirst {
					page = browser.MustPage("https://feedrewind.com/subscriptions/add")
				} else {
					page.MustNavigate("https://feedrewind.com/subscriptions/add")
				}
				isFirst = false
				page.MustElement("#start_url").MustInput(startUrl)
				page.MustElementR("button", "Go").MustClick()
				page.MustWaitLoad()
				page.MustSetViewport(width, height, 0.33, false)
				scrolled := false
				for range time.Tick(time.Second) {
					_, err := page.Timeout(time.Second).Element("#progress_count")
					if err != nil {
						break
					}
					if !scrolled {
						page.Mouse.MustScroll(0, 120)
						scrolled = true
					}
				}
				page.MustWaitLoad()
				_, err = page.Timeout(time.Second).Element("#select_posts")
				crawlSucceeded := err == nil
				outputLock.Lock()
				if crawlSucceeded == successfulIdsSet[startLinkId] {
					fmt.Printf("As expected: %d (%s)\n", startLinkId, startUrl)
					successes++
				} else if crawlSucceeded {
					fmt.Printf("Previously failed, now succeeded: %d (%s)\n", startLinkId, startUrl)
					falsePositives = append(falsePositives, startLinkId)
				} else {
					fmt.Printf("Previously succeeded, now failed: %d (%s)\n", startLinkId, startUrl)
					falseNegatives = append(falseNegatives, startLinkId)
				}
				outputLock.Unlock()
			}
		}()
	}
	wg.Wait()
	fmt.Println("Done")
	fmt.Printf("Went as expected: %d/%d\n", successes, idCount)
	if len(falsePositives) > 0 {
		fmt.Printf("Previously failed, now succeeded: %v\n", falsePositives)
	}
	if len(falseNegatives) > 0 {
		fmt.Printf("Previously succeeded, now failed: %v\n", falseNegatives)
	}

	return nil
}
