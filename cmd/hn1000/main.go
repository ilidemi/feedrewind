package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	neturl "net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"feedrewind.com/crawler"

	"gioui.org/app"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/goccy/go-json"
	"github.com/spf13/cobra"
	"modernc.org/sqlite"

	_ "net/http/pprof"
)

func main() {
	err := os.Chdir("cmd/hn1000") // Undo the hack that the development config init does
	if err != nil {
		panic(err)
	}

	rootCmd := &cobra.Command{
		Use: "hn1000",
	}

	rootCmd.AddCommand(&cobra.Command{
		Use:  "download",
		Args: cobra.ExactArgs(1),
		Run: func(_ *cobra.Command, args []string) {
			mustDownload(args[0])
		},
	})

	rootCmd.AddCommand(&cobra.Command{
		Use: "fix-names",
		Run: func(_ *cobra.Command, args []string) {
			mustFixNames()
		},
	})

	rootCmd.AddCommand(&cobra.Command{
		Use: "initialize-links",
		Run: func(_ *cobra.Command, args []string) {
			mustInitializeLinks()
		},
	})

	rootCmd.AddCommand(&cobra.Command{
		Use: "find-feeds",
		Run: func(_ *cobra.Command, args []string) {
			mustFindFeeds()
		},
	})

	rootCmd.AddCommand(&cobra.Command{
		Use: "classify-feeds",
		Run: func(_ *cobra.Command, args []string) {
			mustClassifyFeeds(args)
		},
	})

	rootCmd.AddCommand(&cobra.Command{
		Use: "fix-404-blogs",
		Run: func(_ *cobra.Command, args []string) {
			mustFix404Blogs()
		},
	})

	rootCmd.AddCommand(&cobra.Command{
		Use: "undaily-substacks",
		Run: func(_ *cobra.Command, args []string) {
			mustUndailySubstacks()
		},
	})

	if err := rootCmd.Execute(); err != nil {
		panic(err)
	}
}

func mustDownload(countStr string) {
	count, err := strconv.Atoi(countStr)
	if err != nil {
		panic(err)
	}

	err = os.RemoveAll("response/")
	if err != nil {
		panic(err)
	}
	err = os.MkdirAll("response/", 0666)
	if err != nil {
		panic(err)
	}

	totalHits := 0
	page := 0
	pageEnd := time.Now()
	for {
		pageStart := pageEnd.AddDate(0, 0, -3)
		points := 50
		url := fmt.Sprintf(
			"https://hn.algolia.com/api/v1/search_by_date?tags=story&hitsPerPage=1000&"+
				"numericFilters=created_at_i>%d,created_at_i<=%d,points>=%d"+
				"&attributesToRetrieve=created_at,points,title,url&attributesToHighlight=[]",
			pageStart.Unix(), pageEnd.Unix(), points,
		)
		resp, err := http.Get(url)
		if err != nil {
			panic(err)
		}
		bodyBytes, err := io.ReadAll(resp.Body)
		defer resp.Body.Close()
		if err != nil {
			panic(err)
		}
		if resp.StatusCode != http.StatusOK {
			panic(fmt.Errorf("%d %s", resp.StatusCode, string(bodyBytes)))
		}

		var bodyJson map[string]any
		err = json.Unmarshal(bodyBytes, &bodyJson)
		if err != nil {
			panic(err)
		}

		outBytes, err := json.MarshalIndent(bodyJson, "", "    ")
		if err != nil {
			panic(err)
		}

		filename := fmt.Sprintf("response/response%02d.json", page)
		err = os.WriteFile(filename, outBytes, 0666)
		if err != nil {
			panic(err)
		}

		page++
		pageEnd = pageStart
		hits := bodyJson["hits"].([]any)
		fmt.Printf("%v - %d hits at %d points\n", pageEnd, len(hits), points)
		if len(hits) == 0 {
			panic("No new hits")
		}
		totalHits += len(hits)
		if totalHits > count {
			break
		}
	}
}

func mustFixNames() {
	for i := range 100 {
		oldPath := fmt.Sprintf("response/response%02d.json", i)
		newPath := fmt.Sprintf("response/response%03d.json", i)
		err := os.Rename(oldPath, newPath)
		if err != nil {
			panic(err)
		}
	}
}

func mustOpenDb() *sql.DB {
	db, err := sql.Open("sqlite", "data.db")
	if err != nil {
		panic(err)
	}
	_, err = db.Exec(`PRAGMA journal_mode = WAL`)
	if err != nil {
		panic(err)
	}
	_, err = db.Exec(`PRAGMA synchronous = normal`)
	if err != nil {
		panic(err)
	}
	_, err = db.Exec(`PRAGMA journal_size_limit = 6144000`)
	if err != nil {
		panic(err)
	}
	_, err = db.Exec(`PRAGMA busy_timeout = 1000`)
	if err != nil {
		panic(err)
	}
	return db
}

func mustInitializeLinks() {
	db := mustOpenDb()
	defer db.Close()
	_, err := db.Exec(`
		create table if not exists initial_links (
			url text not null,
			title text not null,
			created_at text not null,
			points integer not null
		)
	`)
	if err != nil {
		panic(err)
	}
	_, err = db.Exec(`delete from initial_links`)
	if err != nil {
		panic(err)
	}

	type Response struct {
		Hits []struct {
			Created_At string
			Points     int
			Title      string
			Url        *string
		}
	}

	entries, err := os.ReadDir("response")
	if err != nil {
		panic(err)
	}

	for _, entry := range entries {
		responseBytes, err := os.ReadFile("response/" + entry.Name())
		if err != nil {
			panic(err)
		}
		var response Response
		err = json.Unmarshal(responseBytes, &response)
		if err != nil {
			panic(err)
		}

		for _, hit := range response.Hits {
			if hit.Url == nil {
				continue
			}
			sqliteCreatedAt := strings.Replace(strings.Replace(hit.Created_At, "T", " ", 1), "Z", "", 1)
			_, err := db.Exec(`
				insert into initial_links (url, title, created_at, points)
				values ($1, $2, $3, $4)
			`, hit.Url, hit.Title, sqliteCreatedAt, hit.Points)
			if err != nil {
				panic(err)
			}
		}
	}

	row := db.QueryRow(`select count(*) from initial_links`)
	var count int
	err = row.Scan(&count)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Inserted %d initial links\n", count)
}

func mustFindFeeds() {
	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()
	db := mustOpenDb()
	defer db.Close()

	_, err := db.Exec(`
		create table if not exists feeds (
			root_canonical_url text primary key,
			root_url text not null,
			feed_url text,
			num_submissions integer not null,
			last_posted_at text not null,
			total_points integer not null
		)
	`)
	if err != nil {
		panic(err)
	}
	_, err = db.Exec(`
		create table if not exists weird_feeds (
			initial_url text not null,
			log text not null,
			has_multiple_feeds boolean not null,
			inserted_at text not null
		)
	`)
	if err != nil {
		panic(err)
	}
	_, err = db.Exec(`alter table initial_links add column processed_at text`)
	if err != nil {
		var sqliteErr *sqlite.Error
		if !(errors.As(err, &sqliteErr) && sqliteErr.Code() == 1) {
			panic(err)
		}
	}

	row := db.QueryRow(`select count(1) from initial_links where processed_at is null`)
	var totalCount int
	err = row.Scan(&totalCount)
	if err != nil {
		panic(err)
	}

	skippedHosts := map[string]bool{
		"arstechnica.com":      true,
		"en.wikipedia.org":     true,
		"fortune.com":          true,
		"spectrum.ieee.org":    true,
		"www.latimes.com":      true,
		"www.theregister.com":  true,
		"www.tomshardware.com": true,
	}

	const workerCount = 32
	var wg sync.WaitGroup
	wg.Add(workerCount)
	activeHosts := map[string]bool{}
	processedCount := 0
	var activeHostsProcessedCountLock sync.Mutex
	startTime := time.Now()
	var sqliteBusyCount atomic.Int64
	for workerId := range workerCount {
		go func() {
			defer wg.Done()
			for {
				activeHostsProcessedCountLock.Lock()
				// This only kind of works by accident. Without the domain lock multiple workers could easily
				// pick up the same row. A better implementation would have processing_epoch, pick up stuff
				// that doesn't conflict with the current epoch but take the rows abandoned at a previous
				// epoch.
				var initialQuery strings.Builder
				fmt.Fprint(&initialQuery, `
					select rowid, url, created_at, points from initial_links
					where processed_at is null
				`)
				for activeHost := range activeHosts {
					fmt.Fprintf(&initialQuery, `and url not like 'http://%s%%'`, activeHost)
					fmt.Fprintf(&initialQuery, `and url not like 'http://www.%s%%'`, activeHost)
					fmt.Fprintf(&initialQuery, `and url not like 'https://%s%%'`, activeHost)
					fmt.Fprintf(&initialQuery, `and url not like 'https://www.%s%%'`, activeHost)
				}
				fmt.Fprint(&initialQuery, `order by created_at desc limit 1`)
				row := db.QueryRow(initialQuery.String())
				var initialRowId int64
				var initialUrl, initialCreatedAt string
				var initialPoints int
				err := row.Scan(&initialRowId, &initialUrl, &initialCreatedAt, &initialPoints)
				if errors.Is(err, sql.ErrNoRows) {
					activeHostsProcessedCountLock.Unlock()
					fmt.Printf("w%02d: done\n", workerId)
					break
				} else if err != nil {
					panic(err)
				}
				initialCanonicalUrl := toCanonicalUrl(initialUrl)

				row = db.QueryRow(`
					select rowid from feeds where $1 like root_canonical_url || '%'
				`, initialCanonicalUrl)
				var matchingRowId int64
				err = row.Scan(&matchingRowId)
				if errors.Is(err, sql.ErrNoRows) {
					// no-op, actually go on fetching
				} else if err != nil {
					panic(err)
				} else {
					success := false
					for attempt := range 3 {
						tx, err := db.Begin()
						if err != nil {
							panic(err)
						}
						// transaction is upgraded on the first write
						_, err = tx.Exec(`update feeds set feed_url = feed_url where 0`)
						var sqliteErr *sqlite.Error
						if errors.As(err, &sqliteErr) && sqliteErr.Code() == 5 {
							sqliteBusyCount.Add(1)
							fmt.Printf("w%02d: SQLITE_BUSY (%d)\n", workerId, attempt+1)
							time.Sleep(time.Second)
							continue
						} else if err != nil {
							panic(err)
						}
						_, err = tx.Exec(`
							update feeds
							set num_submissions = num_submissions + 1,
								total_points = total_points + $1
							where rowid = $2
						`, initialPoints, matchingRowId)
						if err != nil {
							panic(err)
						}
						_, err = tx.Exec(`
							update initial_links set processed_at = datetime('now') where rowid = $1
						`, initialRowId)
						if err != nil {
							panic(err)
						}
						err = tx.Commit()
						if err != nil {
							panic(err)
						}
						processedCount++
						success = true
						break
					}
					if !success {
						panic("Couldn't update the db")
					}

					activeHostsProcessedCountLock.Unlock()
					continue
				}

				activeHost := initialCanonicalUrl
				if index := strings.Index(initialCanonicalUrl, "/"); index != -1 {
					activeHost = initialCanonicalUrl[:index]
				}
				if skippedHosts[activeHost] {
					success := false
					for attempt := range 3 {
						tx, err := db.Begin()
						if err != nil {
							panic(err)
						}
						// transaction is upgraded on the first write
						_, err = tx.Exec(`
							update initial_links set processed_at = datetime('now') where rowid = $1
						`, initialRowId)
						var sqliteErr *sqlite.Error
						if errors.As(err, &sqliteErr) && sqliteErr.Code() == 5 {
							sqliteBusyCount.Add(1)
							fmt.Printf("w%02d: SQLITE_BUSY (%d)\n", workerId, attempt+1)
							time.Sleep(time.Second)
							continue
						} else if err != nil {
							panic(err)
						}
						err = tx.Commit()
						if err != nil {
							panic(err)
						}
						processedCount++
						success = true
						break
					}
					if !success {
						panic("Couldn't update the db")
					}

					activeHostsProcessedCountLock.Unlock()
					continue
				}
				activeHosts[activeHost] = true
				activeHostsProcessedCountLock.Unlock()

				url, err := neturl.Parse(initialUrl)
				if err != nil {
					panic(err)
				}

				var feedUrl string
				var lastFeedUrl string
				var rootUrl string
				hasMultipleFeeds := false
				isWeird := false
				var log strings.Builder
				logger := crawler.NewDummyLogger()
				crawlCtx := crawler.NewCrawlContext(
					crawler.NewHttpClientImplCtx(context.Background(), true), //nolint:gocritic
					nil,
					crawler.NewMockProgressLogger(logger),
				)
				requests := 0
				requestsStartTime := time.Now()
				for {
					result := crawler.DiscoverFeedsAtUrl(url.String(), true, &crawlCtx, logger)
					requests++
					if singleFeedResult, ok := result.(*crawler.DiscoveredSingleFeed); ok {
						lastFeedUrl = singleFeedResult.Feed.Url
						if feedUrl == "" ||
							feedUrl == lastFeedUrl ||
							!((strings.Contains(feedUrl, "/blog/") &&
								!strings.Contains(lastFeedUrl, "/blog/")) ||
								strings.Contains(feedUrl, "/posts/") &&
									!strings.Contains(lastFeedUrl, "/posts/")) {
							feedUrl = lastFeedUrl
							rootUrl = url.String()
						}
					}
					fmt.Fprintf(&log, "%s -> %#v\n", url, result)
					// switch typedResult := result.(type) {
					// case *crawler.DiscoverFeedsErrorNoFeeds:
					// 	fmt.Fprintf(&log, "%s -> %#v\n", url, typedResult)
					// case *crawler.DiscoveredSingleFeed:
					// 	typedResult.MaybeStartPage = nil
					// 	typedResult.Feed.Content = ""
					// 	typedResult.Feed.ParsedFeed = nil
					// 	lastFeedUrl = typedResult.Feed.Url
					// 	if feedUrl == "" {
					// 		feedUrl = lastFeedUrl
					// 		rootUrl = url.String()
					// 	} else if feedUrl == lastFeedUrl {
					// 		rootUrl = url.String()
					// 	} else {
					// 		isWeird = true
					// 	}
					// 	fmt.Fprintf(&log, "%s -> %#v\n", url, typedResult)
					// case *crawler.DiscoveredMultipleFeeds:
					// 	if len(typedResult.Feeds) == 2 &&
					// 		(strings.HasSuffix(typedResult.Feeds[0].Title, "Comments Feed") ||
					// 			strings.HasSuffix(typedResult.Feeds[1].Title, "Comments Feed")) {

					// 		if strings.HasSuffix(typedResult.Feeds[0].Title, "Comments Feed") {
					// 			lastFeedUrl = typedResult.Feeds[1].Url
					// 		} else {
					// 			lastFeedUrl = typedResult.Feeds[0].Url
					// 		}
					// 		if feedUrl == "" {
					// 			feedUrl = lastFeedUrl
					// 			rootUrl = url.String()
					// 		} else if feedUrl == lastFeedUrl {
					// 			rootUrl = url.String()
					// 		} else {
					// 			isWeird = true
					// 		}
					// 	} else {
					// 		isWeird = true
					// 		hasMultipleFeeds = true
					// 	}
					// 	typedResult.StartPage.Content = ""
					// 	fmt.Fprintf(&log, "%s -> %#v\n", url, typedResult)
					// case *crawler.DiscoverFeedsErrorCouldNotReach:
					// 	errStr := typedResult.Error.Error()
					// 	if !strings.Contains(errStr, "Permanent error (4") {
					// 		isWeird = true
					// 	}
					// 	fmt.Fprintf(&log, "%s -> %#v (%s)\n", url, typedResult, errStr)
					// default:
					// 	isWeird = true
					// 	fmt.Fprintf(&log, "%s -> %#v\n", url, typedResult)
					// }

					if url.Path == "" || url.Path == "/" {
						break
					}
					url.Path = strings.TrimSuffix(url.Path, "/")
					url.Path = url.Path[:strings.LastIndex(url.Path, "/")+1]
					log.WriteString("\n")
				}

				success := false
				for attempt := range 3 {
					tx, err := db.Begin()
					if err != nil {
						panic(err)
					}
					// transaction is upgraded on the first write
					_, err = tx.Exec(`update feeds set feed_url = feed_url where 0`)
					var sqliteErr *sqlite.Error
					if errors.As(err, &sqliteErr) && sqliteErr.Code() == 5 {
						sqliteBusyCount.Add(1)
						fmt.Printf("w%02d: SQLITE_BUSY (%d)\n", workerId, attempt+1)
						time.Sleep(time.Second)
						continue
					} else if err != nil {
						panic(err)
					}
					if feedUrl == "" {
						// Feed not found and that's okay
					} else if isWeird {
						_, err := tx.Exec(`
						insert into weird_feeds (initial_url, log, has_multiple_feeds, inserted_at)
						values ($1, $2, $3, datetime('now'))
					`, initialUrl, log.String(), hasMultipleFeeds)
						if err != nil {
							panic(err)
						}
					} else {
						canonicalUrl := toCanonicalUrl(rootUrl)
						_, err := tx.Exec(`
							insert into feeds (
								root_canonical_url, root_url, feed_url, num_submissions, last_posted_at,
								total_points
							) values ($1, $2, $3, 1, $4, $5)
						`, canonicalUrl, rootUrl, feedUrl, initialCreatedAt, initialPoints)
						if err != nil {
							panic(err)
						}
					}
					_, err = tx.Exec(`
						update initial_links set processed_at = datetime('now') where rowid = $1
					`, initialRowId)
					if err != nil {
						panic(err)
					}
					err = tx.Commit()
					if err != nil {
						panic(err)
					}
					success = true
					break
				}
				if !success {
					panic("Couldn't update the db")
				}

				activeHostsProcessedCountLock.Lock()
				delete(activeHosts, activeHost)
				processedCount++
				doneFraction := float64(processedCount) / float64(totalCount)
				donePercent := doneFraction * 100
				timeRemaining := (time.Since(startTime) * time.Duration(totalCount-processedCount) /
					time.Duration(processedCount)).Round(time.Second)
				fmt.Printf(
					"w%02d: %d/%d, %.1f%%, %v remaining, %d busy (%d req, %v)\n",
					workerId, processedCount, totalCount, donePercent, timeRemaining, sqliteBusyCount.Load(),
					requests, time.Since(requestsStartTime).Round(time.Second),
				)
				activeHostsProcessedCountLock.Unlock()
			}
		}()
	}
	wg.Wait()
}

func toCanonicalUrl(url string) string {
	canonicalUrl := url[strings.Index(url, "://")+3:]
	canonicalUrl = strings.TrimPrefix(canonicalUrl, "www.")
	return canonicalUrl
}

func mustClassifyFeeds(args []string) {
	blogsGoal := 0
	if len(args) == 1 {
		var err error
		blogsGoal, err = strconv.Atoi(args[0])
		if err != nil {
			panic(err)
		}
	}

	db := mustOpenDb()
	defer db.Close()

	_, err := db.Exec(`
		create table if not exists blog_feeds(
			root_url text not null,
			feed_url text not null
		)
	`)
	if err != nil {
		panic(err)
	}

	_, err = db.Exec(`alter table feeds add column processed_at text`)
	var sqliteErr *sqlite.Error
	if err != nil && !(errors.As(err, &sqliteErr) && sqliteErr.Code() == 1) {
		panic(err)
	}

	type Feed struct {
		RootCanonicalUrl string
		RootUrl          string
		FeedUrl          string
	}
	var feeds []Feed
	rows, err := db.Query(`
		select root_canonical_url, root_url, feed_url from feeds
		where processed_at is null
		order by num_submissions desc, total_points desc
	`)
	if err != nil {
		panic(err)
	}
	for rows.Next() {
		var f Feed
		err := rows.Scan(&f.RootCanonicalUrl, &f.RootUrl, &f.FeedUrl)
		if err != nil {
			panic(err)
		}
		feeds = append(feeds, f)
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}

	row := db.QueryRow(`
		select (select count(1) from blog_feeds), (select count(1) from feeds where processed_at is not null)
	`)
	var blogCount, processedCount int
	err = row.Scan(&blogCount, &processedCount)
	if err != nil {
		panic(err)
	}
	sessionBlogCount, sessionProcessedCount := 0, 0
	sessionGoal := int(math.Ceil(float64(blogsGoal-blogCount) / 0.565))

	l := launcher.New().Headless(false)
	browserUrl := l.MustLaunch()
	browser := rod.New().ControlURL(browserUrl).MustConnect()
	pageChan := make(chan string, 100)
	page := browser.MustPage(feeds[0].RootUrl)
	go func() {
		for url := range pageChan {
			page.MustNavigate(url)
		}
	}()

	go func() {
		window := new(app.Window)
		window.Option(app.MaxSize(unit.Dp(300), unit.Dp(250)))
		theme := material.NewTheme()
		var yesButton, noButton widget.Clickable
		var ops op.Ops
		type C = layout.Context
		type D = layout.Dimensions
		for {
			switch e := window.Event().(type) {
			case app.DestroyEvent:
				os.Exit(0)
			case app.FrameEvent:
				// This graphics context is used for managing the rendering state.
				gtx := app.NewContext(&ops, e)

				//nolint:exhaustruct
				layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx C) D {
					return layout.Flex{
						Axis: layout.Vertical,
					}.Layout(gtx,
						layout.Rigid(func(gtx C) D {
							label := material.Label(theme, unit.Sp(12), feeds[0].RootUrl)
							label.MaxLines = 1
							return label.Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
						layout.Rigid(material.Label(theme, unit.Sp(16), "Is this a blog?").Layout),
						layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
						layout.Rigid(func(gtx C) D {
							return layout.Flex{
								Axis: layout.Horizontal,
							}.Layout(gtx,
								layout.Rigid(func(gtx C) D {
									btn := material.Button(theme, &yesButton, "Yes")
									gtx.Constraints.Min.X = gtx.Dp(75)
									if yesButton.Clicked(gtx) {
										blogCount++
										processedCount++
										sessionBlogCount++
										sessionProcessedCount++
										if sessionGoal > 0 {
											sessionGoal--
										}
										_, err := db.Exec(`
											insert into blog_feeds (root_url, feed_url) values ($1, $2)
										`, feeds[0].RootUrl, feeds[0].FeedUrl)
										if err != nil {
											panic(err)
										}
										_, err = db.Exec(`
											update feeds set processed_at = datetime('now')
											where root_canonical_url = $1
										`, feeds[0].RootCanonicalUrl)
										if err != nil {
											panic(err)
										}
										feeds = feeds[1:]
										if len(feeds) == 0 {
											fmt.Println("Done")
											os.Exit(0)
										}
										pageChan <- feeds[0].RootUrl
									}
									return btn.Layout(gtx)
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
								layout.Rigid(func(gtx C) D {
									btn := material.Button(theme, &noButton, "No")
									gtx.Constraints.Min.X = gtx.Dp(75)
									if noButton.Clicked(gtx) {
										processedCount++
										sessionProcessedCount++
										if sessionGoal > 0 {
											sessionGoal--
										}
										_, err = db.Exec(`
											update feeds set processed_at = datetime('now')
											where root_canonical_url = $1
										`, feeds[0].RootCanonicalUrl)
										if err != nil {
											panic(err)
										}
										feeds = feeds[1:]
										if len(feeds) == 0 {
											fmt.Println("Done")
											os.Exit(0)
										}
										pageChan <- feeds[0].RootUrl
									}
									return btn.Layout(gtx)
								}),
							)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
						layout.Rigid(func(gtx C) D {
							label := fmt.Sprintf("Goal: %d", sessionGoal)
							return material.Label(theme, unit.Sp(32), label).Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
						layout.Rigid(func(gtx C) D {
							label := fmt.Sprintf("Blogs: %d/%d", blogCount, processedCount)
							return material.Label(theme, unit.Sp(16), label).Layout(gtx)
						}),
						layout.Rigid(func(gtx C) D {
							label := fmt.Sprintf(
								"This session: %d/%d", sessionBlogCount, sessionProcessedCount,
							)
							return material.Label(theme, unit.Sp(16), label).Layout(gtx)
						}),
					)
				})

				// Pass the drawing operations to the GPU.
				e.Frame(gtx.Ops)
			}
		}
	}()
	app.Main()
}

func mustFix404Blogs() {
	db := mustOpenDb()
	defer db.Close()

	type Feed struct {
		RootCanonicalUrl string
		RootUrl          string
	}

	var feeds []Feed
	rows, err := db.Query(`
		select root_canonical_url, root_url from feeds
		where root_url like '%/blog/' and processed_at is null
	`)
	if err != nil {
		panic(err)
	}
	for rows.Next() {
		var f Feed
		err := rows.Scan(&f.RootCanonicalUrl, &f.RootUrl)
		if err != nil {
			panic(err)
		}
		feeds = append(feeds, f)
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}

	fmt.Println(len(feeds), "feeds")
	workerCount := 50
	var wg sync.WaitGroup
	wg.Add(workerCount)
	feedCh := make(chan Feed)
	var dbLock sync.Mutex
	go func() {
		for _, feed := range feeds {
			feedCh <- feed
		}
		close(feedCh)
	}()
	for range workerCount {
		go func() {
			defer wg.Done()
			for feed := range feedCh {
				resp, err := http.Get(feed.RootUrl)
				if err != nil {
					panic(err)
				}
				resp.Body.Close()
				dbLock.Lock()
				if resp.StatusCode == 404 {
					_, err := db.Exec(`
						update feeds set root_canonical_url = $1, root_url = $2
						where root_canonical_url = $3
					`, strings.TrimSuffix(feed.RootCanonicalUrl, "blog/"),
						strings.TrimSuffix(feed.RootUrl, "blog/"),
						feed.RootCanonicalUrl)
					if err != nil {
						panic(err)
					}
				}
				fmt.Printf("%s -> %d\n", feed.RootUrl, resp.StatusCode)
				dbLock.Unlock()
			}
		}()
	}
	wg.Wait()
}

func mustUndailySubstacks() {
	db := mustOpenDb()
	defer db.Close()

	var substackUrls []string
	rows, err := db.Query(`
		select root_url from blog_feeds where feed_url like root_url || 'feed' order by rowid asc
	`)
	if err != nil {
		panic(err)
	}
	for rows.Next() {
		var rootUrl string
		err := rows.Scan(&rootUrl)
		if err != nil {
			panic(err)
		}
		substackUrls = append(substackUrls, rootUrl)
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}

	sessionProcessedCount := 0
	totalUrls := len(substackUrls)

	l := launcher.New().Headless(false)
	browserUrl := l.MustLaunch()
	browser := rod.New().ControlURL(browserUrl).MustConnect()
	pageChan := make(chan string, 100)
	page := browser.MustPage(substackUrls[0] + "archive")
	go func() {
		for url := range pageChan {
			page.MustNavigate(url)
		}
	}()

	go func() {
		window := new(app.Window)
		window.Option(app.MaxSize(unit.Dp(300), unit.Dp(250)))
		theme := material.NewTheme()
		var dailyButton, normalButton widget.Clickable
		var ops op.Ops
		type C = layout.Context
		type D = layout.Dimensions
		for {
			switch e := window.Event().(type) {
			case app.DestroyEvent:
				os.Exit(0)
			case app.FrameEvent:
				// This graphics context is used for managing the rendering state.
				gtx := app.NewContext(&ops, e)

				//nolint:exhaustruct
				layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx C) D {
					return layout.Flex{
						Axis: layout.Vertical,
					}.Layout(gtx,
						layout.Rigid(func(gtx C) D {
							label := material.Label(theme, unit.Sp(12), substackUrls[0])
							label.MaxLines = 1
							return label.Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
						layout.Rigid(material.Label(theme, unit.Sp(16), "Daily or normal?").Layout),
						layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
						layout.Rigid(func(gtx C) D {
							return layout.Flex{
								Axis: layout.Horizontal,
							}.Layout(gtx,
								layout.Rigid(func(gtx C) D {
									btn := material.Button(theme, &dailyButton, "Daily")
									gtx.Constraints.Min.X = gtx.Dp(75)
									if dailyButton.Clicked(gtx) {
										sessionProcessedCount++
										_, err := db.Exec(`
											delete from blog_feeds where root_url = $1
										`, substackUrls[0])
										if err != nil {
											panic(err)
										}
										substackUrls = substackUrls[1:]
										if len(substackUrls) == 0 {
											fmt.Println("Done")
											os.Exit(0)
										}
										pageChan <- substackUrls[0] + "archive"
									}
									return btn.Layout(gtx)
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
								layout.Rigid(func(gtx C) D {
									btn := material.Button(theme, &normalButton, "Normal")
									gtx.Constraints.Min.X = gtx.Dp(75)
									if normalButton.Clicked(gtx) {
										sessionProcessedCount++
										substackUrls = substackUrls[1:]
										if len(substackUrls) == 0 {
											fmt.Println("Done")
											os.Exit(0)
										}
										pageChan <- substackUrls[0] + "archive"
									}
									return btn.Layout(gtx)
								}),
							)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
						layout.Rigid(func(gtx C) D {
							label := fmt.Sprintf("Progress: %d/%d", sessionProcessedCount, totalUrls)
							return material.Label(theme, unit.Sp(16), label).Layout(gtx)
						}),
					)
				})

				// Pass the drawing operations to the GPU.
				e.Frame(gtx.Ops)
			}
		}
	}()
	app.Main()
}
