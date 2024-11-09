package crawl

import (
	"context"
	"errors"
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

	"feedrewind.com/config"
	"feedrewind.com/crawler"
	"feedrewind.com/db/pgw"
	"feedrewind.com/log"
	"feedrewind.com/oops"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/launcher/flags"
	"github.com/spf13/cobra"
)

var Crawl *cobra.Command
var CrawlRobots *cobra.Command
var PuppeteerScaleTest *cobra.Command
var HN1000ScaleTest *cobra.Command

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

	HN1000ScaleTest = &cobra.Command{
		Use: "hn1000-scale-test",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return hn1000ScaleTest()
		},
	}
}

var defaultStartLinkId = 1132
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
	var gErr GuidedCrawlingError
	var isGuidedCrawlingError bool
	if errors.As(err, &gErr) {
		isGuidedCrawlingError = true
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
	if isGuidedCrawlingError {
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
		order by (id in (603, 1321, 1010, 1501, 1132, 1271)) desc, id asc
	`) // these special ids are slow and start late otherwise
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
		var gErr GuidedCrawlingError
		if errors.As(err, &gErr) {
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
	pool := connectDB()
	return runScaleTest(puppeteerIds, successfulIds, 4, pool)
}

func hn1000ScaleTest() error {
	successfulIds := []int64{
		764, 765, 766, 767, 770, 771, 772, 773, 774, 775, 776, 777, 779, 780, 781, 782, 783, 786, 787, 788,
		789, 791, 792, 793, 797, 799, 802, 803, 806, 807, 809, 810, 811, 812, 813, 814, 815, 817, 818, 819,
		820, 821, 822, 823, 824, 826, 828, 829, 830, 831, 832, 834, 839, 840, 841, 842, 844, 846, 847, 848,
		849, 850, 851, 852, 854, 855, 856, 857, 858, 859, 860, 861, 862, 863, 864, 865, 866, 867, 868, 869,
		870, 872, 873, 874, 875, 876, 877, 878, 881, 882, 883, 884, 885, 888, 889, 890, 892, 893, 894, 895,
		896, 897, 898, 899, 900, 901, 903, 904, 905, 906, 908, 909, 910, 911, 913, 914, 915, 916, 917, 918,
		919, 920, 921, 923, 924, 925, 927, 929, 930, 931, 932, 933, 934, 935, 936, 937, 938, 939, 940, 941,
		945, 946, 947, 948, 949, 950, 951, 952, 953, 954, 955, 958, 960, 961, 962, 963, 965, 966, 968, 969,
		970, 971, 972, 973, 974, 976, 977, 978, 979, 980, 981, 982, 983, 985, 987, 988, 989, 990, 991, 992,
		993, 995, 996, 997, 999, 1002, 1003, 1004, 1005, 1006, 1007, 1009, 1010, 1012, 1013, 1014, 1015, 1016,
		1017, 1018, 1019, 1020, 1021, 1022, 1023, 1024, 1025, 1026, 1027, 1028, 1029, 1030, 1031, 1032, 1033,
		1034, 1035, 1036, 1037, 1038, 1039, 1040, 1041, 1044, 1045, 1046, 1048, 1049, 1050, 1052, 1053, 1054,
		1056, 1058, 1060, 1061, 1062, 1063, 1064, 1066, 1067, 1068, 1069, 1070, 1071, 1073, 1076, 1077, 1078,
		1079, 1081, 1082, 1085, 1086, 1087, 1088, 1089, 1090, 1092, 1093, 1095, 1096, 1097, 1098, 1099, 1100,
		1101, 1102, 1103, 1105, 1106, 1107, 1108, 1109, 1111, 1114, 1115, 1116, 1117, 1119, 1121, 1122, 1124,
		1125, 1126, 1127, 1128, 1129, 1130, 1131, 1133, 1134, 1135, 1136, 1137, 1138, 1139, 1140, 1141, 1142,
		1143, 1145, 1146, 1148, 1149, 1150, 1151, 1152, 1153, 1154, 1156, 1157, 1158, 1161, 1162, 1164, 1166,
		1167, 1168, 1169, 1170, 1171, 1172, 1173, 1174, 1175, 1176, 1177, 1178, 1179, 1180, 1181, 1183, 1184,
		1185, 1186, 1187, 1188, 1189, 1190, 1191, 1192, 1193, 1194, 1195, 1197, 1198, 1199, 1200, 1201, 1202,
		1203, 1205, 1208, 1210, 1211, 1212, 1213, 1214, 1215, 1216, 1218, 1219, 1221, 1222, 1223, 1224, 1225,
		1226, 1227, 1229, 1230, 1232, 1233, 1234, 1236, 1237, 1238, 1239, 1240, 1242, 1243, 1246, 1247, 1248,
		1249, 1250, 1251, 1252, 1253, 1254, 1255, 1256, 1258, 1260, 1261, 1262, 1263, 1264, 1265, 1266,
		1268, 1269, 1270, 1271, 1272, 1273, 1274, 1275, 1276, 1277, 1278, 1280, 1281, 1282, 1283, 1284, 1285,
		1286, 1287, 1288, 1289, 1290, 1291, 1292, 1293, 1294, 1295, 1296, 1297, 1298, 1299, 1300, 1301, 1302,
		1303, 1304, 1305, 1306, 1307, 1308, 1309, 1310, 1311, 1312, 1313, 1314, 1315, 1316, 1317, 1318, 1319,
		1320, 1321, 1322, 1323, 1324, 1325, 1326, 1330, 1331, 1332, 1334, 1335, 1336, 1337, 1339, 1340, 1341,
		1342, 1343, 1344, 1345, 1346, 1347, 1348, 1349, 1350, 1351, 1352, 1354, 1355, 1356, 1358, 1360, 1362,
		1364, 1365, 1366, 1368, 1369, 1370, 1372, 1374, 1375, 1376, 1377, 1378, 1379, 1380, 1381, 1382, 1383,
		1384, 1386, 1387, 1388, 1389, 1390, 1391, 1393, 1394, 1395, 1398, 1399, 1400, 1401, 1402, 1403, 1404,
		1405, 1406, 1407, 1408, 1409, 1410, 1411, 1413, 1414, 1416, 1417, 1418, 1419, 1421, 1422, 1423, 1424,
		1425, 1426, 1427, 1428, 1431, 1432, 1433, 1435, 1436, 1437, 1438, 1440, 1441, 1442, 1443, 1444, 1445,
		1446, 1447, 1448, 1449, 1450, 1451, 1452, 1455, 1456, 1457, 1458, 1460, 1462, 1463, 1464, 1465, 1466,
		1468, 1469, 1470, 1471, 1473, 1474, 1475, 1476, 1477, 1478, 1479, 1480, 1481, 1483, 1484, 1486, 1488,
		1489, 1490, 1491, 1492, 1493, 1495, 1496, 1498, 1499, 1500, 1502, 1503, 1504, 1505, 1506, 1508, 1509,
		1511, 1512, 1513, 1515, 1516, 1519, 1520, 1522, 1523, 1524, 1525, 1527, 1528, 1529, 1530, 1535, 1536,
		1537, 1538, 1539, 1541, 1542, 1543, 1545, 1546, 1547, 1548, 1549, 1550, 1551, 1552, 1553, 1554, 1555,
		1556, 1557, 1558, 1559, 1560, 1561, 1562, 1564, 1565, 1566, 1567, 1568, 1569, 1570, 1571, 1572, 1574,
		1575, 1576, 1577, 1578, 1579, 1580, 1581, 1582, 1584, 1585, 1586, 1587, 1588, 1589, 1590, 1591, 1592,
		1593, 1594, 1595, 1596, 1598, 1599, 1600, 1601, 1602, 1603, 1604, 1605, 1607, 1608, 1609, 1610, 1611,
		1612, 1613, 1614, 1615, 1616, 1617, 1618, 1619, 1620, 1621, 1622, 1623, 1624, 1625, 1626, 1627, 1628,
		1629, 1630, 1631, 1632, 1633, 1634, 1635, 1636, 1637, 1638, 1639, 1641, 1642, 1643, 1644, 1645, 1646,
		1647, 1648, 1649, 1650, 1652, 1653, 1654, 1655, 1656, 1657, 1659, 1660, 1661, 1663, 1664, 1665, 1666,
		1667, 1668, 1669, 1670, 1671, 1672, 1673, 1674, 1675, 1677, 1678, 1680, 1681, 1683, 1684, 1686, 1687,
		1688, 1689, 1690, 1691, 1692, 1693, 1694, 1696, 1697, 1698, 1699, 1700, 1701, 1702, 1703, 1704, 1705,
		1706, 1707, 1708, 1709, 1710, 1711, 1712, 1713, 1714, 1715, 1716, 1717, 1718, 1719, 1720, 1721, 1722,
		1723, 1724, 1725, 1726, 1727, 1729, 1730, 1731, 1732, 1733, 1734, 1735, 1736, 1737, 1738, 1740, 1741,
		1742, 1744, 1745, 1746, 1748, 1749, 1750, 1751, 1753, 1754, 1755, 1756, 1757, 1758, 1759, 1761, 1762,
		1763, 1764, 1765, 1766,
	}

	pool := connectDB()
	var ids []int64
	rows, err := pool.Query(`select id from start_links where source = 'hn1000'`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id int64
		err := rows.Scan(&id)
		if err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	return runScaleTest(ids, successfulIds, 15, pool)
}

func runScaleTest(ids []int64, successfulIds []int64, windowCount int, pool *pgw.Pool) error {
	email := ""
	password := ""
	if email == "" || password == "" {
		return oops.New("missing credentials")
	}

	successfulIdsSet := map[int64]bool{}
	for _, id := range successfulIds {
		successfulIdsSet[id] = true
	}
	rand.Shuffle(len(ids), func(i, j int) {
		ids[i], ids[j] = ids[j], ids[i]
	})

	idCh := make(chan int64, len(ids))
	for _, id := range ids {
		idCh <- id
	}
	close(idCh)
	var wg sync.WaitGroup
	wg.Add(windowCount)
	successes := 0
	var falsePositives []int64
	var falseNegatives []int64
	var unknowns []int64
	var outputLock sync.Mutex
	startTime := time.Now()
	for windowIdx := range windowCount {
		go func() {
			defer wg.Done()

			windowsWide := 4
			width := 3840 / windowsWide
			height := 500
			firstScreenTall := 2120 / height
			var top, left int
			if windowIdx/windowsWide < firstScreenTall {
				top = (windowIdx / windowsWide) * height
				left = (windowIdx % windowsWide) * width
			} else {
				top = 2160 + (windowIdx-windowsWide*firstScreenTall)*height
				left = 0
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

			for startLinkId := range idCh {
				row := pool.QueryRow(`
					select coalesce(url, rss_url) from start_links where id = $1
				`, startLinkId)
				var startUrl string
				err := row.Scan(&startUrl)
				if err != nil {
					panic(err)
				}
				page.MustNavigate("https://feedrewind.com/subscriptions/add")
				page.MustElement("#start_url").MustInput(startUrl)
				page.MustElementR("button", "Go").MustClick()
				err = page.WaitLoad()
				var crawlSucceeded, crawlFailed bool
				if err != nil {
					fmt.Printf("!!! WaitLoad failed for %d (%s): %v\n", startLinkId, startUrl, err)
				} else {
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
					_, err = page.Timeout(3 * time.Second).Element("#select_posts")
					crawlSucceeded = err == nil
					if !crawlSucceeded {
						_, err := page.Timeout(time.Second).Element("#blog_failed")
						crawlFailed = err == nil
					}
				}

				outputLock.Lock()
				var status string
				if !crawlSucceeded && !crawlFailed {
					status = "Neither succeeded nor failed"
					unknowns = append(unknowns, startLinkId)
				} else if crawlSucceeded == successfulIdsSet[startLinkId] {
					status = "As expected"
					successes++
				} else if crawlSucceeded {
					status = "Previously failed, now succeeded"
					falsePositives = append(falsePositives, startLinkId)
				} else {
					status = "Previously succeeded, now failed"
					falseNegatives = append(falseNegatives, startLinkId)
				}
				progressFrac := float64(successes+len(falsePositives)+len(falseNegatives)+len(unknowns)) /
					float64(len(ids))
				progressPercent := progressFrac * 100
				eta := time.Duration(float64(time.Since(startTime)) / progressFrac * (1 - progressFrac)).
					Round(time.Second)
				fmt.Printf(
					"%s: %d (%s) (%.1f%%, eta %v)\n",
					status, startLinkId, startUrl, progressPercent, eta,
				)
				outputLock.Unlock()
			}
		}()
	}
	wg.Wait()
	fmt.Println("Done")
	fmt.Printf("Went as expected: %d/%d\n", successes, len(ids))
	if len(falsePositives) > 0 {
		fmt.Printf("Previously failed, now succeeded: %v\n", falsePositives)
	}
	if len(falseNegatives) > 0 {
		fmt.Printf("Previously succeeded, now failed: %v\n", falseNegatives)
	}
	if len(unknowns) > 0 {
		fmt.Printf("Neither succeeded nor failed: %v\n", unknowns)
	}

	return nil
}
