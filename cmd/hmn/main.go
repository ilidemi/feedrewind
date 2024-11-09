package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	neturl "net/url"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"feedrewind.com/crawler"

	"github.com/goccy/go-json"
	"github.com/spf13/cobra"
	_ "modernc.org/sqlite"
	"mvdan.cc/xurls/v2"
)

func main() {
	err := os.Chdir("cmd/hmn") // Undo the hack that the development config init does
	if err != nil {
		panic(err)
	}

	rootCmd := &cobra.Command{
		Use: "hmn",
	}

	rootCmd.AddCommand(&cobra.Command{
		Use: "rename",
		Run: func(_ *cobra.Command, _ []string) {
			rename()
		},
	})

	rootCmd.AddCommand(&cobra.Command{
		Use:  "import-channel",
		Args: cobra.ExactArgs(1),
		Run: func(_ *cobra.Command, args []string) {
			importChannel(args[0])
		},
	})

	rootCmd.AddCommand(&cobra.Command{
		Use: "validate-urls",
		Run: func(_ *cobra.Command, _ []string) {
			validateUrls()
		},
	})

	rootCmd.AddCommand(&cobra.Command{
		Use: "count-hosts",
		Run: func(_ *cobra.Command, _ []string) {
			countHosts()
		},
	})

	rootCmd.AddCommand(&cobra.Command{
		Use: "find-feeds",
		Run: func(_ *cobra.Command, _ []string) {
			findFeeds()
		},
	})

	rootCmd.AddCommand(&cobra.Command{
		Use: "crawl",
		Run: func(_ *cobra.Command, _ []string) {
			crawl()
		},
	})

	rootCmd.AddCommand(&cobra.Command{
		Use: "export-lists",
		Run: func(_ *cobra.Command, _ []string) {
			exportLists()
		},
	})

	if err := rootCmd.Execute(); err != nil {
		panic(err)
	}
}

func rename() {
	entries, err := os.ReadDir("raw_messages")
	if err != nil {
		panic(err)
	}

	regex := regexp.MustCompile("Handmade Network - .+ - ([a-z-]+) \\[\\d+\\](\\.json)")

	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "Handmade Network") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			panic(err)
		}
		if info.IsDir() {
			continue
		}
		if info.Size() == 0 {
			continue
		}
		if time.Since(info.ModTime()) < 3*time.Second {
			continue
		}

		match := regex.FindStringSubmatch(entry.Name())
		if match == nil {
			panic(fmt.Errorf("couldn't match filename: %s", entry.Name()))
		}
		err = os.Rename("raw_messages/"+entry.Name(), "raw_messages/"+match[1]+match[2])
		if err != nil {
			panic(err)
		}
		fmt.Println(match[1])
	}
}

func importChannel(name string) {
	db := openDb()
	defer db.Close()

	var channels []string
	if name == "new" {
		existingChannels := map[string]bool{}
		rows, err := db.Query(`select distinct channel from raw_links`)
		if err != nil {
			panic(err)
		}
		for rows.Next() {
			var channel string
			err := rows.Scan(&channel)
			if err != nil {
				panic(err)
			}
			existingChannels[channel] = true
		}
		if err := rows.Err(); err != nil {
			panic(err)
		}

		entries, err := os.ReadDir("raw_messages")
		if err != nil {
			panic(err)
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), "Handmade Network") {
				continue
			}
			channel := entry.Name()[:len(entry.Name())-5]
			if !existingChannels[channel] {
				channels = append(channels, channel)
			}
		}
		if len(channels) == 0 {
			fmt.Println("Nothing new")
			return
		}
	} else if name == "all" {
		entries, err := os.ReadDir("raw_messages")
		if err != nil {
			panic(err)
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), "Handmade Network") {
				continue
			}
			channel := entry.Name()[:len(entry.Name())-5]
			channels = append(channels, channel)
		}
	} else {
		channels = []string{name}
	}

	for _, channel := range channels {
		var filenames []string
		singleFilename := "raw_messages/" + channel + ".json"
		_, err := os.Stat(singleFilename)
		if err == nil {
			filenames = []string{singleFilename}
		} else if errors.Is(err, os.ErrNotExist) {
			// no-op
		} else {
			panic(err)
		}
		dirName := "raw_messages/" + channel
		_, err = os.Stat(dirName)
		if err == nil {
			entries, err := os.ReadDir(dirName)
			if err != nil {
				panic(err)
			}
			for _, entry := range entries {
				filenames = append(filenames, dirName+"/"+entry.Name())
			}
		} else if errors.Is(err, os.ErrNotExist) {
			// no-op
		} else {
			panic(err)
		}
		_, err = db.Exec(`
			create table if not exists raw_links(
				url text not null,
				timestamp text not null,
				message_id integer not null,
				channel text not null,
				author text not null
			)
		`)
		if err != nil {
			panic(err)
		}
		rows, err := db.Query(`PRAGMA table_info(raw_links);`)
		if err != nil {
			panic(err)
		}
		hasThreadId := false
		for rows.Next() {
			var cid, notnull, pk int64
			var dfltValue *int64
			var name, typeStr string
			err := rows.Scan(&cid, &name, &typeStr, &notnull, &dfltValue, &pk)
			if err != nil {
				panic(err)
			}
			if name == "thread_id" {
				hasThreadId = true
			}
		}
		if err := rows.Err(); err != nil {
			panic(err)
		}
		if !hasThreadId {
			_, err := db.Exec(`alter table raw_links add column thread_id integer`)
			if err != nil {
				panic(err)
			}
		}
		result, err := db.Exec(`
			delete from raw_links where channel = $1
		`, channel)
		if err != nil {
			panic(err)
		}
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			panic(err)
		}
		if rowsAffected > 0 {
			fmt.Printf("%s: deleted %d\n", channel, rowsAffected)
		}

		type RawLink struct {
			Url       string
			Timestamp string
			MessageId int64
			ThreadId  *int64
			Author    string
		}
		var rawLinks []RawLink
		for _, filename := range filenames {
			jsonBytes, err := os.ReadFile(filename)
			if err != nil {
				panic(err)
			}
			type Export struct {
				Channel struct {
					Id         string
					CategoryId *string
				}
				Messages []struct {
					Id        string
					Timestamp string
					Content   string
					Author    struct {
						Name string
					}
				}
			}
			var export Export
			err = json.Unmarshal(jsonBytes, &export)
			if err != nil {
				panic(err)
			}

			var threadId *int64
			if export.Channel.CategoryId != nil {
				threadIdVal, err := strconv.ParseInt(export.Channel.Id, 10, 64)
				if err != nil {
					panic(err)
				}
				threadId = &threadIdVal
			}

			urlRegex := xurls.Strict()
			for _, message := range export.Messages {
				messageId, err := strconv.ParseInt(message.Id, 10, 64)
				if err != nil {
					panic(err)
				}
				urls := urlRegex.FindAllString(message.Content, -1)
				for _, url := range urls {
					if !(strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")) {
						continue
					}
					if strings.Contains(url, "localhost:") {
						continue
					}
					_, err := neturl.Parse(url)
					if err != nil {
						continue
					}
					rawLinks = append(rawLinks, RawLink{
						Url:       url,
						Timestamp: message.Timestamp,
						MessageId: messageId,
						ThreadId:  threadId,
						Author:    message.Author.Name,
					})
				}
			}
		}

		tx, err := db.Begin()
		if err != nil {
			panic(err)
		}
		stmt, err := tx.Prepare(`
			insert into raw_links (url, timestamp, message_id, thread_id, channel, author)
			values ($1, $2, $3, $4, $5, $6)
		`)
		if err != nil {
			panic(err)
		}
		for _, rawLink := range rawLinks {
			_, err := stmt.Exec(
				rawLink.Url, rawLink.Timestamp, rawLink.MessageId, rawLink.ThreadId, channel, rawLink.Author,
			)
			if err != nil {
				panic(err)
			}
		}
		if err := stmt.Close(); err != nil {
			panic(err)
		}
		if err := tx.Commit(); err != nil {
			panic(err)
		}
		fmt.Printf("%s: inserted %d\n", channel, len(rawLinks))
	}
}

func validateUrls() {
	db := openDb()
	defer db.Close()

	rows, err := db.Query(`select rowid, url from raw_links`)
	if err != nil {
		panic(err)
	}
	for rows.Next() {
		var rowId int64
		var url string
		err := rows.Scan(&rowId, &url)
		if err != nil {
			panic(err)
		}

		_, err = neturl.Parse(url)
		if err != nil {
			fmt.Printf("%d %s: %v\n", rowId, url, err)
		}
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}

}

var skippedHosts map[string]bool

func init() {
	skippedHostsList := []string{
		"github.com", "youtube.com", "youtu.be", "twitter.com", "discord.com", "tenor.com",
		"en.wikipedia.org", "gist.github.com", "i.imgur.com", "cdn.discordapp.com", "handmade.network",
		"developer.mozilla.org", "stackoverflow.com", "developer.apple.com", "learn.microsoft.com",
		"gfycat.com", "desmos.com", "shadertoy.com", "godbolt.org", "store.steampowered.com",
		"news.ycombinator.com", "streamable.com", "twitch.tv", "amazon.com", "media.discordapp.net",
		"reddit.com", "docs.microsoft.com", "archive.org", "wiki.archlinux.org", "gitlab.com", "discord.gg",
		"arxiv.org", "discordapp.com", "fxtwitter.com", "x.com", "git.sr.ht", "pastebin.com", "vxtwitter.com",
		"gdcvault.com", "imgur.com", "math.stackexchange.com", "en.m.wikipedia.org", "web.archive.org",
		"iquilezles.org", "bitbucket.org", "en.cppreference.com", "media.handmade-seattle.com",
		"hero.handmade.network", "en.wiktionary.org", "gitlab.freedesktop.org",
	}
	skippedHosts = map[string]bool{}
	for _, host := range skippedHostsList {
		skippedHosts[host] = true
	}
}

func countHosts() {
	db := openDb()
	defer db.Close()

	var urls []string
	rows, err := db.Query(`select url from raw_links`)
	if err != nil {
		panic(err)
	}
	for rows.Next() {
		var url string
		err := rows.Scan(&url)
		if err != nil {
			panic(err)
		}
		urls = append(urls, url)
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}

	canonicalUrls := map[string]bool{}
	for _, url := range urls {
		canonicalUrl := url
		canonicalUrl = strings.TrimPrefix(canonicalUrl, "http://")
		canonicalUrl = strings.TrimPrefix(canonicalUrl, "https://")
		canonicalUrl = strings.TrimPrefix(canonicalUrl, "www.")
		canonicalUrls[canonicalUrl] = true
	}

	countByHost := map[string]int{}
	for url := range canonicalUrls {
		slashIndex := strings.Index(url, "/")
		if slashIndex == -1 {
			slashIndex = len(url)
		}
		host := url[:slashIndex]
		if skippedHosts[host] {
			continue
		}
		countByHost[host]++
	}

	var hosts []string
	for host := range countByHost {
		hosts = append(hosts, host)
	}
	slices.SortFunc(hosts, func(a, b string) int {
		return countByHost[b] - countByHost[a]
	})
	for _, host := range hosts {
		if countByHost[host] > 2 {
			fmt.Println(host, countByHost[host])
		}
	}
}

func findFeeds() {
	db := openDb()
	defer db.Close()

	_, err := db.Exec(`
		create table if not exists discarded_links(
			link_id integer not null,
			reason text not null
		)
	`)
	if err != nil {
		panic(err)
	}
	_, err = db.Exec(`
		create table if not exists matched_links(
			link_id integer not null,
			feed_root_canonical_url text not null
		)
	`)
	if err != nil {
		panic(err)
	}
	_, err = db.Exec(`
		create table if not exists feeds(
			root_canonical_url text primary key,
			root_url text not null,
			feed_url text
		)
	`)
	if err != nil {
		panic(err)
	}

	urlsByRowId := map[int64]string{}
	rows, err := db.Query(`
		select rowid, url from raw_links
		where rowid not in (select link_id from discarded_links) and
			rowid not in (select link_id from matched_links)
	`)
	if err != nil {
		panic(err)
	}
	for rows.Next() {
		var rowId int64
		var url string
		err := rows.Scan(&rowId, &url)
		if err != nil {
			panic(err)
		}
		urlsByRowId[rowId] = url
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}

	var skippedCount int
	var toProcessCount int
	urlRowIdsByHost := map[string][]int64{}
	uniqueUrls := map[string]bool{}
	for rowId, url := range urlsByRowId {
		uri, err := neturl.Parse(url)
		if err != nil {
			panic(err)
		}
		host := uri.Hostname()
		host = strings.TrimPrefix(host, "www.")
		if skippedHosts[host] {
			_, err := db.Exec(`
				insert into discarded_links (link_id, reason) values ($1, 'skipped_host')
			`, rowId)
			if err != nil {
				panic(err)
			}
			skippedCount++
		} else {
			uniqueUrls[url] = true
			urlRowIdsByHost[host] = append(urlRowIdsByHost[host], rowId)
			toProcessCount++
		}
	}
	fmt.Println("New skipped:", skippedCount)
	fmt.Println("Unique urls:", len(uniqueUrls))
	fmt.Println("Unique hosts:", len(urlRowIdsByHost))

	var popularHostCount int
	for _, urlRowIds := range urlRowIdsByHost {
		if len(urlRowIds) >= 2 {
			popularHostCount++
		}
	}
	fmt.Println("Popular hosts:", popularHostCount)

	urlRowIdsByHostByCount := map[int]map[string]map[int64]bool{}
	for host, urlRowIds := range urlRowIdsByHost {
		if _, ok := urlRowIdsByHostByCount[len(urlRowIds)]; !ok {
			urlRowIdsByHostByCount[len(urlRowIds)] = map[string]map[int64]bool{}
		}
		if _, ok := urlRowIdsByHostByCount[len(urlRowIds)][host]; !ok {
			urlRowIdsByHostByCount[len(urlRowIds)][host] = map[int64]bool{}
		}
		for _, urlRowId := range urlRowIds {
			urlRowIdsByHostByCount[len(urlRowIds)][host][urlRowId] = true
		}
	}
	var hostCounts []int
	for count := range urlRowIdsByHostByCount {
		hostCounts = append(hostCounts, count)
	}
	slices.SortFunc(hostCounts, func(a, b int) int {
		return b - a
	})
	type UrlRowIdsByHostCount struct {
		InitialCount    int
		UrlRowIdsByHost map[string]map[int64]bool
	}
	var urlRowIdsByHostCounts []UrlRowIdsByHostCount
	for _, count := range hostCounts {
		urlRowIdsByHostCounts = append(urlRowIdsByHostCounts, UrlRowIdsByHostCount{
			InitialCount:    count,
			UrlRowIdsByHost: urlRowIdsByHostByCount[count],
		})
	}

	const workerCount = 32
	var wg sync.WaitGroup
	wg.Add(workerCount)
	activeHosts := map[string]bool{}
	processedCount := 0
	var lock sync.Mutex
	startTime := time.Now()
	for workerId := range workerCount {
		go func() {
			defer wg.Done()
			for {
				lock.Lock()
				var initialUrlRowId int64 = -1
				var initialUrl string
			pickUrlLoop:
				for _, urlRowIdsByHostCount := range urlRowIdsByHostCounts {
					for host, urlRowIds := range urlRowIdsByHostCount.UrlRowIdsByHost {
						if !activeHosts[host] {
							for urlRowId := range urlRowIds {
								initialUrlRowId = urlRowId
								var ok bool
								initialUrl, ok = urlsByRowId[urlRowId]
								if !ok {
									panic(fmt.Errorf("url not found: %d", urlRowId))
								}
								delete(urlRowIds, urlRowId)
								break pickUrlLoop
							}
						}
					}
				}
				if initialUrlRowId == -1 {
					fmt.Printf("w%02d: done\n", workerId)
					lock.Unlock()
					break
				}

				initialCanonicalUrl := toCanonicalUrl(initialUrl)
				row := db.QueryRow(`
					select root_canonical_url from feeds where $1 like root_canonical_url || '%'
				`, initialCanonicalUrl)
				var feedRootCanonicalUrl string
				err := row.Scan(&feedRootCanonicalUrl)
				if errors.Is(err, sql.ErrNoRows) {
					// no-op, will need a fetch
				} else if err != nil {
					panic(err)
				} else {
					_, err := db.Exec(`
						insert into matched_links (link_id, feed_root_canonical_url)
						values ($1, $2)
					`, initialUrlRowId, feedRootCanonicalUrl)
					if err != nil {
						panic(err)
					}
					processedCount++
					lock.Unlock()
					continue
				}
				activeHost := initialCanonicalUrl
				if index := strings.Index(initialCanonicalUrl, "/"); index != -1 {
					activeHost = initialCanonicalUrl[:index]
				}
				activeHosts[activeHost] = true
				lock.Unlock()

				uri, err := neturl.Parse(initialUrl)
				if err != nil {
					panic(err)
				}
				if uri.Path == "/" {
					uri.Path = ""
				}

				var feedUrl string
				var lastFeedUrl string
				var rootUrl string
				logger := crawler.NewDummyLogger()
				crawlCtx := crawler.NewCrawlContext(
					crawler.NewHttpClientImpl(context.Background(), nil, true), //nolint:gocritic
					nil,
					crawler.NewMockProgressLogger(logger),
				)
				requests := 0
				requestsStartTime := time.Now()
				for {
					result := crawler.DiscoverFeedsAtUrl(uri.String(), true, &crawlCtx, logger)
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
							rootUrl = uri.String()
						}
					}
					if uri.Path == "" && uri.RawQuery == "" {
						break
					}
					if uri.RawQuery != "" {
						uri.RawQuery = ""
					} else {
						uri.Path = strings.TrimSuffix(uri.Path, "/")
						uri.Path = uri.Path[:strings.LastIndex(uri.Path, "/")+1]
						if uri.Path == "/" {
							uri.Path = ""
						}
					}
					uri.Fragment = ""
					uri.RawFragment = ""
				}

				lock.Lock()
				if feedUrl == "" {
					_, err := db.Exec(`
						insert into discarded_links (link_id, reason) values ($1, 'feed_not_found')
					`, initialUrlRowId)
					if err != nil {
						panic(err)
					}
				} else {
					rootCanonicalUrl := toCanonicalUrl(rootUrl)
					_, err := db.Exec(`
						insert into feeds (root_canonical_url, root_url, feed_url)
						values ($1, $2, $3)
					`, rootCanonicalUrl, rootUrl, feedUrl)
					if err != nil {
						panic(err)
					}
					_, err = db.Exec(`
						insert into matched_links (link_id, feed_root_canonical_url)
						values ($1, $2)
					`, initialUrlRowId, rootCanonicalUrl)
					if err != nil {
						panic(err)
					}
				}
				delete(activeHosts, activeHost)
				processedCount++
				doneFraction := float64(processedCount) / float64(toProcessCount)
				donePercent := doneFraction * 100
				timeRemaining := (time.Since(startTime) * time.Duration(toProcessCount-processedCount) /
					time.Duration(processedCount)).Round(time.Second)
				fmt.Printf(
					"w%02d: %d/%d, %.1f%%, %v remaining (%d req, %v)\n",
					workerId, processedCount, toProcessCount, donePercent, timeRemaining,
					requests, time.Since(requestsStartTime).Round(time.Second),
				)
				lock.Unlock()
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

type ProgressSaver struct {
	RootCanonicalUrl string
}

func (s *ProgressSaver) SaveStatusAndCount(status string, maybeCount *int) error {
	fmt.Printf("%s: status: %s count: %s\n", s.RootCanonicalUrl, status, SprintIntPtr(maybeCount))
	return nil
}

func (s *ProgressSaver) SaveStatus(status string) error {
	fmt.Printf("%s: status: %s\n", s.RootCanonicalUrl, status)
	return nil
}

func (s *ProgressSaver) SaveCount(maybeCount *int) error {
	fmt.Printf("%s: count: %s\n", s.RootCanonicalUrl, SprintIntPtr(maybeCount))
	return nil
}

func (s *ProgressSaver) EmitTelemetry(regressions string, extra map[string]any) {
}

func SprintIntPtr(value *int) string {
	if value == nil {
		return "nil"
	} else {
		return fmt.Sprint(*value)
	}
}

func crawl() {
	db := openDb()
	defer db.Close()

	_, err := db.Exec(`
		create table if not exists post_lists (
			feed_root_canonical_url text primary key,
			post_titles text not null
		)
	`)
	if err != nil {
		panic(err)
	}
	_, err = db.Exec(`
		create table if not exists crawl_failures (
			feed_root_canonical_url text primary key,
			reason text not null
		)
	`)
	if err != nil {
		panic(err)
	}

	rootUrlByRootCanonicalUrl := map[string]string{}
	rows, err := db.Query(`
		WITH limited_urls AS (
		    SELECT
		        ml.link_id,
		        ml.feed_root_canonical_url,
		        rl.author,
		        rl.channel,
		        rl.url,
		        ROW_NUMBER() OVER (
		            PARTITION BY ml.feed_root_canonical_url, rl.url
		            ORDER BY RANDOM()
		        ) AS rn_url
		    FROM matched_links ml
		    JOIN raw_links rl ON ml.link_id = rl.rowid
		    where (rl.url not like '%bvisness.me/apps%' and
		    		(rl.url not like '%bvisness.me/%' or author != 'bvisness')
		    	) and
		    	(rl.url not like '%rfleury.com/%' or author != 'ryanfleury') and
		    	author != 'abnercoimbre'
		),
		selected_urls AS (
		    SELECT *
		    FROM limited_urls
		    WHERE rn_url <= 3
		),
		limited_authors AS (
		    SELECT
		        su.link_id,
		        su.feed_root_canonical_url,
		        su.author,
		        su.channel,
		        su.url,
		        ROW_NUMBER() OVER (
		            PARTITION BY su.feed_root_canonical_url, su.author
		            ORDER BY su.link_id
		        ) AS rn_author
		    FROM selected_urls su
		),
		selected_links AS (
		    SELECT *
		    FROM limited_authors
		    WHERE rn_author <= 3
		),
		mlinks AS (
			SELECT
			    sl.feed_root_canonical_url,
			    COUNT(*) AS total_matches,
			    -- Top Author
			    (
			        SELECT sl2.author || ' (' || COUNT(*) || ')'
			        FROM selected_links sl2
			        WHERE sl2.feed_root_canonical_url = sl.feed_root_canonical_url
			        GROUP BY sl2.author
			        ORDER BY COUNT(*) DESC
			        LIMIT 1
			    ) AS top_author,
			    -- Top Channel
			    (
			        SELECT sl3.channel || ' (' || COUNT(*) || ')'
			        FROM selected_links sl3
			        WHERE sl3.feed_root_canonical_url = sl.feed_root_canonical_url
			        GROUP BY sl3.channel
			        ORDER BY COUNT(*) DESC
			        LIMIT 1
			    ) AS top_channel,
			    -- Library Count
			    (
			        SELECT COUNT(*)
			        FROM selected_links sl4
			        WHERE sl4.feed_root_canonical_url = sl.feed_root_canonical_url
			          AND sl4.channel = 'the-library'
			    ) AS library_count
			FROM selected_links sl
			GROUP BY sl.feed_root_canonical_url
			ORDER BY total_matches DESC
		)
		SELECT feed_root_canonical_url,
			(select root_url from feeds where root_canonical_url = mlinks.feed_root_canonical_url) as root_url
		FROM mlinks
		WHERE (total_matches >= 6 or library_count >= 3) and
			feed_root_canonical_url not in (
				'khronos.org/', 'gafferongames.com', 'cs.cmu.edu/', 'odin-lang.org', 'gnu.org/', 'theverge.com/',
				'jcgt.org/', 'lwn.net/', 'microsoft.com/en-us/research/', 'old.reddit.com/', 'unicode.org/',
				'joelonsoftware.com/', 'code.visualstudio.com/', 'dev.to/', 'gpuopen.com/', 'phoronix.com/', 
				'developer.chrome.com/', 'gamedev.stackexchange.com/', 'github.blog/', 'gravitymoth.com/', 
				'devblogs.microsoft.com/directx/', 'thephd.dev/', 'visualstudio.microsoft.com/', 'caniuse.com',
				'copetti.org/', 'git.musl-libc.org/cgit/musl/', 'theorangeduck.com/', 'aur.archlinux.org/', 
				'codegolf.stackexchange.com/', 'kickstarter.com/projects/', 'scattered-thoughts.net/', 
				'unix.stackexchange.com/', 'worrydream.com/', 'asawicki.info/', 'blog.cloudflare.com/',
				'blogs.windows.com/', 'css-tricks.com/', 'docs.rs/', 'wired.com/', 'web.dev/', 'warp.dev',
				'pcg-random.org/', 'wiki.libsdl.org/', 'unrealengine.com/', 'sublimetext.com/', 'sourceforge.net/',
				'pikuma.com/', 'devblogs.microsoft.com/visualstudio/', 'devblogs.microsoft.com/commandline/',
				'buttondown.email/hillelwayne/', 'bun.sh', 'andrewkelley.me/', 'techcrunch.com/', 'superuser.com/',
				'rudyfaile.com/', 'marctenbosch.com/', 'macrumors.com/', 'kotaku.com/', 'hacks.mozilla.org/',
				'gamedeveloper.com/', 'gamasutra.com/', 'eurogamer.net/', 'catch22.net/', 'bottosson.github.io/',
				'blog.selfshadow.com/', 'archlinux.org/packages/', 'tonsky.me/', 'theregister.com/',
				'steamdb.info/', 'solhsa.com/', 'love2d.org', 'hillelwayne.com/', 'go.dev/blog/', 
				'git.kernel.org/pub/scm/linux/kernel/git/torvalds/linux.git/', 'ffmpeg.org/',
				'eli.thegreenplace.net/', 'dev.cancel.fm/', 'box2d.org/', '01.org/', 'snapnet.dev/', 
				'positech.co.uk/cliffsblog/', 'alain.xyz/', 'yosoygames.com.ar/', 'samwho.dev/',
				'kevroletin.github.io/', 'devblogs.microsoft.com/premier-developer/', 'cgg.mff.cuni.cz/',
				'ryanfleury.substack.com', 'devblogs.microsoft.com/cppblog/'
			) and
			feed_root_canonical_url not in (select feed_root_canonical_url from post_lists) and
			feed_root_canonical_url not in (select feed_root_canonical_url from crawl_failures)
	`)
	if err != nil {
		panic(err)
	}
	for rows.Next() {
		var rootCanonicalUrl, rootUrl string
		err := rows.Scan(&rootCanonicalUrl, &rootUrl)
		if err != nil {
			panic(err)
		}
		rootUrlByRootCanonicalUrl[rootCanonicalUrl] = rootUrl
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}

	var wg sync.WaitGroup
	wg.Add(len(rootUrlByRootCanonicalUrl))
	crawler.SetMaxBrowserCount(len(rootUrlByRootCanonicalUrl))
	var lock sync.Mutex
	for rootCanonicalUrl, rootUrl := range rootUrlByRootCanonicalUrl {
		go func() {
			defer wg.Done()

			puppeteerClient := crawler.NewPuppeteerClientImpl()
			logger := crawler.NewDummyLogger()
			progressSaver := ProgressSaver{
				RootCanonicalUrl: rootCanonicalUrl,
			}
			crawlCtx := crawler.NewCrawlContext(
				crawler.NewHttpClientImpl(context.Background(), nil, true), //nolint:gocritic
				puppeteerClient,
				crawler.NewProgressLogger(&progressSaver),
			)
			feedsResult := crawler.DiscoverFeedsAtUrl(rootUrl, false, &crawlCtx, logger)
			singleFeed, ok := feedsResult.(*crawler.DiscoveredSingleFeed)
			if !ok {
				panic(fmt.Errorf("single feed not present for %s: %v", rootCanonicalUrl, feedsResult))
			}

			feed := crawler.Feed{
				Title:    singleFeed.Feed.Title,
				Url:      singleFeed.Feed.Url,
				FinalUrl: singleFeed.Feed.FinalUrl,
				Content:  singleFeed.Feed.Content,
			}
			guidedCrawlResult, crawlErr :=
				crawler.GuidedCrawl(singleFeed.MaybeStartPage, feed, &crawlCtx, logger)
			if crawlErr == nil && guidedCrawlResult.HardcodedError != nil {
				crawlErr = fmt.Errorf("hardcoded error: %v", guidedCrawlResult.HardcodedError)
			}
			if crawlErr == nil && guidedCrawlResult.HistoricalError != nil {
				crawlErr = fmt.Errorf("historical error: %v", guidedCrawlResult.HistoricalError)
			}

			lock.Lock()
			if crawlErr != nil {
				_, err := db.Exec(`
					insert into crawl_failures (feed_root_canonical_url, reason)
					values ($1, $2)
				`, rootCanonicalUrl, crawlErr.Error())
				if err != nil {
					panic(err)
				}
				fmt.Printf("%s: error %v\n", rootCanonicalUrl, crawlErr)
			} else {
				var sb strings.Builder
				sb.WriteString("Blog: ")
				sb.WriteString(feed.Title)
				sb.WriteString("\n")
				for _, link := range guidedCrawlResult.HistoricalResult.Links {
					sb.WriteString(link.Title.Value)
					sb.WriteString("\n")
				}
				_, err := db.Exec(`
					insert into post_lists (feed_root_canonical_url, post_titles)
					values ($1, $2)
				`, rootCanonicalUrl, sb.String())
				if err != nil {
					panic(err)
				}
				titleCount := len(guidedCrawlResult.HistoricalResult.Links)
				fmt.Printf("%s: inserted %d titles\n", rootCanonicalUrl, titleCount)
			}
			lock.Unlock()
		}()
	}
	wg.Wait()
}

func exportLists() {
	db := openDb()
	defer db.Close()

	var sb strings.Builder
	rows, err := db.Query(`select feed_root_canonical_url, post_titles from post_lists`)
	if err != nil {
		panic(err)
	}
	for rows.Next() {
		var url, titles string
		err := rows.Scan(&url, &titles)
		if err != nil {
			panic(err)
		}
		sb.WriteString(url)
		sb.WriteString("\n")
		sb.WriteString(titles)
		sb.WriteString("\n\n")
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}

	err = os.WriteFile("titles.txt", []byte(sb.String()), 0666)
	if err != nil {
		panic(err)
	}
}

func openDb() *sql.DB {
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
