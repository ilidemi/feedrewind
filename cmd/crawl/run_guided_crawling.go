package crawl

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"time"

	"feedrewind.com/crawler"
	"feedrewind.com/db/pgw"
	"feedrewind.com/oops"

	"github.com/jackc/pgx/v5"
)

type GuidedCrawlingResult struct {
	StartUrl                                     string `eval:"neutral"`
	Source                                       string `eval:"neutral"`
	Comment                                      string `eval:"neutral"`
	CommentStatus                                crawler.Status
	GroundTruthPattern                           string `eval:"neutral"`
	FeedUrl                                      string `eval:"neutral"`
	FeedLinks                                    int    `eval:"neutral"`
	BlogUrl                                      string `eval:"neutral_present"`
	BlogUrlStatus                                crawler.Status
	HistoricalLinksFound                         bool   `eval:"boolean"`
	HistoricalLinksMatching                      string `eval:"boolean"`
	HistoricalLinksMatchingStatus                crawler.Status
	HistoricalLinksPattern                       string `eval:"neutral_present"`
	HistoricalLinksPatternStatus                 crawler.Status
	HistoricalLinksCount                         string `eval:"neutral_present"`
	HistoricalLinksCountStatus                   crawler.Status
	HistoricalLinksTitlesPartiallyMatching       string `eval:"neutral_present"`
	HistoricalLinksTitlesPartiallyMatchingStatus crawler.Status
	HistoricalLinksTitlesExactlyMatching         string `eval:"neutral_present"`
	HistoricalLinksTitlesExactlyMatchingStatus   crawler.Status
	HistoricalLinksTitlesMatchingFeed            string `eval:"neutral_present"`
	HistoricalLinksTitlesMatchingFeedStatus      crawler.Status
	NoGuidedRegression                           Ternary `eval:"neutral"`
	NoGuidedRegressionStatus                     crawler.Status
	MainUrl                                      string `eval:"neutral_present"`
	MainUrlStatus                                crawler.Status
	OldestLink                                   string `eval:"neutral_present"`
	OldestLinkStatus                             crawler.Status
	Extra                                        []string `eval:"neutral"`
	TotalRequests                                int      `eval:"neutral"`
	TotalPages                                   int      `eval:"neutral"`
	TotalNetworkRequests                         int      `eval:"neutral"`
	DuplicateFetches                             int      `eval:"neutral"`
	TitleRequests                                int      `eval:"neutral"`
	TotalTime                                    int      `eval:"neutral"`
}

type Ternary int

const (
	TernaryUnknown Ternary = iota
	TernaryTrue
	TernaryFalse
)

func (t Ternary) String() string {
	return []string{"", "true", "false"}[t]
}

func (r GuidedCrawlingResult) ColumnValues() []any {
	st := reflect.TypeOf(r)
	value := reflect.ValueOf(r)
	var columnValues []any
	for i := 0; i < value.NumField(); i++ {
		field := st.Field(i)
		if strings.HasSuffix(field.Name, "Status") {
			continue
		}
		columnValue := value.Field(i).Interface()
		columnValues = append(columnValues, columnValue)
	}
	if len(columnValues) != len(GuidedCrawlingColumnNames) {
		panic(fmt.Errorf(
			"Column values count mismatch: got %d, expected %d",
			len(columnValues), len(GuidedCrawlingColumnNames),
		))
	}
	return columnValues
}

func (r GuidedCrawlingResult) ColumnStatuses() []crawler.Status {
	st := reflect.TypeOf(r)
	value := reflect.ValueOf(r)
	var columnStatuses []crawler.Status
	for i := 0; i < value.NumField(); i++ {
		field := st.Field(i)
		if strings.HasSuffix(field.Name, "Status") {
			continue
		}
		statusField := value.FieldByName(field.Name + "Status")
		if statusField != (reflect.Value{}) {
			manualStatus := statusField.Interface().(crawler.Status)
			if manualStatus != crawler.StatusNone {
				columnStatuses = append(columnStatuses, manualStatus)
				continue
			}
		}

		var columnStatus crawler.Status
		fieldValue := value.Field(i)
		tag := field.Tag.Get("eval")
		switch tag {
		case "neutral":
			columnStatus = crawler.StatusNeutral
		case "neutral_present":
			if fieldValue.IsZero() {
				columnStatus = crawler.StatusFailure
			} else {
				columnStatus = crawler.StatusNeutral
			}
		case "boolean":
			if fieldValue.Bool() {
				columnStatus = crawler.StatusSuccess
			} else {
				columnStatus = crawler.StatusFailure
			}
		default:
			panic("Unknown eval tag: " + tag)
		}
		columnStatuses = append(columnStatuses, columnStatus)
	}
	if len(columnStatuses) != len(GuidedCrawlingColumnNames) {
		panic(fmt.Errorf(
			"Column statuses count mismatch: got %d, expected %d",
			len(columnStatuses), len(GuidedCrawlingColumnNames),
		))
	}
	return columnStatuses
}

var GuidedCrawlingColumnNames []string

func init() {
	replaceStrings := make([]string, 0, 52)
	for c := 'A'; c <= 'Z'; c++ {
		replaceStrings = append(replaceStrings, string(c))
		replaceStrings = append(replaceStrings, " "+string(c-'A'+'a'))
	}
	replacer := strings.NewReplacer(replaceStrings...)
	st := reflect.TypeOf(GuidedCrawlingResult{}) //nolint:exhaustruct
	for i := 0; i < st.NumField(); i++ {
		field := st.Field(i)
		if strings.HasSuffix(field.Name, "Status") {
			continue
		}
		columnName := replacer.Replace(field.Name)[1:]
		GuidedCrawlingColumnNames = append(GuidedCrawlingColumnNames, columnName)
	}
}

type GuidedCrawlingError struct {
	Inner  *oops.Error
	Result GuidedCrawlingResult
}

func (e GuidedCrawlingError) Error() string {
	return e.Inner.Error()
}

func (e GuidedCrawlingError) Unwrap() error {
	return e.Inner
}

func newError(inner error, result GuidedCrawlingResult) *GuidedCrawlingError {
	oopsErr, ok := inner.(*oops.Error)
	if !ok {
		oopsErr = oops.Wrap(inner).(*oops.Error)
	}
	return &GuidedCrawlingError{
		Inner:  oopsErr,
		Result: result,
	}
}

func runGuidedCrawl(
	startLinkId int, saveSuccesses bool, allowJS bool, conn *pgw.Conn, logger crawler.Logger,
) (*GuidedCrawlingResult, error) {
	startLinkRow := conn.QueryRow(`select source, url, rss_url from start_links where id = $1`, startLinkId)
	var startLinkUrl *string
	var startLinkFeedUrl *string
	var result GuidedCrawlingResult
	err := startLinkRow.Scan(&result.Source, &startLinkUrl, &startLinkFeedUrl)
	if err != nil {
		return nil, newError(err, result)
	}
	if startLinkUrl == nil && startLinkFeedUrl == nil {
		err := oops.New("url and rss_url are both null")
		return nil, newError(err, result)
	}

	if startLinkUrl != nil {
		result.StartUrl = fmt.Sprintf(`<a href="%[1]s">%[1]s</a>`, *startLinkUrl)
	} else {
		result.StartUrl = fmt.Sprintf(`<a href="%[1]s">%[1]s</a>`, *startLinkFeedUrl)
	}
	mockHttpClient := NewMockHttpClient(conn, startLinkId)
	var puppeteerClient crawler.PuppeteerClient
	if allowJS {
		puppeteerClient = NewCachingPuppeteerClient(conn, startLinkId)
	} else {
		puppeteerClient = NewMockPuppeteerClient(conn, startLinkId)
	}

	tempProgressLogger := crawler.NewMockProgressLogger(crawler.NewDummyLogger())
	crawlCtx := crawler.NewCrawlContext(&mockHttpClient, puppeteerClient, tempProgressLogger)
	startTime := time.Now()

	defer func() {
		result.DuplicateFetches = crawlCtx.DuplicateFetches
		result.TotalRequests = crawlCtx.RequestsMade + crawlCtx.PuppeteerRequestsMade
		result.TotalPages = crawlCtx.FetchedCuris.Length
		result.TotalNetworkRequests = mockHttpClient.NetworkRequestsMade + crawlCtx.PuppeteerRequestsMade
		result.TitleRequests = crawlCtx.TitleRequestsMade
		result.TotalTime = int(math.Round(time.Since(startTime).Seconds()))
	}()

	{
		commentRow := conn.QueryRow(`
			select severity, issue from known_issues where start_link_id = $1
		`, startLinkId)
		var issue, severity string
		err := commentRow.Scan(&severity, &issue)
		if errors.Is(err, pgx.ErrNoRows) {
			// Do nothing
		} else if err != nil {
			return &result, newError(err, result)
		} else {
			result.Comment = issue
			if severity == "fail" {
				result.CommentStatus = crawler.StatusFailure
				err := oops.Newf("Known issue: %s", issue)
				return &result, newError(err, result)
			}
		}
	}

	gtTables := []string{"go_historical_ground_truth", "historical_ground_truth"}
	var gtKnown bool
	var gtPattern, gtBlogCanonicalUrl, gtMainPageCanonicalUrl, gtOldestEntryCanonicalUrl string
	var gtEntriesCount int
	var gtTitleStrs, gtLinks []string
tables:
	for _, gtTable := range gtTables {
		query := fmt.Sprintf(`
			select
				pattern, entries_count, blog_canonical_url, main_page_canonical_url,
				oldest_entry_canonical_url, titles, links
			from %s
			where start_link_id = $1 
		`, gtTable)
		gtRow := conn.QueryRow(query, startLinkId)
		err := gtRow.Scan(
			&gtPattern, &gtEntriesCount, &gtBlogCanonicalUrl, &gtMainPageCanonicalUrl,
			&gtOldestEntryCanonicalUrl, &gtTitleStrs, &gtLinks,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			gtKnown = false
		} else if err != nil {
			return &result, newError(err, result)
		} else {
			gtKnown = true
			result.GroundTruthPattern = gtPattern
			break tables
		}
	}

	if allowJS {
		_, err := conn.Exec(`delete from mock_puppeteer_pages where start_link_id = $1`, startLinkId)
		if err != nil {
			return &result, newError(err, result)
		}
	}
	_, err = conn.Exec(`delete from historical where start_link_id = $1`, startLinkId)
	if err != nil {
		return &result, newError(err, result)
	}

	var startUrl string
	if startLinkFeedUrl != nil {
		startUrl = *startLinkFeedUrl
	} else {
		startUrl = *startLinkUrl
	}
	discoverFeedsResult := crawler.DiscoverFeedsAtUrl(startUrl, false, &crawlCtx, logger)
	err = nil
	var feed crawler.Feed
	var maybeStartPage *crawler.DiscoveredStartPage
	switch dResult := discoverFeedsResult.(type) {
	case *crawler.DiscoverFeedsErrorBadFeed:
		err = oops.Newf("Bad feed at %s", startUrl)
	case *crawler.DiscoverFeedsErrorCouldNotReach:
		err = oops.Newf("Could not reach feed at %s (%v)", startUrl, dResult.Error)
	case *crawler.DiscoverFeedsErrorNoFeeds:
		err = oops.Newf("No feeds at %s", startUrl)
	case *crawler.DiscoverFeedsErrorNotAUrl:
		err = oops.Newf("Not a url: %s", startUrl)
	case *crawler.DiscoveredMultipleFeeds:
		err = oops.Newf("Multiple feeds at %s", startUrl)
	case *crawler.DiscoveredSingleFeed:
		feed = crawler.Feed{
			Title:    dResult.Feed.Title,
			Url:      dResult.Feed.Url,
			FinalUrl: dResult.Feed.FinalUrl,
			Content:  dResult.Feed.Content,
		}
		maybeStartPage = dResult.MaybeStartPage
	default:
		panic("unknown discover feeds result type")
	}
	if err != nil {
		return &result, newError(err, result)
	}

	crawlCtx.ProgressLogger = crawler.NewMockProgressLogger(logger)
	guidedCrawlResult, err := crawler.GuidedCrawl(maybeStartPage, feed, &crawlCtx, logger)
	if err != nil {
		return &result, newError(err, result)
	}
	if guidedCrawlResult.HardcodedError != nil {
		return &result, newError(guidedCrawlResult.HardcodedError, result)
	}
	result.FeedUrl = guidedCrawlResult.FeedResult.Url
	result.FeedLinks = guidedCrawlResult.FeedResult.Links
	historicalResult := guidedCrawlResult.HistoricalResult
	historicalError := guidedCrawlResult.HistoricalError
	result.HistoricalLinksFound = historicalResult != nil

	var hasGuidedSucceededBefore bool
	pastSuccessRow := conn.QueryRow(`select 1 from guided_successes where start_link_id = $1`, startLinkId)
	var one int
	err = pastSuccessRow.Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		hasGuidedSucceededBefore = false
	} else if err != nil {
		return &result, newError(err, result)
	} else {
		hasGuidedSucceededBefore = true
	}

	if historicalResult == nil {
		if gtKnown {
			if gtBlogCanonicalUrl != "" {
				result.BlogUrl = fmt.Sprintf("(%s)", gtBlogCanonicalUrl)
			}
			result.BlogUrlStatus = crawler.StatusFailure
			result.HistoricalLinksPattern = fmt.Sprintf("(%s)", gtPattern)
			result.HistoricalLinksPatternStatus = crawler.StatusFailure
			result.HistoricalLinksCount = fmt.Sprintf("(%d)", gtEntriesCount)
			result.HistoricalLinksCountStatus = crawler.StatusFailure
			result.MainUrl = fmt.Sprintf("(%s)", gtMainPageCanonicalUrl)
			result.MainUrlStatus = crawler.StatusFailure
			result.OldestLink = fmt.Sprintf("(%s)", gtOldestEntryCanonicalUrl)
			result.OldestLinkStatus = crawler.StatusFailure
			if hasGuidedSucceededBefore {
				result.NoGuidedRegression = TernaryFalse
				result.NoGuidedRegressionStatus = crawler.StatusFailure
			}
		}
		if historicalError != nil {
			return &result, newError(historicalError, result)
		}
		err := oops.New("Historical links not found")
		return &result, newError(err, result)
	}

	historicalBlogLinkStr := historicalResult.BlogLink.Curi.String()
	historicalMainLinkStr := historicalResult.MainLink.Curi.String()
	entriesCount := len(historicalResult.Links)
	oldestLink := historicalResult.Links[len(historicalResult.Links)-1]
	logger.Info("Historical links: %d", entriesCount)
	for _, link := range historicalResult.Links {
		logger.Info("%s %s (%s)", link.Title.Value, link.Title.Source, link.Url)
	}
	result.HistoricalLinksTitlesMatchingFeed = guidedCrawlResult.FeedResult.MatchingTitles
	result.HistoricalLinksTitlesMatchingFeedStatus = guidedCrawlResult.FeedResult.MatchingTitlesStatus

	linkTitles := make([]crawler.LinkTitle, 0, len(historicalResult.Links))
	linkTitleValues := make([]string, 0, len(historicalResult.Links))
	linkCuris := make([]crawler.CanonicalUri, 0, len(historicalResult.Links))
	linkCuriStrs := make([]string, 0, len(historicalResult.Links))
	for _, link := range historicalResult.Links {
		linkTitles = append(linkTitles, link.Title)
		linkTitleValues = append(linkTitleValues, link.Title.Value)
		linkCuris = append(linkCuris, link.Curi)
		linkCuriStrs = append(linkCuriStrs, link.Curi.String())
	}

	_, err = conn.Exec(`
		insert into historical(
			start_link_id, pattern, entries_count, blog_canonical_url, main_page_canonical_url,
			oldest_entry_canonical_url, titles, links
		) values (
			$1, $2, $3, $4, $5, $6, $7, $8
		)
	`, startLinkId, historicalResult.Pattern, entriesCount, historicalBlogLinkStr, historicalMainLinkStr,
		oldestLink.Curi.String(), linkTitleValues, linkCuriStrs,
	)
	if err != nil {
		return nil, err
	}

	if gtKnown {
		historicalLinksMatching := true

		if gtBlogCanonicalUrl == historicalBlogLinkStr {
			result.BlogUrlStatus = crawler.StatusSuccess
			result.BlogUrl = htmlLink(historicalResult.BlogLink)
		} else {
			historicalLinksMatching = false
			result.BlogUrlStatus = crawler.StatusFailure
			result.BlogUrl = htmlLinkMismatch(historicalResult.BlogLink, gtBlogCanonicalUrl)
		}

		if historicalResult.Pattern == gtPattern {
			result.HistoricalLinksPatternStatus = crawler.StatusSuccess
			result.HistoricalLinksPattern = historicalResult.Pattern
		} else {
			historicalLinksMatching = false
			result.HistoricalLinksPatternStatus = crawler.StatusFailure
			result.HistoricalLinksPattern = fmt.Sprintf("%s<br>(%s)", historicalResult.Pattern, gtPattern)
		}

		var gtCuris []crawler.CanonicalUri
		if len(gtLinks) > 0 {
			gtCuris = make([]crawler.CanonicalUri, 0, len(gtLinks))
			for _, link := range gtLinks {
				gtCuris = append(gtCuris, crawler.CanonicalUriFromDbString(link))
			}
		}

		if entriesCount != gtEntriesCount {
			historicalLinksMatching = false
			result.HistoricalLinksCountStatus = crawler.StatusFailure
			result.HistoricalLinksCount = fmt.Sprintf("%d (%d)", entriesCount, gtEntriesCount)
			if len(gtCuris) > 0 {
				logger.Info("Ground truth links:")
				for _, curi := range gtCuris {
					logger.Info("%s", curi.String())
				}
			} else {
				logger.Info("Ground truth links not present")
			}
		} else {
			if len(gtCuris) > 0 {
				type LinkMismatch struct {
					Curi   crawler.CanonicalUri
					GTCuri crawler.CanonicalUri
				}
				var linkMismatches []LinkMismatch
				for i := 0; i < len(linkCuris); i++ {
					if !crawler.CanonicalUriEqual(linkCuris[i], gtCuris[i], guidedCrawlResult.CuriEqCfg) {
						linkMismatches = append(linkMismatches, LinkMismatch{
							Curi:   linkCuris[i],
							GTCuri: gtCuris[i],
						})
					}
				}
				if len(linkMismatches) == 0 {
					result.HistoricalLinksCountStatus = crawler.StatusSuccess
					result.HistoricalLinksCount = fmt.Sprint(entriesCount)
				} else {
					historicalLinksMatching = false
					result.HistoricalLinksCountStatus = crawler.StatusFailure
					result.HistoricalLinksCount =
						fmt.Sprintf("%d (uri mismatch: %d)", entriesCount, len(linkMismatches))
					logger.Info("Historical link mismatches (%d):", len(linkMismatches))
					for _, mismatch := range linkMismatches {
						logger.Info("%s != GT %s", mismatch.Curi.String(), mismatch.GTCuri.String())
					}
				}
			} else {
				result.HistoricalLinksCountStatus = crawler.StatusNeutral
			}
		}

		// Main page url is compared as FYI but doesn't affect status
		if historicalMainLinkStr == gtMainPageCanonicalUrl {
			result.MainUrl = htmlLink(historicalResult.MainLink)
		} else {
			result.MainUrl = htmlLinkMismatch(historicalResult.MainLink, gtMainPageCanonicalUrl)
		}

		gtOldestCuri := crawler.CanonicalUriFromDbString(gtOldestEntryCanonicalUrl)
		if crawler.CanonicalUriEqual(gtOldestCuri, oldestLink.Curi, guidedCrawlResult.CuriEqCfg) {
			result.OldestLinkStatus = crawler.StatusSuccess
			result.OldestLink = htmlLink(oldestLink.Link)
		} else {
			historicalLinksMatching = false
			result.OldestLinkStatus = crawler.StatusFailure
			result.OldestLink = htmlLinkMismatch(oldestLink.Link, gtOldestEntryCanonicalUrl)
		}

		var gtTitles []crawler.LinkTitle
		if len(gtTitleStrs) > 0 {
			gtTitles = make([]crawler.LinkTitle, 0, len(gtTitleStrs))
			for _, titleStr := range gtTitleStrs {
				gtTitles = append(
					gtTitles, crawler.NewLinkTitle(titleStr, crawler.LinkTitleSourceGroundTruth, nil),
				)
			}
		}
		if len(gtTitles) == 0 {
			logger.Info("Ground truth titles not present")
			result.HistoricalLinksTitlesPartiallyMatchingStatus = crawler.StatusNeutral
			result.HistoricalLinksTitlesExactlyMatchingStatus = crawler.StatusNeutral
		} else if entriesCount == len(gtTitles) {
			type TitleMismatch struct {
				Title   crawler.LinkTitle
				GTTitle crawler.LinkTitle
			}
			var exactTitleMismatches []TitleMismatch
			for i := 0; i < len(linkTitles); i++ {
				if linkTitles[i].EqualizedValue != gtTitles[i].EqualizedValue &&
					linkTitles[i].EqualizedValue != strings.TrimSpace(gtTitles[i].EqualizedValue) {

					exactTitleMismatches = append(exactTitleMismatches, TitleMismatch{
						Title:   linkTitles[i],
						GTTitle: gtTitles[i],
					})
				}
			}

			if len(exactTitleMismatches) == 0 {
				result.HistoricalLinksTitlesPartiallyMatchingStatus = crawler.StatusSuccess
				result.HistoricalLinksTitlesPartiallyMatching = fmt.Sprint(len(linkTitles))
				result.HistoricalLinksTitlesExactlyMatchingStatus = crawler.StatusSuccess
				result.HistoricalLinksTitlesExactlyMatching = fmt.Sprint(len(linkTitles))
			} else {
				historicalLinksMatching = false

				var partialTitleMismatches []TitleMismatch
				for _, mismatch := range exactTitleMismatches {
					titleEq := mismatch.Title.EqualizedValue
					gtTitleEq := mismatch.GTTitle.EqualizedValue
					if !strings.HasSuffix(titleEq, gtTitleEq) && !strings.HasPrefix(gtTitleEq, titleEq) {
						partialTitleMismatches = append(partialTitleMismatches, mismatch)
					}
				}

				if len(partialTitleMismatches) == 0 {
					result.HistoricalLinksTitlesPartiallyMatchingStatus = crawler.StatusSuccess
					result.HistoricalLinksTitlesPartiallyMatching = fmt.Sprint(len(linkTitles))
				} else {
					result.HistoricalLinksTitlesPartiallyMatchingStatus = crawler.StatusFailure
					result.HistoricalLinksTitlesPartiallyMatching =
						fmt.Sprintf("%d (%d)", len(gtTitles)-len(partialTitleMismatches), len(gtTitles))
					logger.Info("Partially mismatching titles (%d):", len(partialTitleMismatches))
					for _, mismatch := range partialTitleMismatches {
						logger.Info(
							"Partial %s != GT %q", mismatch.Title.PrintString(), mismatch.GTTitle.Value,
						)
					}
				}

				result.HistoricalLinksTitlesExactlyMatchingStatus = crawler.StatusFailure
				result.HistoricalLinksTitlesExactlyMatching =
					fmt.Sprintf("%d (%d)", len(gtTitles)-len(exactTitleMismatches), len(gtTitles))
				logger.Info("Exactly mismatching titles (%d):", len(exactTitleMismatches))
				for _, mismatch := range exactTitleMismatches {
					logger.Info(
						"Exact %s != GT %q", mismatch.Title.PrintString(), mismatch.GTTitle.Value,
					)
				}
			}
		} else {
			gtTitleEqValues := make(map[string]bool)
			for _, title := range gtTitles {
				gtTitleEqValues[title.EqualizedValue] = true
			}
			titlesMatchingCount := 0
			for _, title := range linkTitles {
				if gtTitleEqValues[title.EqualizedValue] {
					titlesMatchingCount++
				}
			}
			historicalLinksMatching = false
			result.HistoricalLinksTitlesPartiallyMatchingStatus = crawler.StatusFailure
			result.HistoricalLinksTitlesPartiallyMatching =
				fmt.Sprintf("%d (%d)", titlesMatchingCount, len(gtTitles))
			result.HistoricalLinksTitlesExactlyMatchingStatus = crawler.StatusFailure
			result.HistoricalLinksTitlesExactlyMatching =
				fmt.Sprintf("%d (%d)", titlesMatchingCount, len(gtTitles))
			logger.Info("Missing ground truth titles (%d):", len(linkTitles)-titlesMatchingCount)
			for _, title := range linkTitles {
				if !gtTitleEqValues[title.EqualizedValue] {
					logger.Info(title.Value)
				}
			}
		}

		result.HistoricalLinksMatching = fmt.Sprint(historicalLinksMatching)

		if hasGuidedSucceededBefore {
			if historicalLinksMatching {
				result.NoGuidedRegressionStatus = crawler.StatusSuccess
			} else {
				result.NoGuidedRegressionStatus = crawler.StatusFailure
			}
			if historicalLinksMatching {
				result.NoGuidedRegression = TernaryTrue
			} else {
				result.NoGuidedRegression = TernaryFalse
			}
		} else if historicalLinksMatching && saveSuccesses {
			_, err := conn.Exec(`
				insert into guided_successes (start_link_id, timestamp) values ($1, now())
			`, startLinkId)
			if err != nil {
				return &result, newError(err, result)
			}
			logger.Info("Saved guided success")
		}
	} else {
		result.BlogUrl = htmlLink(historicalResult.BlogLink)
		result.HistoricalLinksMatching = "?"
		result.HistoricalLinksMatchingStatus = crawler.StatusNeutral
		result.NoGuidedRegressionStatus = crawler.StatusNeutral
		result.HistoricalLinksPattern = historicalResult.Pattern
		result.HistoricalLinksCount = fmt.Sprint(entriesCount)
		result.HistoricalLinksTitlesPartiallyMatchingStatus = crawler.StatusNeutral
		result.HistoricalLinksTitlesExactlyMatchingStatus = crawler.StatusNeutral
		result.MainUrl = htmlLink(historicalResult.MainLink)
		result.OldestLink = htmlLink(oldestLink.Link)
	}

	result.Extra = historicalResult.Extra

	return &result, nil
}

func htmlLink(link crawler.Link) string {
	return fmt.Sprintf(`<a href="%s">%s</a>`, link.Url, link.Curi.String())
}

func htmlLinkMismatch(link crawler.Link, gtCuri string) string {
	return fmt.Sprintf(`<a href="%s">%s</a><br>(%s)`, link.Url, link.Curi.String(), gtCuri)
}
