package models

import (
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

type BlogPostId int64
type BlogPostCategoryId int64

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
			defer util.CommitOrRollbackErr(nestedTx, err)
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
			defer util.CommitOrRollbackErr(nestedTx, err)

			err = Blog_Downgrade(nestedTx, blog.Id, startFeed.Url)
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
				defer util.CommitOrRollbackErr(nestedTx, err)

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
	blogIdInt, err := mutil.GenerateRandomId(tx, "blogs")
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
	err = blogPostLock_Create(tx, blogId)
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

func blogPostLock_Create(tx pgw.Queryable, blogId BlogId) error {
	_, err := tx.Exec(`
		insert into blog_post_locks (blog_id) values ($1)
	`, blogId)
	return err
}

var ErrNoLatestVersion = errors.New("blog with latest version not found")

func Blog_Downgrade(tx pgw.Queryable, blogId BlogId, feedUrl string) error {
	row := tx.QueryRow(`
		update blogs
		set version = (select count(1) from blogs where feed_url = $1)
		where id = $2 and version = $3
		returning version
	`, feedUrl, blogId, BlogLatestVersion)
	var version int64
	err := row.Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNoLatestVersion
	} else if err != nil {
		return err
	}

	return nil
}

func Blog_GetNameStatusById(tx pgw.Queryable, blogId BlogId) (name string, status BlogStatus, err error) {
	row := tx.QueryRow("select name, status from blogs where id = $1", blogId)
	err = row.Scan(&name, &status)
	if err != nil {
		return "", "", err
	}

	return name, status, nil
}
