package models

import (
	"errors"
	"feedrewind/crawler"
	"feedrewind/db/pgw"
	"feedrewind/log"
	"feedrewind/models/mutil"
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

func Blog_MustGetLatestByFeedUrl(tx pgw.Queryable, feedUrl string) (Blog, bool) {
	row := tx.QueryRow(`
		select id, name, status, update_action, coalesce(url, feed_url) from blogs
		where feed_url = $1 and version = $2
	`, feedUrl, BlogLatestVersion)

	var b Blog
	err := row.Scan(&b.Id, &b.Name, &b.Status, &b.UpdateAction, &b.BestUrl)
	if errors.Is(err, pgx.ErrNoRows) {
		return Blog{}, false //nolint:exhaustruct
	} else if err != nil {
		panic(err)
	}

	return b, true
}

// Break circular dependency between models and jobs. Jobs will be using models, models only sometimes need to
// schedule a job.
type GuidedCrawlingJobMustScheduleFunc func(tx pgw.Queryable, blogId BlogId, startFeedId StartFeedId)

func Blog_MustCreateOrUpdate(
	tx pgw.Queryable, startFeed StartFeed,
	guidedCrawlingJobMustScheduleFunc GuidedCrawlingJobMustScheduleFunc,
) Blog {
	blog, ok := Blog_MustGetLatestByFeedUrl(tx, startFeed.Url)
	if !ok {
		log.Info().
			Str("feed_url", startFeed.Url).
			Msg("Creating a new blog")
		blog = func() Blog {
			nestedTx := tx.MustBegin()
			defer util.CommitOrRollback(nestedTx, true, "")
			blog, ok := Blog_MustCreateWithCrawling(nestedTx, startFeed, guidedCrawlingJobMustScheduleFunc)
			if !ok {
				// Another writer must've created the record at the same time, let's use that
				panic(fmt.Errorf(
					"Blog %s with latest version didn't exist, then existed, now doesn't exist",
					startFeed.Url,
				))
			}
			return blog
		}()
	}

	// A blog that is currently being crawled will come out fresh
	// A blog that failed crawl can't be fixed by updating
	// But a blog that failed update from feed can be retried, who knows
	if blog.Status == BlogStatusCrawlInProgress ||
		blog.Status == BlogStatusCrawlFailed ||
		blog.Status == BlogStatusCrawledLooksWrong {
		return blog
	}

	// Update blog from feed
	var blogPostCuris []crawler.CanonicalUri
	logger := &crawler.ZeroLogger{}
	rows, err := tx.Query(`select url from blog_posts where blog_id = $1 order by index desc`, blog.Id)
	if err != nil {
		panic(err)
	}
	for rows.Next() {
		var blogPostUrl string
		err := rows.Scan(&blogPostUrl)
		if err != nil {
			panic(err)
		}
		blogPostLink, ok := crawler.ToCanonicalLink(blogPostUrl, logger, nil)
		if !ok {
			panic(fmt.Errorf("couldn't parse blog post url: %s", blogPostUrl))
		}
		blogPostCuris = append(blogPostCuris, blogPostLink.Curi)
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}
	curiEqCfg := BlogCanonicalEqualityConfig_MustGet(tx, blog.Id)
	discardedFeedEntryUrls := BlogDiscardedFeedEntry_MustListUrls(tx, blog.Id)
	missingFromFeedEntryUrls := BlogMissingFromFeedEntry_MustListUrls(tx, blog.Id)
	finalUri, err := url.Parse(startFeed.Url)
	if err != nil {
		panic(err)
	}
	newLinks, newLinksOk := crawler.MustExtractNewPostsFromFeed(
		*startFeed.MaybeParsedFeed, finalUri, blogPostCuris, discardedFeedEntryUrls, missingFromFeedEntryUrls,
		curiEqCfg, logger, logger,
	)

	if newLinksOk && len(newLinks) == 0 {
		log.Info().
			Str("feed_url", startFeed.Url).
			Msg("Blog doesn't need updating")
		return blog
	}

	switch blog.UpdateAction {
	case BlogUpdateActionRecrawl:
		log.Info().
			Str("feed_url", startFeed.Url).
			Msg("Blog is marked to recrawl on update")
		nestedTx := tx.MustBegin()
		defer util.CommitOrRollback(nestedTx, true, "")
		if Blog_MustDowngrade(nestedTx, blog.Id, startFeed.Url) {
			blog, ok := Blog_MustCreateWithCrawling(nestedTx, startFeed, guidedCrawlingJobMustScheduleFunc)
			if !ok {
				panic(fmt.Errorf("Couldn't create blog %s with latest version", startFeed.Url))
			}
			return blog
		} else {
			// Another writer deprecated this blog at the same time
			blog, ok := Blog_MustGetLatestByFeedUrl(nestedTx, startFeed.Url)
			if !ok {
				panic(fmt.Errorf(
					"Blog %s with latest version was deprecated by another request but the latest version still doesn't exist",
					startFeed.Url,
				))
			}
			return blog
		}
	case BlogUpdateActionUpdateFromFeedOrFail:
		if newLinksOk {
			log.Info().
				Str("feed_url", startFeed.Url).
				Int("new_links_count", len(newLinks)).
				Msg("Updating blog from feed")
			row := tx.QueryRow(`
				select id from blog_post_categories
				where blog_id = $1 and name = 'Everything'
			`, blog.Id)
			var everythingId BlogPostCategoryId
			err := row.Scan(&everythingId)
			if err != nil {
				panic(err)
			}
			nestedTx := tx.MustBegin()
			defer util.CommitOrRollback(nestedTx, true, "")
			rows, err = nestedTx.Query(`
				select blog_id from blog_post_locks
				where blog_id = $1
				for update nowait
			`, blog.Id)
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.LockNotAvailable {
				log.Info().
					Str("feed_url", startFeed.Url).
					Msg("Someone else is updating the blog posts, just waiting till they're done")
				rows, err := nestedTx.Query(`
					select blog_id from blog_post_locks
					where blog_id = $1
					for update
				`, blog.Id)
				if err != nil {
					panic(err)
				}
				rows.Close()
				log.Info().
					Str("feed_url", startFeed.Url).
					Msg("Done waiting")

				// Assume that the other writer has put the fresh posts in
				// There could be a race condition where two updates and a post publish happened at the
				// same time, the other writer succeeds with an older list, the current writer fails with
				// a newer list and ends up using the older list. But if both updates came a second
				// earlier, both would get the older list, and the new post would still get published a
				// second later, so the UX is the same and there's nothing we could do about it.
				blog, ok := Blog_MustGetLatestByFeedUrl(nestedTx, startFeed.Url)
				if !ok {
					panic(fmt.Errorf(
						"Blog %s with latest version was updated by another request but the latest version also doesn't exist",
						startFeed.Url,
					))
				}
				return blog
			} else if err != nil {
				panic(err)
			}
			rows.Close()

			row = nestedTx.QueryRow("select max(index) from blog_posts where blog_id = $1", blog.Id)
			var maxIndex int
			err = row.Scan(&maxIndex)
			if err != nil {
				panic(err)
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
					values (blog_post_ids.id, $5)
				`, blog.Id, index, link.Url, title, everythingId)
			}
			err = nestedTx.SendBatch(batch).Close()
			if err != nil {
				panic(err)
			}
			return blog
		} else {
			log.Warn().
				Str("feed_url", startFeed.Url).
				Msg("Couldn't update blog from feed, marking as failed")
			tx.MustExec("update blogs set status = $1 where id = $2", BlogStatusUpdateFromFeedFailed, blog.Id)
			blog.Status = BlogStatusUpdateFromFeedFailed
			return blog
		}
	case BlogUpdateActionFail:
		log.Warn().
			Str("feed_url", startFeed.Url).
			Msg("Blog is marked to fail on update")
		tx.MustExec("update blogs set status = $1 where id = $2", BlogStatusUpdateFromFeedFailed, blog.Id)
		blog.Status = BlogStatusUpdateFromFeedFailed
		return blog
	case BlogUpdateActionNoOp:
		log.Info().
			Str("feed_url", startFeed.Url).
			Msg("Blog is marked to never update")
		return blog
	default:
		panic(fmt.Errorf("unexpected blog update action: %s", blog.UpdateAction))
	}
}

func Blog_MustCreateWithCrawling(
	tx pgw.Queryable, startFeed StartFeed,
	guidedCrawlingJobMustScheduleFunc GuidedCrawlingJobMustScheduleFunc,
) (blog Blog, ok bool) {
	blogId := BlogId(mutil.MustGenerateRandomId(tx, "blogs"))
	status := BlogStatusCrawlInProgress
	updateAction := BlogUpdateActionRecrawl
	_, err := tx.Exec(`
		insert into blogs (id, name, feed_url, url, status, status_updated_at, version, update_action)
		values ($1, $2, $3, null, $4, utc_now(), $5, $6)
	`, blogId, startFeed.Title, startFeed.Url, status, BlogLatestVersion, updateAction)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation &&
		pgErr.ConstraintName == "index_blogs_on_feed_url_and_version" {
		return Blog{}, false //nolint:exhaustruct
	} else if err != nil {
		panic(err)
	}

	BlogCrawlProgress_MustCreate(tx, blogId)
	BlogPostLock_MustCreate(tx, blogId)
	guidedCrawlingJobMustScheduleFunc(tx, blogId, startFeed.Id)

	return Blog{
		Id:           blogId,
		Name:         startFeed.Title,
		Status:       status,
		UpdateAction: updateAction,
		BestUrl:      startFeed.Url,
	}, true
}

func BlogPostLock_MustCreate(tx pgw.Queryable, blogId BlogId) {
	tx.MustExec(`
		insert into blog_post_locks (blog_id) values ($1)
	`, blogId)
}

func Blog_MustDowngrade(tx pgw.Queryable, blogId BlogId, feedUrl string) (success bool) {
	row := tx.QueryRow(`
		update blogs
		set version = (select count(1) from blogs where feed_url = $1)
		where id = $2 and version = $3
		returning version
	`, feedUrl, blogId, BlogLatestVersion)
	var version int64
	err := row.Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return false
	} else if err != nil {
		panic(err)
	}

	return true
}

func Blog_MustGetNameStatusById(tx pgw.Queryable, blogId BlogId) (name string, status BlogStatus) {
	row := tx.QueryRow("select name, status from blogs where id = $1", blogId)
	err := row.Scan(&name, &status)
	if err != nil {
		panic(err)
	}

	return name, status
}
