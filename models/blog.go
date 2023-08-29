package models

import (
	"encoding/binary"
	"errors"
	"feedrewind/crawler"
	"feedrewind/db/pgw"
	"feedrewind/log"
	"feedrewind/models/mutil"
	"feedrewind/oops"
	"feedrewind/util"
	"fmt"
	"net/url"

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
	Id           BlogId
	Name         string
	Status       BlogStatus
	UpdateAction BlogUpdateAction
	BestUrl      string
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

func Blog_GetLatestByFeedUrl(tx pgw.Queryable, feedUrl string) (*Blog, error) {
	row := tx.QueryRow(`
		select id, name, status, update_action, coalesce(url, feed_url) from blogs
		where feed_url = $1 and version = $2
	`, feedUrl, BlogLatestVersion)

	var b Blog
	err := row.Scan(&b.Id, &b.Name, &b.Status, &b.UpdateAction, &b.BestUrl)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrBlogNotFound
	} else if err != nil {
		return nil, err
	}

	return &b, nil
}

// Break circular dependency between models and jobs. Jobs will be using models, models only sometimes need to
// schedule a job.
type GuidedCrawlingJobScheduleFunc func(tx pgw.Queryable, blogId BlogId, startFeedId StartFeedId) error

func Blog_CreateOrUpdate(
	tx pgw.Queryable, startFeed *StartFeed, guidedCrawlingJobScheduleFunc GuidedCrawlingJobScheduleFunc,
) (*Blog, error) {
	blog, err := Blog_GetLatestByFeedUrl(tx, startFeed.Url)
	if errors.Is(err, ErrBlogNotFound) {
		log.Info().Msgf("Creating a new blog for feed url %s", startFeed.Url)
		blog, err = func() (blog *Blog, err error) {
			nestedTx, err := tx.Begin()
			if err != nil {
				return nil, err
			}
			defer util.CommitOrRollbackErr(nestedTx, &err)
			blog, err = blog_CreateWithCrawling(nestedTx, startFeed, guidedCrawlingJobScheduleFunc)
			if errors.Is(err, errBlogAlreadyExists) {
				// Another writer must've created the record at the same time, let's use that
				blog, err = Blog_GetLatestByFeedUrl(nestedTx, startFeed.Url)
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
	logger := &crawler.ZeroLogger{}
	rows, err := tx.Query(`select url from blog_posts where blog_id = $1 order by index desc`, blog.Id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var blogPostUrl string
		err := rows.Scan(&blogPostUrl)
		if err != nil {
			return nil, err
		}
		blogPostLink, ok := crawler.ToCanonicalLink(blogPostUrl, logger, nil)
		if !ok {
			return nil, oops.Newf("couldn't parse blog post url: %s", blogPostUrl)
		}
		blogPostCuris = append(blogPostCuris, blogPostLink.Curi)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	curiEqCfg, err := blogCanonicalEqualityConfig_Get(tx, blog.Id)
	if err != nil {
		return nil, err
	}
	discardedFeedEntryUrls, err := blogDiscardedFeedEntry_ListUrls(tx, blog.Id)
	if err != nil {
		return nil, err
	}
	missingFromFeedEntryUrls, err := blogMissingFromFeedEntry_ListUrls(tx, blog.Id)
	if err != nil {
		return nil, err
	}
	finalUri, err := url.Parse(startFeed.Url)
	if err != nil {
		return nil, oops.Wrap(err)
	}
	newLinks, err := crawler.ExtractNewPostsFromFeed(
		startFeed.MaybeParsedFeed, finalUri, blogPostCuris, discardedFeedEntryUrls, missingFromFeedEntryUrls,
		curiEqCfg, logger, logger,
	)
	newLinksOk := true
	if errors.Is(err, crawler.ErrExtractNewPostsNoMatch) {
		newLinksOk = false
	} else if err != nil {
		return nil, err
	}

	if newLinksOk && len(newLinks) == 0 {
		log.Info().Msgf("Blog %s doesn't need updating", startFeed.Url)
		return blog, nil
	}

	switch blog.UpdateAction {
	case BlogUpdateActionRecrawl:
		log.Info().Msgf("Blog %s is marked to recrawl on update", startFeed.Url)
		return func() (newBlog *Blog, err error) {
			nestedTx, err := tx.Begin()
			if err != nil {
				return nil, err
			}
			defer util.CommitOrRollbackErr(nestedTx, &err)

			err = Blog_Downgrade(nestedTx, blog.Id)
			if err != nil {
				newBlog, err = blog_CreateWithCrawling(nestedTx, startFeed, guidedCrawlingJobScheduleFunc)
			}
			if errors.Is(err, ErrNoLatestVersion) || errors.Is(err, errBlogAlreadyExists) {
				// Another writer deprecated this blog at the same time
				newBlog, err = Blog_GetLatestByFeedUrl(nestedTx, startFeed.Url)
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
			log.Info().Msgf("Updating blog %s from feed with %d new links", startFeed.Url, len(newLinks))
			row := tx.QueryRow(`
				select id from blog_post_categories
				where blog_id = $1 and name = 'Everything'
			`, blog.Id)
			var everythingId BlogPostCategoryId
			err := row.Scan(&everythingId)
			if err != nil {
				return nil, err
			}

			return func() (newBlog *Blog, err error) {
				nestedTx, err := tx.Begin()
				if err != nil {
					return nil, err
				}
				defer util.CommitOrRollbackErr(nestedTx, &err)

				rows, err = nestedTx.Query(`
					select blog_id from blog_post_locks
					where blog_id = $1
					for update nowait
				`, blog.Id)
				var pgErr *pgconn.PgError
				if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.LockNotAvailable {
					log.Info().Msgf("Someone else is updating the blog posts for %s, just waiting till they're done", startFeed.Url)
					rows, err := nestedTx.Query(`
						select blog_id from blog_post_locks
						where blog_id = $1
						for update
					`, blog.Id)
					if err != nil {
						return nil, err
					}
					rows.Close()
					log.Info().Msg("Done waiting")

					// Assume that the other writer has put the fresh posts in
					// There could be a race condition where two updates and a post publish happened at the
					// same time, the other writer succeeds with an older list, the current writer fails with
					// a newer list and ends up using the older list. But if both updates came a second
					// earlier, both would get the older list, and the new post would still get published a
					// second later, so the UX is the same and there's nothing we could do about it.
					newBlog, err = Blog_GetLatestByFeedUrl(nestedTx, startFeed.Url)
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

				row = nestedTx.QueryRow("select max(index) from blog_posts where blog_id = $1", blog.Id)
				var maxIndex int
				err = row.Scan(&maxIndex)
				if err != nil {
					return nil, err
				}
				batch := &pgx.Batch{} //nolint:exhaustruct
				for i := 0; i < len(newLinks); i++ {
					index := maxIndex + 1 + i
					link := newLinks[len(newLinks)-1-i]
					var title string
					if link.MaybeTitle != nil {
						title = link.MaybeTitle.Value
					} else {
						title = link.Url
					}
					batch.Queue(`
					with blog_post_ids as (
						insert into blog_posts (blog_id, index, url, title)
						values ($1, $2, $3, $4)
						returning id
					)
					insert into blog_post_category_assignments (blog_post_id, category_id)
					select id, $5 from blog_post_ids
				`, blog.Id, index, link.Url, title, everythingId)
				}
				err = nestedTx.SendBatch(batch).Close()
				if err != nil {
					return nil, err
				}
				return blog, nil
			}()
		} else {
			log.Warn().Msgf("Couldn't update blog %s from feed, marking as failed", startFeed.Url)
			_, err := tx.Exec(`
				update blogs set status = $1 where id = $2
			`, BlogStatusUpdateFromFeedFailed, blog.Id)
			if err != nil {
				return nil, err
			}
			blog.Status = BlogStatusUpdateFromFeedFailed
			return blog, nil
		}
	case BlogUpdateActionFail:
		log.Warn().Msgf("Blog %s is marked to fail on update", startFeed.Url)
		_, err := tx.Exec(`
			update blogs set status = $1 where id = $2
		`, BlogStatusUpdateFromFeedFailed, blog.Id)
		if err != nil {
			return nil, err
		}
		blog.Status = BlogStatusUpdateFromFeedFailed
		return blog, nil
	case BlogUpdateActionNoOp:
		log.Info().Msgf("Blog %s is marked to never update", startFeed.Url)
		return blog, nil
	default:
		panic(fmt.Errorf("unexpected blog update action: %s", blog.UpdateAction))
	}
}

var errBlogAlreadyExists = errors.New("blog already exists")

func blog_CreateWithCrawling(
	tx pgw.Queryable, startFeed *StartFeed,
	guidedCrawlingJobScheduleFunc GuidedCrawlingJobScheduleFunc,
) (*Blog, error) {
	blogIdInt, err := mutil.RandomId(tx, "blogs")
	if err != nil {
		return nil, err
	}
	blogId := BlogId(blogIdInt)
	status := BlogStatusCrawlInProgress
	updateAction := BlogUpdateActionRecrawl
	_, err = tx.Exec(`
		insert into blogs (id, name, feed_url, url, status, status_updated_at, version, update_action)
		values ($1, $2, $3, null, $4, utc_now(), $5, $6)
	`, blogId, startFeed.Title, startFeed.Url, status, BlogLatestVersion, updateAction)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation &&
		pgErr.ConstraintName == "index_blogs_on_feed_url_and_version" {
		return nil, errBlogAlreadyExists
	} else if err != nil {
		return nil, err
	}

	err = blogCrawlProgress_Create(tx, blogId)
	if err != nil {
		return nil, err
	}
	err = BlogPostLock_Create(tx, blogId)
	if err != nil {
		return nil, err
	}
	err = guidedCrawlingJobScheduleFunc(tx, blogId, startFeed.Id)
	if err != nil {
		return nil, err
	}

	return &Blog{
		Id:           blogId,
		Name:         startFeed.Title,
		Status:       status,
		UpdateAction: updateAction,
		BestUrl:      startFeed.Url,
	}, nil
}

func Blog_Create(
	tx pgw.Queryable, name string, feedUrl string, url string, status BlogStatus, version int,
	updateAction BlogUpdateAction,
) (BlogId, error) {
	blogIdInt, err := mutil.RandomId(tx, "blogs")
	if err != nil {
		return 0, err
	}
	blogId := BlogId(blogIdInt)
	_, err = tx.Exec(`
		insert into blogs(id, name, feed_url, url, status, status_updated_at, version, update_action)
		values ($1, $2, $3, $4, $5, utc_now(), $6, $7)
	`, blogId, name, feedUrl, url, status, version, updateAction)
	return blogId, err
}

var ErrNoLatestVersion = errors.New("blog with latest version not found")

func Blog_Downgrade(tx pgw.Queryable, blogId BlogId) error {
	row := tx.QueryRow(`
		update blogs
		set version = (
			select max(version) from blogs
			where feed_url = (select feed_url from blogs where id = $1) and version != $2
		) + 1
		where id = $1 and version = $2
		returning version
	`, blogId, BlogLatestVersion)
	var version int64
	err := row.Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNoLatestVersion
	} else if err != nil {
		return err
	}

	return nil
}

func Blog_GetNameStatus(tx pgw.Queryable, blogId BlogId) (name string, status BlogStatus, err error) {
	row := tx.QueryRow("select name, status from blogs where id = $1", blogId)
	err = row.Scan(&name, &status)
	if err != nil {
		return "", "", err
	}

	return name, status, nil
}

func Blog_GetStatus(tx pgw.Queryable, blogId BlogId) (BlogStatus, error) {
	row := tx.QueryRow("select status from blogs where id = $1", blogId)
	var status BlogStatus
	err := row.Scan(&status)
	if err != nil {
		return "", err
	}

	return status, nil
}

func Blog_GetCrawlProgressMap(tx pgw.Queryable, blogId BlogId) (map[string]any, error) {
	status, err := Blog_GetStatus(tx, blogId)
	if err != nil {
		return nil, err
	}
	switch status {
	case BlogStatusCrawlInProgress:
		blogCrawlProgress, err := BlogCrawlProgress_Get(tx, blogId)
		if err != nil {
			return nil, err
		}

		log.Info().Msgf("Blog %d crawl in progress (epoch %d)", blogId, blogCrawlProgress.Epoch)
		return map[string]any{
			"epoch":  blogCrawlProgress.Epoch,
			"status": blogCrawlProgress.Progress,
			"count":  blogCrawlProgress.Count,
		}, nil
	default:
		log.Info().Msgf("Blog %d crawl done", blogId)
		return map[string]any{
			"done": true,
		}, nil
	}

}

func Blog_GetBestUrl(tx pgw.Queryable, blogId BlogId) (string, error) {
	row := tx.QueryRow(`
		select coalesce(url, feed_url) from blogs
		where id = $1
	`, blogId)

	var result string
	err := row.Scan(&result)
	return result, err
}

type BlogWithCounts struct {
	Name            string
	FeedUrl         string
	PostsCount      int
	CategoriesCount int
}

func Blog_GetWithCounts(tx pgw.Queryable, blogId BlogId) (*BlogWithCounts, error) {
	row := tx.QueryRow(`
		select
			name,
			feed_url,
			(select count(1) from blog_posts where blog_id = $1),
			(select count(1) from blog_post_categories where blog_id = $1)
		from blogs
		where id = $1
	`, blogId)
	var b BlogWithCounts
	err := row.Scan(&b.Name, &b.FeedUrl, &b.PostsCount, &b.CategoriesCount)
	return &b, err
}

// BlogCanonicalEqualityConfig

func blogCanonicalEqualityConfig_Get(
	tx pgw.Queryable, blogId BlogId,
) (*crawler.CanonicalEqualityConfig, error) {
	row := tx.QueryRow(`
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

func BlogCanonicalEqualityConfig_Create(
	tx pgw.Queryable, blogId BlogId, sameHosts []string, expectTumblrPaths bool,
) error {
	_, err := tx.Exec(`
		insert into blog_canonical_equality_configs(blog_id, same_hosts, expect_tumblr_paths)
		values ($1, $2, $3)
	`, blogId, sameHosts, expectTumblrPaths)
	return err
}

// BlogDiscardedFeedEntry, BlogMissingFeedEntry

func blogDiscardedFeedEntry_ListUrls(tx pgw.Queryable, blogId BlogId) (map[string]bool, error) {
	return blogFeedEntry_ListUrls(tx, "blog_discarded_feed_entries", blogId)
}

func blogMissingFromFeedEntry_ListUrls(tx pgw.Queryable, blogId BlogId) (map[string]bool, error) {
	return blogFeedEntry_ListUrls(tx, "blog_missing_from_feed_entries", blogId)
}

func blogFeedEntry_ListUrls(tx pgw.Queryable, table string, blogId BlogId) (map[string]bool, error) {
	rows, err := tx.Query("select url from "+table+" where blog_id = $1", blogId)
	if err != nil {
		return nil, err
	}
	urls := make(map[string]bool)
	for rows.Next() {
		var url string
		err := rows.Scan(&url)
		if err != nil {
			return nil, err
		}
		urls[url] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return urls, nil
}

func BlogDiscardedFeedEntry_CreateMany(tx pgw.Queryable, blogId BlogId, urls []string) error {
	return blogFeedEntry_CreateMany(tx, "blog_discarded_feed_entries", blogId, urls)
}

func BlogMissingFromFeedEntry_CreateMany(tx pgw.Queryable, blogId BlogId, urls []string) error {
	return blogFeedEntry_CreateMany(tx, "blog_missing_from_feed_entries", blogId, urls)
}

func blogFeedEntry_CreateMany(tx pgw.Queryable, table string, blogId BlogId, urls []string) error {
	var batch pgx.Batch
	for _, url := range urls {
		batch.Queue(`
			insert into `+table+`(blog_id, url)
			values ($1, $2)
		`, blogId, url)
	}
	err := tx.SendBatch(&batch).Close()
	return err
}

// BlogPostLock

func BlogPostLock_Create(tx pgw.Queryable, blogId BlogId) error {
	_, err := tx.Exec(`
		insert into blog_post_locks (blog_id) values ($1)
	`, blogId)
	return err
}

// BlogCrawlProgress

type BlogCrawlProgress struct {
	Count    int32
	Progress string
	Epoch    int32
}

func BlogCrawlProgress_Get(tx pgw.Queryable, blogId BlogId) (*BlogCrawlProgress, error) {
	row := tx.QueryRow(`
		select count, progress, epoch from blog_crawl_progresses where blog_id = $1
	`, blogId)
	var countPtr *int32
	var progressPtr *string
	var epoch int32
	err := row.Scan(&countPtr, &progressPtr, &epoch)
	if err != nil {
		return nil, err
	}
	var count int32
	if countPtr != nil {
		count = *countPtr
	}
	var progress string
	if progressPtr != nil {
		progress = *progressPtr
	}

	return &BlogCrawlProgress{
		Count:    count,
		Progress: progress,
		Epoch:    epoch,
	}, nil
}

func blogCrawlProgress_Create(tx pgw.Queryable, blogId BlogId) error {
	_, err := tx.Exec(`
		insert into blog_crawl_progresses (blog_id, epoch) values ($1, 0)
	`, blogId)
	return err
}

// BlogCrawlClientToken

type BlogCrawlClientToken string

var ErrBlogCrawlClientTokenNotFound = errors.New("blog crawl client token not found")

func BlogCrawlClientToken_GetById(tx pgw.Queryable, blogId BlogId) (BlogCrawlClientToken, error) {
	row := tx.QueryRow("select value from blog_crawl_client_tokens where blog_id = $1", blogId)
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
func BlogCrawlClientToken_Create(tx pgw.Queryable, blogId BlogId) (BlogCrawlClientToken, error) {
	buf := make([]byte, 8)
	valueInt := binary.LittleEndian.Uint64(buf)
	value := BlogCrawlClientToken(fmt.Sprintf("%x", valueInt))
	_, err := tx.Exec(`
		insert into blog_crawl_client_tokens (blog_id, value) values ($1, $2)
	`, blogId, value)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation &&
		pgErr.ConstraintName == "blog_crawl_client_tokens_pkey" {
		return "", nil
	} else if err != nil {
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
	tx pgw.Queryable, blogId BlogId, userId UserId, value BlogCrawlVoteValue,
) error {
	var sqlUserId *UserId
	if userId != 0 {
		sqlUserId = &userId
	}
	_, err := tx.Exec(`
		insert into blog_crawl_votes (blog_id, user_id, value) values ($1, $2, $3)
	`, blogId, sqlUserId, value)
	return err
}

// BlogPost

type BlogPostId int64

type NewBlogPost struct {
	Index int32
	Url   string
	Title string
}

type CreatedBlogPost struct {
	Id  BlogPostId
	Url string
}

func BlogPost_CreateMany(tx pgw.Queryable, blogId BlogId, posts []NewBlogPost) ([]CreatedBlogPost, error) {
	var batch pgx.Batch
	var createdPosts []CreatedBlogPost
	for _, post := range posts {
		batch.
			Queue(`
				insert into blog_posts(blog_id, index, url, title)
				values ($1, $2, $3, $4)
				returning id, url
			`, blogId, post.Index, post.Url, post.Title).
			QueryRow(func(row pgx.Row) error {
				var p CreatedBlogPost
				err := row.Scan(&p.Id, &p.Url)
				if err != nil {
					return err
				}

				createdPosts = append(createdPosts, p)
				return nil
			})
	}
	err := tx.SendBatch(&batch).Close()
	return createdPosts, err
}

type BlogPost struct {
	Id    BlogPostId
	Index int32
	Url   string
	Title string
}

func BlogPost_List(tx pgw.Queryable, blogId BlogId) ([]BlogPost, error) {
	rows, err := tx.Query(`
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

type NewBlogPostCategory struct {
	Name  string
	Index int32
	IsTop bool
}

type CreatedBlogPostCategory struct {
	Id   BlogPostCategoryId
	Name string
}

func BlogPostCategory_CreateMany(
	tx pgw.Queryable, blogId BlogId, categories []NewBlogPostCategory,
) ([]CreatedBlogPostCategory, error) {
	var batch pgx.Batch
	var createdCategories []CreatedBlogPostCategory
	for _, category := range categories {
		batch.
			Queue(`
				insert into blog_post_categories(blog_id, name, index, is_top)
				values ($1, $2, $3, $4)
				returning id, name
			`, blogId, category.Name, category.Index, category.IsTop).
			QueryRow(func(row pgx.Row) error {
				var p CreatedBlogPostCategory
				err := row.Scan(&p.Id, &p.Name)
				if err != nil {
					return err
				}

				createdCategories = append(createdCategories, p)
				return nil
			})
	}
	err := tx.SendBatch(&batch).Close()
	return createdCategories, err
}

type BlogPostCategory struct {
	Id          BlogPostCategoryId
	Name        string
	IsTop       bool
	BlogPostIds map[BlogPostId]bool
}

func BlogPostCategory_ListOrdered(tx pgw.Queryable, blogId BlogId) ([]BlogPostCategory, error) {
	rows, err := tx.Query(`
		select id, name, is_top from blog_post_categories where blog_id = $1 order by index asc
	`, blogId)
	if err != nil {
		return nil, err
	}

	var categories []BlogPostCategory
	for rows.Next() {
		var category BlogPostCategory
		err := rows.Scan(&category.Id, &category.Name, &category.IsTop)
		if err != nil {
			return nil, err
		}

		categories = append(categories, category)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = tx.Query(`
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
	tx pgw.Queryable, categoryId BlogPostCategoryId,
) (name string, postsCount int, err error) {
	row := tx.QueryRow(`
		select
			name,
			(select count(1) from blog_post_category_assignments where category_id = blog_post_categories.id)
		from blog_post_categories
		where id = $1
	`, categoryId)
	err = row.Scan(&name, &postsCount)
	return name, postsCount, err
}

// BlogPostCategoryAssignment

type NewBlogPostCategoryAssignment struct {
	BlogPostId BlogPostId
	CategoryId BlogPostCategoryId
}

func BlogPostCategoryAssignment_CreateMany(tx pgw.Queryable, assignments []NewBlogPostCategoryAssignment) error {
	var batch pgx.Batch
	for _, assignment := range assignments {
		batch.Queue(`
			insert into blog_post_category_assignments(blog_post_id, category_id)
			values ($1, $2)
		`, assignment.BlogPostId, assignment.CategoryId)
	}
	err := tx.SendBatch(&batch).Close()
	return err
}
