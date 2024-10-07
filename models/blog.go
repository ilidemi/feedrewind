package models

import (
	"errors"
	"feedrewind/crawler"
	"feedrewind/db/pgw"
	"feedrewind/models/mutil"
	"feedrewind/oops"
	"feedrewind/util"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type BlogId int64

type BlogStatus string

const (
	BlogStatusCrawlInProgress      BlogStatus = "crawl_in_progress"
	BlogStatusCrawlFailed          BlogStatus = "crawl_failed"
	BlogStatusCrawledVoting        BlogStatus = "crawled_voting"
	BlogStatusCrawledConfirmed     BlogStatus = "crawled_confirmed"
	BlogStatusCrawledLooksWrong    BlogStatus = "crawled_looks_wrong"
	BlogStatusManuallyInserted     BlogStatus = "manually_inserted"
	BlogStatusUpdateFromFeedFailed BlogStatus = "update_from_feed_failed"
)

var BlogFailedAutoStatuses = map[BlogStatus]bool{
	BlogStatusCrawlFailed:       true,
	BlogStatusCrawledLooksWrong: true,
}

var BlogFailedStatuses = map[BlogStatus]bool{
	BlogStatusCrawlFailed:          true,
	BlogStatusCrawledLooksWrong:    true,
	BlogStatusUpdateFromFeedFailed: true,
}

var BlogCrawledStatuses = map[BlogStatus]bool{
	BlogStatusCrawledVoting:    true,
	BlogStatusCrawledConfirmed: true,
	BlogStatusManuallyInserted: true,
}

type BlogUpdateAction string

const (
	BlogUpdateActionRecrawl              BlogUpdateAction = "recrawl"
	BlogUpdateActionUpdateFromFeedOrFail BlogUpdateAction = "update_from_feed_or_fail"
	BlogUpdateActionNoOp                 BlogUpdateAction = "no_op"
	BlogUpdateActionFail                 BlogUpdateAction = "fail"
)

type Blog struct {
	Id               BlogId
	Name             string
	Status           BlogStatus
	UpdateAction     BlogUpdateAction
	MaybeStartFeedId *StartFeedId
	BestUrl          string
}

const BlogLatestVersion = 1000000

// Invariants for a given feed_url:
// If there is a good blog to use, it is always available at BlogLatestVersion
// N blogs that are not BlogLatestVersion have versions 1..N
// It is possible to not have a blog BlogLatestVersion if version N is
// crawl_failed/update_from_feed_failed/crawled_looks_wrong
//
// Invariant for a given feed_url + version:
// Either status is crawl_in_progress/crawl_failed/crawled_looks_wrong or blog posts are filled out

var ErrBlogNotFound = errors.New("blog not found")

func Blog_GetLatestByFeedUrl(qu pgw.Queryable, feedUrl string) (*Blog, error) {
	row := qu.QueryRow(`
		select id, name, status, update_action, start_feed_id, coalesce(url, feed_url) from blogs
		where feed_url = $1 and version = $2
	`, feedUrl, BlogLatestVersion)

	var b Blog
	err := row.Scan(&b.Id, &b.Name, &b.Status, &b.UpdateAction, &b.MaybeStartFeedId, &b.BestUrl)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrBlogNotFound
	} else if err != nil {
		return nil, err
	}

	return &b, nil
}

// Break circular dependency between models and jobs. Jobs will be using models, models only sometimes need to
// schedule a job.
type GuidedCrawlingJobScheduleFunc func(qu pgw.Queryable, blogId BlogId, startFeedId StartFeedId) error

func Blog_CreateOrUpdate(
	qu pgw.Queryable, startFeed *StartFeed, guidedCrawlingJobScheduleFunc GuidedCrawlingJobScheduleFunc,
) (*Blog, error) {
	logger := qu.Logger()
	blog, err := Blog_GetLatestByFeedUrl(qu, startFeed.Url)
	if errors.Is(err, ErrBlogNotFound) {
		logger.Info().Msgf("Creating a new blog for feed url %s", startFeed.Url)
		blog, err = func() (blog *Blog, err error) {
			tx, err := qu.Begin()
			if err != nil {
				return nil, err
			}
			defer util.CommitOrRollbackErr(tx, &err)
			blog, err = blog_CreateWithCrawling(tx, startFeed, guidedCrawlingJobScheduleFunc)
			if errors.Is(err, errBlogAlreadyExists) {
				// Another writer must've created the record at the same time, let's use that
				blog, err = Blog_GetLatestByFeedUrl(tx, startFeed.Url)
				if err != nil {
					return nil, oops.Wrapf(
						err,
						"Blog %s with latest version didn't exist, then existed, now doesn't exist",
						startFeed.Url,
					)
				}
			} else if err != nil {
				return nil, err
			}
			return blog, nil
		}()
		if err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}

	// A blog that is currently being crawled will come out fresh
	// A blog that failed crawl can't be fixed by updating
	// But a blog that failed update from feed can be retried, who knows
	if blog.Status == BlogStatusCrawlInProgress ||
		blog.Status == BlogStatusCrawlFailed ||
		blog.Status == BlogStatusCrawledLooksWrong {
		return blog, nil
	}

	// Update blog from feed
	var blogPostCuris []crawler.CanonicalUri
	zlogger := crawler.ZeroLogger{Logger: logger, MaybeLogScreenshotFunc: nil}
	rows, err := qu.Query(`select url from blog_posts where blog_id = $1 order by index desc`, blog.Id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var blogPostUrl string
		err := rows.Scan(&blogPostUrl)
		if err != nil {
			return nil, err
		}
		blogPostLink, ok := crawler.ToCanonicalLink(blogPostUrl, &zlogger, nil)
		if !ok {
			return nil, oops.Newf("couldn't parse blog post url: %s", blogPostUrl)
		}
		blogPostCuris = append(blogPostCuris, blogPostLink.Curi)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	curiEqCfg, err := BlogCanonicalEqualityConfig_Get(qu, blog.Id)
	if err != nil {
		return nil, err
	}

	rows, err = qu.Query(`select url from blog_discarded_feed_entries where blog_id = $1`, blog.Id)
	if err != nil {
		return nil, err
	}
	discardedFeedEntryUrls := make(map[string]bool)
	for rows.Next() {
		var url string
		err := rows.Scan(&url)
		if err != nil {
			return nil, err
		}
		discardedFeedEntryUrls[url] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = qu.Query(`select url from blog_missing_from_feed_entries where blog_id = $1`, blog.Id)
	if err != nil {
		return nil, err
	}
	missingFromFeedEntryUrls := make(map[string]bool)
	for rows.Next() {
		var url string
		err := rows.Scan(&url)
		if err != nil {
			return nil, err
		}
		missingFromFeedEntryUrls[url] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	finalUri, err := url.Parse(startFeed.Url)
	if err != nil {
		return nil, oops.Wrap(err)
	}
	newLinks, err := crawler.ExtractNewPostsFromFeed(
		startFeed.MaybeParsedFeed, finalUri, blogPostCuris, discardedFeedEntryUrls, missingFromFeedEntryUrls,
		curiEqCfg, &zlogger, &zlogger,
	)
	newLinksOk := true
	if errors.Is(err, crawler.ErrExtractNewPostsNoMatch) {
		newLinksOk = false
	} else if err != nil {
		return nil, err
	}

	if newLinksOk && len(newLinks) == 0 {
		logger.Info().Msgf("Blog %s doesn't need updating", startFeed.Url)
		return blog, nil
	}

	switch blog.UpdateAction {
	case BlogUpdateActionRecrawl:
		logger.Info().Msgf("Blog %s is marked to recrawl on update", startFeed.Url)
		return func() (newBlog *Blog, err error) {
			tx, err := qu.Begin()
			if err != nil {
				return nil, err
			}
			defer util.CommitOrRollbackErr(tx, &err)

			_, err = Blog_Downgrade(tx, blog.Id)
			if err == nil {
				newBlog, err = blog_CreateWithCrawling(tx, startFeed, guidedCrawlingJobScheduleFunc)
				if err != nil {
					return nil, err
				}
			} else if errors.Is(err, ErrNoLatestVersion) || errors.Is(err, errBlogAlreadyExists) {
				// Another writer deprecated this blog at the same time
				newBlog, err = Blog_GetLatestByFeedUrl(tx, startFeed.Url)
				if errors.Is(err, ErrBlogNotFound) {
					return nil, oops.Newf(
						"Blog %s with latest version was deprecated by another request but the latest version still doesn't exist",
						startFeed.Url,
					)
				} else if err != nil {
					return nil, err
				}
			} else if err != nil {
				return nil, err
			}

			return newBlog, nil
		}()
	case BlogUpdateActionUpdateFromFeedOrFail:
		if newLinksOk {
			logger.Info().Msgf("Updating blog %s from feed with %d new links", startFeed.Url, len(newLinks))
			rows, err := qu.Query(`select id, name from blog_post_categories where blog_id = $1`, blog.Id)
			if err != nil {
				return nil, err
			}
			categoryIdByName := map[string]BlogPostCategoryId{}
			for rows.Next() {
				var categoryId BlogPostCategoryId
				var name string
				err := rows.Scan(&categoryId, &name)
				if err != nil {
					return nil, err
				}
				categoryIdByName[name] = categoryId
			}
			if err := rows.Err(); err != nil {
				return nil, err
			}
			everythingId := categoryIdByName["Everything"]

			return func() (newBlog *Blog, err error) {
				tx, err := qu.Begin()
				if err != nil {
					return nil, err
				}
				defer util.CommitOrRollbackErr(tx, &err)

				rows, err = tx.Query(`
					select blog_id from blog_post_locks
					where blog_id = $1
					for update nowait
				`, blog.Id)
				var pgErr *pgconn.PgError
				if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.LockNotAvailable {
					logger.Info().Msgf("Someone else is updating the blog posts for %s, just waiting till they're done", startFeed.Url)
					rows, err := tx.Query(`
						select blog_id from blog_post_locks
						where blog_id = $1
						for update
					`, blog.Id)
					if err != nil {
						return nil, err
					}
					rows.Close()
					logger.Info().Msg("Done waiting")

					// Assume that the other writer has put the fresh posts in
					// There could be a race condition where two updates and a post publish happened at the
					// same time, the other writer succeeds with an older list, the current writer fails with
					// a newer list and ends up using the older list. But if both updates came a second
					// earlier, both would get the older list, and the new post would still get published a
					// second later, so the UX is the same and there's nothing we could do about it.
					newBlog, err = Blog_GetLatestByFeedUrl(tx, startFeed.Url)
					if errors.Is(err, ErrBlogNotFound) {
						return nil, oops.Newf(
							"Blog %s with latest version was updated by another request but the latest version also doesn't exist",
							startFeed.Url,
						)
					} else if err != nil {
						return nil, err
					}
					return newBlog, nil
				} else if err != nil {
					return nil, err
				}
				rows.Close()

				feedLink, _ := crawler.ToCanonicalLink(startFeed.Url, &zlogger, nil)
				isACX := crawler.CanonicalUriEqual(
					feedLink.Curi, crawler.HardcodedAstralCodexTenFeed, curiEqCfg,
				)
				isDontWorryAboutTheVase := crawler.CanonicalUriEqual(
					feedLink.Curi, crawler.HardcodedDontWorryAboutTheVaseFeed, curiEqCfg,
				)
				isOvercomingBias := crawler.CanonicalUriEqual(
					feedLink.Curi, crawler.HardcodedOvercomingBiasFeed, curiEqCfg,
				)

				row := tx.QueryRow("select max(index) from blog_posts where blog_id = $1", blog.Id)
				var maxIndex int
				err = row.Scan(&maxIndex)
				if err != nil {
					return nil, err
				}
				batch := tx.NewBatch()
				for i := range len(newLinks) {
					index := maxIndex + 1 + i
					link := newLinks[len(newLinks)-1-i]
					var title string
					if link.MaybeTitle != nil {
						title = link.MaybeTitle.Value
					} else {
						title = link.Url
					}
					var categoryNames []string
					switch {
					case isACX:
						categoryNames = crawler.ExtractACXCategories(link, logger)
					case isDontWorryAboutTheVase:
						categoryNames = crawler.ExtractDontWorryAboutTheVaseCategories(link, logger)
					case isOvercomingBias:
						categoryNames = crawler.ExtractOvercomingBiasCategories(link, logger)
					}
					categoryIds := []BlogPostCategoryId{everythingId}
					for _, categoryName := range categoryNames {
						categoryId, ok := categoryIdByName[categoryName]
						if !ok {
							row := tx.QueryRow(`
								insert into blog_post_categories (blog_id, name, index, top_status)
								values (
									$1,
									$2,
									(select max(index) from blog_post_categories where blog_id = $1) + 1,
									'custom_only'
								)
								returning id
							`, blog.Id, categoryName)
							err := row.Scan(&categoryId)
							if err != nil {
								return nil, err
							}
							categoryIdByName[categoryName] = categoryId
						}
						categoryIds = append(categoryIds, categoryIdByName[categoryName])
					}
					var sb strings.Builder
					for i, categoryId := range categoryIds {
						if i > 0 {
							fmt.Fprint(&sb, "), (")
						}
						fmt.Fprint(&sb, categoryId)
					}
					batch.Queue(`
						with blog_post_ids as (
							insert into blog_posts (blog_id, index, url, title)
							values ($1, $2, $3, $4)
							returning id
						),
						category_ids(id) as (values(`+sb.String()+`))
						insert into blog_post_category_assignments (blog_post_id, category_id)
						select (select id from blog_post_ids), id from category_ids
					`, blog.Id, index, link.Url, title)
				}
				err = tx.SendBatch(batch).Close()
				if err != nil {
					return nil, err
				}
				_, err = tx.Exec(`
					update blogs set start_feed_id = $1 where id = $2
				`, startFeed.Id, blog.Id)
				if err != nil {
					return nil, err
				}
				return blog, nil
			}()
		} else {
			logger.Warn().Msgf("Couldn't update blog %s from feed, marking as failed", startFeed.Url)
			_, err := qu.Exec(`
				update blogs set status = $1 where id = $2
			`, BlogStatusUpdateFromFeedFailed, blog.Id)
			if err != nil {
				return nil, err
			}
			blog.Status = BlogStatusUpdateFromFeedFailed
			return blog, nil
		}
	case BlogUpdateActionFail:
		logger.Warn().Msgf("Blog %s is marked to fail on update", startFeed.Url)
		_, err := qu.Exec(`
			update blogs set status = $1 where id = $2
		`, BlogStatusUpdateFromFeedFailed, blog.Id)
		if err != nil {
			return nil, err
		}
		blog.Status = BlogStatusUpdateFromFeedFailed
		return blog, nil
	case BlogUpdateActionNoOp:
		logger.Info().Msgf("Blog %s is marked to never update", startFeed.Url)
		return blog, nil
	default:
		panic(fmt.Errorf("unexpected blog update action: %s", blog.UpdateAction))
	}
}

var errBlogAlreadyExists = errors.New("blog already exists")

func blog_CreateWithCrawling(
	qu pgw.Queryable, startFeed *StartFeed,
	guidedCrawlingJobScheduleFunc GuidedCrawlingJobScheduleFunc,
) (*Blog, error) {
	row := qu.QueryRow(`
		select count(1) from blogs where feed_url = $1 and version = $2
	`, startFeed.Url, BlogLatestVersion)
	var count int
	err := row.Scan(&count)
	if err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, errBlogAlreadyExists
	}

	blogIdInt, err := mutil.RandomId(qu, "blogs")
	if err != nil {
		return nil, err
	}
	blogId := BlogId(blogIdInt)
	status := BlogStatusCrawlInProgress
	updateAction := BlogUpdateActionRecrawl
	_, err = qu.Exec(`
		insert into blogs (
			id, name, feed_url, url, status, status_updated_at, version, update_action, start_feed_id
		)
		values ($1, $2, $3, null, $4, utc_now(), $5, $6, $7)
	`, blogId, startFeed.Title, startFeed.Url, status, BlogLatestVersion, updateAction, startFeed.Id)
	if err != nil {
		return nil, err
	}

	_, err = qu.Exec(`insert into blog_crawl_progresses (blog_id, epoch) values ($1, 0)`, blogId)
	if err != nil {
		return nil, err
	}
	_, err = qu.Exec(`insert into blog_post_locks (blog_id) values ($1)`, blogId)
	if err != nil {
		return nil, err
	}
	err = guidedCrawlingJobScheduleFunc(qu, blogId, startFeed.Id)
	if err != nil {
		return nil, err
	}

	startFeedId := startFeed.Id
	return &Blog{
		Id:               blogId,
		Name:             startFeed.Title,
		Status:           status,
		UpdateAction:     updateAction,
		MaybeStartFeedId: &startFeedId,
		BestUrl:          startFeed.Url,
	}, nil
}

var ErrNoLatestVersion = errors.New("blog with latest version not found")

func Blog_Downgrade(qu pgw.Queryable, blogId BlogId) (int32, error) {
	row := qu.QueryRow(`
		update blogs
		set version = (
			select coalesce(max(version), 0) from blogs
			where feed_url = (select feed_url from blogs where id = $1) and version != $2
		) + 1,
			status_updated_at = utc_now()
		where id = $1 and version = $2
		returning version
	`, blogId, BlogLatestVersion)
	var version int32
	err := row.Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNoLatestVersion
	} else if err != nil {
		return 0, err
	}

	return version, nil
}

type CrawledBlogPost struct {
	Url        string
	Title      string
	Categories []string
}

func Blog_InitCrawled(
	tx *pgw.Tx, blogId BlogId, url string, crawledBlogPosts []CrawledBlogPost,
	categories []NewBlogPostCategory, discardedFeedUrls []string, curiEqCfg *crawler.CanonicalEqualityConfig,
) (updatedAt time.Time, err error) {
	row := tx.QueryRow(`select status from blogs where id = $1`, blogId)
	var status BlogStatus
	err = row.Scan(&status)
	if err != nil {
		return updatedAt, err
	}
	if status != BlogStatusCrawlInProgress {
		return updatedAt, oops.Newf(
			"Can only init posts when status is %s, got %s instead", BlogStatusCrawlInProgress, status,
		)
	}

	batch := tx.NewBatch()
	blogPostIds := make([]BlogPostId, len(crawledBlogPosts))
	for i, crawledBlogPost := range crawledBlogPosts {
		batch.Queue(`
			insert into blog_posts (blog_id, index, url, title)
			values ($1, $2, $3, $4)
			returning id
		`, blogId, len(crawledBlogPosts)-i-1, crawledBlogPost.Url, crawledBlogPost.Title,
		).QueryRow(func(row pgw.Row) error {
			return row.Scan(&blogPostIds[i])
		})
	}
	err = tx.SendBatch(batch).Close()
	if err != nil {
		return updatedAt, err
	}

	if len(categories) > 0 {
		batch := tx.NewBatch()
		categoryIds := make([]BlogPostCategoryId, len(categories))
		for i, category := range categories {
			batch.Queue(`
				insert into blog_post_categories (blog_id, name, index, top_status)
				values ($1, $2, $3, $4)
				returning id
			`, blogId, category.Name, category.Index, category.TopStatus,
			).QueryRow(func(row pgw.Row) error {
				return row.Scan(&categoryIds[i])
			})
		}
		err := tx.SendBatch(batch).Close()
		if err != nil {
			return updatedAt, err
		}

		categoryIdsByName := make(map[string]BlogPostCategoryId)
		for i, category := range categories {
			categoryIdsByName[category.Name] = categoryIds[i]
		}

		batch = tx.NewBatch()
		for i, crawledBlogPost := range crawledBlogPosts {
			for _, categoryName := range crawledBlogPost.Categories {
				batch.Queue(`
					insert into blog_post_category_assignments (blog_post_id, category_id)
					values ($1, $2)
				`, blogPostIds[i], categoryIdsByName[categoryName])
			}
		}
		err = tx.SendBatch(batch).Close()
		if err != nil {
			return updatedAt, err
		}
	}

	sameHosts := util.Keys(curiEqCfg.SameHosts)
	_, err = tx.Exec(`
		insert into blog_canonical_equality_configs (blog_id, same_hosts, expect_tumblr_paths)
		values ($1, $2, $3)
	`, blogId, sameHosts, curiEqCfg.ExpectTumblrPaths)
	if err != nil {
		return updatedAt, err
	}

	batch = tx.NewBatch()
	for _, discardedFeedUrl := range discardedFeedUrls {
		batch.Queue(`
			insert into blog_discarded_feed_entries (blog_id, url)
			values ($1, $2)
		`, blogId, discardedFeedUrl)
	}
	err = tx.SendBatch(batch).Close()
	if err != nil {
		return updatedAt, err
	}

	_, err = tx.Exec(`
		update blogs set url = $1, status = $2 where id = $3
	`, url, BlogStatusCrawledVoting, blogId)
	if err != nil {
		return updatedAt, err
	}

	var sb strings.Builder
	sb.WriteString("('")
	isFirst := true
	for status := range BlogCrawledStatuses {
		if !isFirst {
			sb.WriteString("','")
		}
		isFirst = false
		sb.WriteString(string(status))
	}
	sb.WriteString("')")
	row = tx.QueryRow(`
		select count(1) from blog_posts
		where blog_id = (
			select id from blogs
			where feed_url = (select feed_url from blogs where id = $1) and
				version != $2 and
				status in `+sb.String()+`
			order by version desc
			limit 1
		)
	`, blogId, BlogLatestVersion)
	var prevPostsCount int
	err = row.Scan(&prevPostsCount)
	if err != nil {
		return updatedAt, err
	}
	if len(crawledBlogPosts) < prevPostsCount {
		tx.Logger().Warn().Msgf(
			"Blog %d has fewer posts after recrawling: %d -> %d",
			blogId, prevPostsCount, len(crawledBlogPosts),
		)
	}

	row = tx.QueryRow(`select updated_at from blogs where id = $1`, blogId)
	err = row.Scan(&updatedAt)
	if err != nil {
		return updatedAt, err
	}

	return updatedAt, nil
}

func Blog_GetBestUrl(qu pgw.Queryable, blogId BlogId) (string, error) {
	row := qu.QueryRow(`
		select coalesce(url, feed_url) from blogs
		where id = $1
	`, blogId)

	var result string
	err := row.Scan(&result)
	return result, err
}

// BlogPostLock

func BlogPostLock_Create(qu pgw.Queryable, blogId BlogId) error {
	_, err := qu.Exec(`insert into blog_post_locks (blog_id) values ($1)`, blogId)
	return err
}

// BlogCrawlProgress

type BlogCrawlProgress struct {
	Count    int32
	Progress string
	Epoch    int32
}

func BlogCrawlProgress_Get(qu pgw.Queryable, blogId BlogId) (*BlogCrawlProgress, error) {
	row := qu.QueryRow(`
		select count, progress, epoch from blog_crawl_progresses where blog_id = $1
	`, blogId)
	var maybeCount *int32
	var maybeProgress *string
	var epoch int32
	err := row.Scan(&maybeCount, &maybeProgress, &epoch)
	if err != nil {
		return nil, err
	}
	var count int32
	if maybeCount != nil {
		count = *maybeCount
	}
	var progress string
	if maybeProgress != nil {
		progress = *maybeProgress
	}

	return &BlogCrawlProgress{
		Count:    count,
		Progress: progress,
		Epoch:    epoch,
	}, nil
}

// BlogCrawlClientToken

type BlogCrawlClientToken string

var ErrBlogCrawlClientTokenNotFound = errors.New("blog crawl client token not found")

func BlogCrawlClientToken_GetById(qu pgw.Queryable, blogId BlogId) (BlogCrawlClientToken, error) {
	row := qu.QueryRow(`select value from blog_crawl_client_tokens where blog_id = $1`, blogId)
	var result BlogCrawlClientToken
	err := row.Scan(&result)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrBlogCrawlClientTokenNotFound
	} else if err != nil {
		return "", err
	}

	return result, nil
}

// Returns zero value on conflict
func BlogCrawlClientToken_Create(qu pgw.Queryable, blogId BlogId) (BlogCrawlClientToken, error) {
	valueInt, err := util.RandomInt63()
	if err != nil {
		return "", err
	}
	value := BlogCrawlClientToken(fmt.Sprintf("%x", valueInt))
	row := qu.QueryRow(`select count(1) from blog_crawl_client_tokens where value = $1`, value)
	var count int
	err = row.Scan(&count)
	if err != nil {
		return "", err
	}
	if count > 0 {
		return "", nil
	}

	_, err = qu.Exec(`
		insert into blog_crawl_client_tokens (blog_id, value) values ($1, $2)
	`, blogId, value)
	if err != nil {
		return "", err
	}

	return value, nil
}

// BlogCrawlVote

type BlogCrawlVoteValue string

const (
	BlogCrawlVoteLooksWrong BlogCrawlVoteValue = "looks_wrong"
	BlogCrawlVoteConfirmed  BlogCrawlVoteValue = "confirmed"
)

func BlogCrawlVote_Create(
	qu pgw.Queryable, blogId BlogId, userId UserId, value BlogCrawlVoteValue,
) error {
	var sqlUserId *UserId
	if userId != 0 {
		sqlUserId = &userId
	}
	_, err := qu.Exec(`
		insert into blog_crawl_votes (blog_id, user_id, value) values ($1, $2, $3)
	`, blogId, sqlUserId, value)
	return err
}

// BlogPost

type BlogPostId int64

type BlogPost struct {
	Id    BlogPostId
	Index int32
	Url   string
	Title string
}

func BlogPost_List(qu pgw.Queryable, blogId BlogId) ([]BlogPost, error) {
	rows, err := qu.Query(`
		select id, index, url, title from blog_posts where blog_id = $1
	`, blogId)
	if err != nil {
		return nil, err
	}

	var result []BlogPost
	for rows.Next() {
		var p BlogPost
		err := rows.Scan(&p.Id, &p.Index, &p.Url, &p.Title)
		if err != nil {
			return nil, err
		}

		result = append(result, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

// BlogPostCategory

type BlogPostCategoryId int64

type BlogPostCategoryTopStatus string

const (
	BlogPostCategoryTopOnly      BlogPostCategoryTopStatus = "top_only"
	BlogPostCategoryTopAndCustom BlogPostCategoryTopStatus = "top_and_custom"
	BlogPostCategoryCustomOnly   BlogPostCategoryTopStatus = "custom_only"
)

func (s BlogPostCategoryTopStatus) IsTop() bool {
	return s == BlogPostCategoryTopOnly || s == BlogPostCategoryTopAndCustom
}

func (s BlogPostCategoryTopStatus) IsList() bool {
	return s == BlogPostCategoryTopAndCustom || s == BlogPostCategoryCustomOnly
}

type NewBlogPostCategory struct {
	Name      string
	Index     int32
	TopStatus BlogPostCategoryTopStatus
}

type BlogPostCategory struct {
	Id          BlogPostCategoryId
	Name        string
	TopStatus   BlogPostCategoryTopStatus
	BlogPostIds map[BlogPostId]bool
}

func BlogPostCategory_ListOrdered(qu pgw.Queryable, blogId BlogId) ([]BlogPostCategory, error) {
	rows, err := qu.Query(`
		select id, name, top_status from blog_post_categories where blog_id = $1 order by index asc
	`, blogId)
	if err != nil {
		return nil, err
	}

	var categories []BlogPostCategory
	for rows.Next() {
		var category BlogPostCategory
		err := rows.Scan(&category.Id, &category.Name, &category.TopStatus)
		if err != nil {
			return nil, err
		}

		categories = append(categories, category)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = qu.Query(`
		select category_id, blog_post_id from blog_post_category_assignments
		where category_id in (select id from blog_post_categories where blog_id = $1)
	`, blogId)
	if err != nil {
		return nil, err
	}

	blogPostIdsByCategoryId := make(map[BlogPostCategoryId][]BlogPostId)
	for rows.Next() {
		var categoryId BlogPostCategoryId
		var postId BlogPostId
		err := rows.Scan(&categoryId, &postId)
		if err != nil {
			return nil, err
		}
		blogPostIdsByCategoryId[categoryId] = append(blogPostIdsByCategoryId[categoryId], postId)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range categories {
		category := &categories[i]
		category.BlogPostIds = make(map[BlogPostId]bool)
		for _, blogPostId := range blogPostIdsByCategoryId[category.Id] {
			category.BlogPostIds[blogPostId] = true
		}
	}

	return categories, nil
}

func BlogPostCategory_GetNamePostsCountById(
	qu pgw.Queryable, categoryId BlogPostCategoryId,
) (name string, postsCount int, err error) {
	row := qu.QueryRow(`
		select
			name,
			(select count(1) from blog_post_category_assignments where category_id = blog_post_categories.id)
		from blog_post_categories
		where id = $1
	`, categoryId)
	err = row.Scan(&name, &postsCount)
	return name, postsCount, err
}

// BlogCanonicalEqualityConfig

func BlogCanonicalEqualityConfig_Get(
	qu pgw.Queryable, blogId BlogId,
) (*crawler.CanonicalEqualityConfig, error) {
	row := qu.QueryRow(`
		select same_hosts, expect_tumblr_paths from blog_canonical_equality_configs
		where blog_id = $1
	`, blogId)
	var sameHostsSlice []string
	var expectTumblrPaths bool
	err := row.Scan(&sameHostsSlice, &expectTumblrPaths)
	if err != nil {
		return nil, err
	}
	sameHosts := make(map[string]bool)
	for _, sameHost := range sameHostsSlice {
		sameHosts[sameHost] = true
	}
	return &crawler.CanonicalEqualityConfig{
		SameHosts:         sameHosts,
		ExpectTumblrPaths: expectTumblrPaths,
	}, nil
}
