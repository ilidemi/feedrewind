package models

import (
	"feedrewind/db/pgw"
	"feedrewind/models/mutil"
	"feedrewind/util"
	"fmt"
	"strings"
	"time"

	"errors"

	"github.com/jackc/pgx/v5"
)

type SubscriptionId int64

type SubscriptionStatus string

const (
	SubscriptionStatusWaitingForBlog = "waiting_for_blog"
	SubscriptionStatusSetup          = "setup"
	SubscriptionStatusLive           = "live"
)

func Subscription_Exists(tx pgw.Queryable, id SubscriptionId) (bool, error) {
	row := tx.QueryRow("select 1 from subscriptions_without_discarded where id = $1", id)
	var one int
	err := row.Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, err
	}

	return true, nil
}

func Subscription_GetUserIdBlogId(tx pgw.Queryable, id SubscriptionId) (UserId, BlogId, error) {
	row := tx.QueryRow("select user_id, blog_id from subscriptions_without_discarded where id = $1", id)
	var userIdPtr *UserId
	var blogId BlogId
	err := row.Scan(&userIdPtr, &blogId)
	if err != nil {
		return 0, blogId, err
	}

	if userIdPtr == nil {
		return 0, blogId, nil
	}
	return *userIdPtr, blogId, nil
}

func Subscription_SetUserId(tx pgw.Queryable, id SubscriptionId, userId UserId) error {
	_, err := tx.Exec("update subscriptions_without_discarded set user_id = $1 where id = $2", userId, id)
	return err
}

type SubscriptionWithPostCounts struct {
	Id             SubscriptionId
	Name           string
	Status         SubscriptionStatus
	IsPaused       bool
	PublishedCount int
	TotalCount     int
}

func Subscription_ListWithPostCounts(
	tx pgw.Queryable, userId UserId,
) ([]SubscriptionWithPostCounts, error) {
	rows, err := tx.Query(`
		with user_subscriptions as (
			select id, name, status, is_paused, finished_setup_at, created_at
			from subscriptions_without_discarded
			where user_id = $1
		)
		select id, name, status, is_paused, published_count, total_count
		from user_subscriptions
		left join (
			select subscription_id,
				count(published_at) as published_count,
				count(1) as total_count
			from subscription_posts
			where subscription_id in (select id from user_subscriptions)
			group by subscription_id
		) as post_counts on subscription_id = id
		order by finished_setup_at desc, created_at desc
	`, userId)
	if err != nil {
		return nil, err
	}

	var result []SubscriptionWithPostCounts
	for rows.Next() {
		var s SubscriptionWithPostCounts
		var publishedCount, totalCount *int
		err := rows.Scan(&s.Id, &s.Name, &s.Status, &s.IsPaused, &publishedCount, &totalCount)
		if err != nil {
			return nil, err
		}
		if publishedCount != nil {
			s.PublishedCount = *publishedCount
		}
		if totalCount != nil {
			s.TotalCount = *totalCount
		}
		result = append(result, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

type SubscriptionFullWithPostCounts struct {
	Id                  SubscriptionId
	Name                string
	IsPaused            bool
	Status              SubscriptionStatus
	ScheduleVersion     int64
	IsAddedPastMidnight bool
	Url                 string
	PublishedCount      int
	TotalCount          int
}

var ErrSubscriptionNotFound = errors.New("subscription not found")

func Subscription_GetWithPostCounts(
	tx pgw.Queryable, subscriptionId SubscriptionId, userId UserId,
) (*SubscriptionFullWithPostCounts, error) {
	row := tx.QueryRow(`
		select id, name, is_paused, status, schedule_version, is_added_past_midnight,
			(select url from blogs where id = blog_id) as url,
			(
				select count(published_at) from subscription_posts
				where subscription_id = subscriptions_without_discarded.id
			) as published_count,
			(
				select count(1) from subscription_posts
				where subscription_id = subscriptions_without_discarded.id
			) as total_count
		from subscriptions_without_discarded
		where id = $1 and user_id = $2
	`, subscriptionId, userId)

	var s SubscriptionFullWithPostCounts
	err := row.Scan(
		&s.Id, &s.Name, &s.IsPaused, &s.Status, &s.ScheduleVersion, &s.IsAddedPastMidnight, &s.Url,
		&s.PublishedCount, &s.TotalCount,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSubscriptionNotFound
	} else if err != nil {
		return nil, err
	}

	return &s, nil
}

type SubscriptionUserIdBlogBestUrl struct {
	UserId      *UserId
	Status      SubscriptionStatus
	BlogBestUrl string
}

func Subscription_GetUserIdStatusBlogBestUrl(
	tx pgw.Queryable, subscriptionId SubscriptionId,
) (*SubscriptionUserIdBlogBestUrl, error) {
	row := tx.QueryRow(`
		select user_id, status, (
			select coalesce(url, feed_url) from blogs
			where blogs.id = subscriptions_without_discarded.blog_id
		) from subscriptions_without_discarded where id = $1
	`, subscriptionId)
	var s SubscriptionUserIdBlogBestUrl
	err := row.Scan(&s.UserId, &s.Status, &s.BlogBestUrl)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSubscriptionNotFound
	} else if err != nil {
		return nil, err
	}

	return &s, nil
}

func Subscription_SetIsPaused(tx pgw.Queryable, subscriptionId SubscriptionId, isPaused bool) error {
	_, err := tx.Exec(`
		update subscriptions_without_discarded set is_paused = $1 where id = $2
	`, isPaused, subscriptionId)
	return err
}

func Subscription_Delete(tx pgw.Queryable, subscriptionId SubscriptionId) error {
	_, err := tx.Exec(`
		update subscriptions_with_discarded
		set discarded_at = utc_now()
		where id = $1
	`, subscriptionId)
	return err
}

func Subscription_GetName(tx pgw.Queryable, subscriptionId SubscriptionId) (string, error) {
	row := tx.QueryRow(`
		select name from subscriptions_without_discarded where id = $1
	`, subscriptionId)
	var name string
	err := row.Scan(&name)
	return name, err
}

func Subscription_GetOtherNamesByDay(
	tx pgw.Queryable, currentSubscriptionId SubscriptionId, userId UserId,
) (map[util.DayOfWeek][]string, error) {
	rows, err := tx.Query(`
		with user_subscriptions as (
			select id, name, created_at from subscriptions_without_discarded
			where user_id = $1 and
			status = 'live'
		)  
		select name, day_of_week, day_count
		from user_subscriptions
		join (
			select subscription_id,
				count(published_at) as published_count,
				count(1) as total_count
			from subscription_posts
			where subscription_id in (select id from user_subscriptions)
			group by subscription_id
		) as post_counts on post_counts.subscription_id = id
		join (
			select subscription_id, day_of_week, count as day_count
			from schedules
			where count > 0 and subscription_id in (select id from user_subscriptions)
		) as schedules on schedules.subscription_id = id
		where id != $2 and published_count != total_count
		order by created_at desc
	`, userId, currentSubscriptionId)
	if err != nil {
		return nil, err
	}

	result := make(map[util.DayOfWeek][]string)
	for rows.Next() {
		var name string
		var dayOfWeek util.DayOfWeek
		var count int
		err := rows.Scan(&name, &dayOfWeek, &count)
		if err != nil {
			return nil, err
		}

		for i := 0; i < count; i++ {
			result[dayOfWeek] = append(result[dayOfWeek], name)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

type SchedulePreview struct {
	PrevPosts   []SchedulePreviewPrevPost
	NextPosts   []SchedulePreviewNextPost
	PrevHasMore bool
	NextHasMore bool
}

type SchedulePreviewPrevPost struct {
	Url         string
	Title       string
	PublishDate util.Date
}

type SchedulePreviewNextPost struct {
	Url   string
	Title string
}

func Subscription_GetSchedulePreview(
	tx pgw.Queryable, subscriptionId SubscriptionId,
) (*SchedulePreview, error) {
	rows, err := tx.Query(`(
		select
			'prev_post' as tag,
			url,
			title,
			published_at_local_date,
			null::bigint as count
		from subscription_posts
		join (select id, url, title, index from blog_posts) as blog_posts on blog_posts.id = blog_post_id 
		where subscription_id = $1 and published_at is not null
		order by index desc
		limit 2
	) UNION ALL (
		select 'next_post' as tag, url, title, published_at_local_date, null as count
		from subscription_posts
		join (select id, url, title, index from blog_posts) as blog_posts on blog_posts.id = blog_post_id 
		where subscription_id = $1 and published_at is null
		order by index asc
		limit 5
	) UNION ALL (
		select 'published_count' as tag, null, null, null, count(published_at) as count from subscription_posts
		where subscription_id = $1
	) UNION ALL (
		select 'total_count' as tag, null, null, null, count(1) as count from subscription_posts
		where subscription_id = $1
	)`, subscriptionId)
	if err != nil {
		return nil, err
	}

	var result SchedulePreview
	var publishedCount, totalCount int
	for rows.Next() {
		var tag string
		var url, title *string
		var publishDate *util.Date
		var count *int
		err := rows.Scan(&tag, &url, &title, &publishDate, &count)
		if err != nil {
			return nil, err
		}

		switch tag {
		case "prev_post":
			result.PrevPosts = append(result.PrevPosts, SchedulePreviewPrevPost{
				Url:         *url,
				Title:       *title,
				PublishDate: *publishDate,
			})
		case "next_post":
			result.NextPosts = append(result.NextPosts, SchedulePreviewNextPost{
				Url:   *url,
				Title: *title,
			})
		case "published_count":
			publishedCount = *count
		case "total_count":
			totalCount = *count
		default:
			panic(fmt.Errorf("Unknown tag: %s", tag))
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i, j := 0, len(result.PrevPosts)-1; i < j; i, j = i+1, j-1 {
		result.PrevPosts[i], result.PrevPosts[j] = result.PrevPosts[j], result.PrevPosts[i]
	}

	unpublishedCount := totalCount - publishedCount

	result.PrevHasMore = (publishedCount - len(result.PrevPosts)) > 0
	if result.PrevHasMore {
		// Always show 2 lines: either all 2 prev posts or ellipsis and a post
		result.PrevPosts = result.PrevPosts[1:]
	}

	result.NextHasMore = (unpublishedCount - len(result.NextPosts)) > 0
	if result.NextHasMore {
		// Always show 5 lines: either all 5 next posts or 4 posts and ellipsis
		result.NextPosts = result.NextPosts[:len(result.NextPosts)-1]
	}

	return &result, nil
}

type SubscriptionUserIdStatusScheduleVersionBlogBestUrl struct {
	UserId          *UserId
	Status          SubscriptionStatus
	ScheduleVersion int64
	BlogBestUrl     string
}

func Subscription_GetUserIdStatusScheduleVersion(
	tx pgw.Queryable, subscriptionId SubscriptionId,
) (*SubscriptionUserIdStatusScheduleVersionBlogBestUrl, error) {
	row := tx.QueryRow(`
		select user_id, status, schedule_version, (
			select coalesce(url, feed_url) from blogs
			where blogs.id = subscriptions_without_discarded.blog_id
		) from subscriptions_without_discarded where id = $1
	`, subscriptionId)
	var s SubscriptionUserIdStatusScheduleVersionBlogBestUrl
	err := row.Scan(&s.UserId, &s.Status, &s.ScheduleVersion, &s.BlogBestUrl)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSubscriptionNotFound
	} else if err != nil {
		return nil, err
	}

	return &s, nil
}

type SubscriptionBlogStatus struct {
	SubscriptionStatus SubscriptionStatus
	BlogStatus         BlogStatus
	SubscriptionName   string
	BlogId             BlogId
	UserId             *UserId
}

func Subscription_GetStatus(
	tx pgw.Queryable, subscriptionId SubscriptionId,
) (*SubscriptionBlogStatus, error) {
	row := tx.QueryRow(`
		select status, (select status from blogs where id = blog_id) as blog_status, name, blog_id, user_id
		from subscriptions_without_discarded
		where id = $1
	`, subscriptionId)
	var s SubscriptionBlogStatus
	err := row.Scan(&s.SubscriptionStatus, &s.BlogStatus, &s.SubscriptionName, &s.BlogId, &s.UserId)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSubscriptionNotFound
	} else if err != nil {
		return nil, err
	}

	return &s, err
}

type SubscriptionBlogStatusPostsCount struct {
	SubscriptionStatus SubscriptionStatus
	BlogStatus         BlogStatus
	PostsCount         int
	BlogId             BlogId
	BlogBestUrl        string
	UserId             *UserId
}

func Subscription_GetStatusPostsCount(
	tx pgw.Queryable, subscriptionId SubscriptionId,
) (*SubscriptionBlogStatusPostsCount, error) {
	row := tx.QueryRow(`
		select
			status,
			(select status from blogs where id = blog_id) as blog_status,
			(
				select count(1)
				from blog_posts
				where blog_posts.blog_id = subscriptions_without_discarded.blog_id
			),
			blog_id,
			(select coalesce(url, feed_url) from blogs where id = blog_id),
			user_id
		from subscriptions_without_discarded
		where id = $1
	`, subscriptionId)
	var s SubscriptionBlogStatusPostsCount
	err := row.Scan(&s.SubscriptionStatus, &s.BlogStatus, &s.PostsCount, &s.BlogId, &s.BlogBestUrl, &s.UserId)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSubscriptionNotFound
	} else if err != nil {
		return nil, err
	}

	return &s, err
}

type SubscriptionBlogStatusBestUrl struct {
	SubscriptionStatus SubscriptionStatus
	BlogStatus         BlogStatus
	BlogName           string
	BlogBestUrl        string
	BlogId             BlogId
	UserId             *UserId
}

func Subscription_GetStatusBestUrl(
	tx pgw.Queryable, subscriptionId SubscriptionId,
) (*SubscriptionBlogStatusBestUrl, error) {
	row := tx.QueryRow(`
		select
			status,
			(select status from blogs where id = blog_id) as blog_status,
			(select name from blogs where id = blog_id) as blog_name,
			(select coalesce(url, feed_url) from blogs where id = blog_id),
			blog_id,
			user_id
		from subscriptions_without_discarded
		where id = $1
	`, subscriptionId)
	var s SubscriptionBlogStatusBestUrl
	err := row.Scan(&s.SubscriptionStatus, &s.BlogStatus, &s.BlogName, &s.BlogBestUrl, &s.BlogId, &s.UserId)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSubscriptionNotFound
	} else if err != nil {
		return nil, err
	}

	return &s, err
}

func Subscription_UpdateScheduleVersion(
	tx pgw.Queryable, subscriptionId SubscriptionId, scheduleVersion int64,
) error {
	_, err := tx.Exec(`
		update subscriptions_without_discarded set schedule_version = $1 where id = $2
	`, scheduleVersion, subscriptionId)
	return err
}

type SubscriptionCreateResult struct {
	Id          SubscriptionId
	BlogBestUrl string
	BlogStatus  BlogStatus
}

var ErrBlogFailed = errors.New("blog has failed status")

func Subscription_CreateForBlog(
	tx pgw.Queryable, blog *Blog, currentUser *User, productUserId ProductUserId,
) (*SubscriptionCreateResult, error) {
	if BlogFailedStatuses[blog.Status] {
		return nil, ErrBlogFailed
	} else {
		var userId *UserId
		var anonProductUserId *ProductUserId
		if currentUser != nil {
			userId = &currentUser.Id
			anonProductUserId = nil
		} else {
			userId = nil
			anonProductUserId = &productUserId
		}

		return Subscription_Create(
			tx, userId, anonProductUserId, blog, SubscriptionStatusWaitingForBlog, false, 0,
		)
	}
}

func Subscription_Create(
	tx pgw.Queryable, userId *UserId, anonProductUserId *ProductUserId, blog *Blog, status SubscriptionStatus,
	isPaused bool, scheduleVersion int64,
) (*SubscriptionCreateResult, error) {
	idInt, err := mutil.RandomId(tx, "subscriptions_with_discarded")
	if err != nil {
		return nil, err
	}
	id := SubscriptionId(idInt)
	_, err = tx.Exec(`
		insert into subscriptions_without_discarded(
			id, user_id, anon_product_user_id, blog_id, name, status, is_paused, schedule_version
		) values (
			$1, $2, $3, $4, $5, $6, $7, $8
		)
	`, id, userId, anonProductUserId, blog.Id, blog.Name, status, isPaused, scheduleVersion)
	if err != nil {
		return nil, err
	}

	return &SubscriptionCreateResult{
		Id:          id,
		BlogBestUrl: blog.BestUrl,
		BlogStatus:  blog.Status,
	}, nil
}

type SubscriptionBlogCrawlTimes struct {
	UserId               UserId
	BlogFeedUrl          string
	BlogCrawlClientToken BlogCrawlClientToken
	BlogCrawlEpoch       int32
	BlogCrawlEpochTimes  string
}

func Subscription_GetBlogCrawlTimes(
	tx pgw.Queryable, subscriptionId SubscriptionId,
) (*SubscriptionBlogCrawlTimes, error) {
	row := tx.QueryRow(`
		select user_id, blogs.feed_url, blog_crawl_client_tokens.value, blog_crawl_progresses.epoch,
			blog_crawl_progresses.epoch_times
		from subscriptions_without_discarded
		join blogs on subscriptions_without_discarded.blog_id = blogs.id
		join blog_crawl_client_tokens
			on blog_crawl_client_tokens.blog_id = subscriptions_without_discarded.blog_id
		join blog_crawl_progresses
			on blog_crawl_progresses.blog_id = subscriptions_without_discarded.blog_id
		where subscriptions_without_discarded.id = $1
	`, subscriptionId)
	var d SubscriptionBlogCrawlTimes
	err := row.Scan(
		&d.UserId, &d.BlogFeedUrl, &d.BlogCrawlClientToken, &d.BlogCrawlEpoch, &d.BlogCrawlEpochTimes,
	)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// 16 urlsafe random bytes
var psqlRandomId = "rtrim(replace(replace(encode(gen_random_bytes(16), 'base64'), '+', '-'), '/', '_'), '=')"

func Subscription_CreatePostsFromCategory(
	tx pgw.Queryable, subscriptionId SubscriptionId, categoryId BlogPostCategoryId,
) error {
	_, err := tx.Exec(`
		insert into subscription_posts (subscription_id, blog_post_id, random_id, published_at)
		select $1, blog_post_id, `+psqlRandomId+`, null
		from blog_post_category_assignments
		where category_id = $2
	`, subscriptionId, categoryId)
	return err
}

func Subscription_CreatePostsFromIds(
	tx pgw.Queryable, subscriptionId SubscriptionId, blogId BlogId, blogPostIds map[BlogPostId]bool,
) error {
	var blogPostIdsStr strings.Builder
	for blogPostId := range blogPostIds {
		if blogPostIdsStr.Len() > 0 {
			fmt.Fprint(&blogPostIdsStr, ", ")
		}
		fmt.Fprint(&blogPostIdsStr, blogPostId)
	}

	_, err := tx.Exec(`
		insert into subscription_posts (subscription_id, blog_post_id, random_id, published_at)
		select $1, id, `+psqlRandomId+`, null
		from blog_posts
		where blog_id = $2 and id in (`+blogPostIdsStr.String()+`)
	`, subscriptionId, blogId)
	return err
}

func Subscription_UpdateStatus(
	tx pgw.Queryable, subscriptionId SubscriptionId, newStatus SubscriptionStatus,
) error {
	_, err := tx.Exec(`
		update subscriptions_without_discarded set status = $1 where id = $2
	`, newStatus, subscriptionId)
	return err
}

func Subscription_FinishSetup(
	tx pgw.Queryable, subscriptionId SubscriptionId, name string, status SubscriptionStatus,
	finishedSetupAt time.Time, scheduleVersion int, isAddedPastMidnight bool,
) error {
	_, err := tx.Exec(`
		update subscriptions_without_discarded
		set name = $1, status = $2, finished_setup_at = $3, schedule_version = $4, is_added_past_midnight = $5
		where id = $6
	`, name, status, finishedSetupAt, scheduleVersion, isAddedPastMidnight, subscriptionId)
	return err
}

type SubscriptionToPublish struct {
	Id                   SubscriptionId
	Name                 string
	IsPaused             *bool
	FinishedSetupAt      time.Time
	FinalItemPublishedAt *time.Time
	BlogId               BlogId
}

func Subscription_ListSortedToPublish(tx pgw.Queryable, userId UserId) ([]SubscriptionToPublish, error) {
	rows, err := tx.Query(`
		select id, name, is_paused, finished_setup_at, final_item_published_at, blog_id
		from subscriptions_without_discarded
		where user_id = $1 and status = $2
		order by finished_setup_at desc, id desc
	`, userId, SubscriptionStatusLive)
	if err != nil {
		return nil, err
	}

	var result []SubscriptionToPublish
	for rows.Next() {
		var s SubscriptionToPublish
		err := rows.Scan(&s.Id, &s.Name, &s.IsPaused, &s.FinishedSetupAt, &s.FinalItemPublishedAt, &s.BlogId)
		if err != nil {
			return nil, err
		}
		result = append(result, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

func Subscription_SetInitialItemPublishStatus(
	tx pgw.Queryable, subscriptionId SubscriptionId, initialItemPublishStatus PostPublishStatus,
) error {
	_, err := tx.Exec(`
		update subscriptions_without_discarded set initial_item_publish_status = $1 where id = $2
	`, initialItemPublishStatus, subscriptionId)
	return err
}

func Subscription_SetFinalItemPublished(
	tx pgw.Queryable, subscriptionId SubscriptionId, finalItemPublishedAt time.Time,
	finalItemPublishStatus PostPublishStatus,
) error {
	_, err := tx.Exec(`
		update subscriptions_without_discarded set final_item_published_at = $1 where id = $2
	`, finalItemPublishedAt, subscriptionId)
	return err
}

type SubscriptionWithRss struct {
	IsPausedOrFinished bool
	Rss                string
	BlogBestUrl        string
	ProductUserId      ProductUserId
}

func Subscription_GetWithRss(tx pgw.Queryable, subscriptionId SubscriptionId) (*SubscriptionWithRss, error) {
	row := tx.QueryRow(`
		select
			(is_paused or final_item_published_at is not null),
			(select body from subscription_rsses where subscription_id = $1),
			(select coalesce(url, feed_url) from blogs where blogs.id = blog_id),
			(select product_user_id from users where users.id = user_id)
		from subscriptions_without_discarded where id = $1
	`, subscriptionId)
	var s SubscriptionWithRss
	err := row.Scan(&s.IsPausedOrFinished, &s.Rss, &s.BlogBestUrl, &s.ProductUserId)
	return &s, err
}

// SubscriptionPost

type SubscriptionPostId int64
type SubscriptionPostRandomId string

type SubscriptionBlogPost struct {
	Id          SubscriptionPostId
	Title       string
	RandomId    SubscriptionPostRandomId
	PublishedAt *time.Time
}

type PostPublishStatus string

const (
	PostPublishStatusRssPublished PostPublishStatus = "rss_published"
	PostPublishStatusEmailPending PostPublishStatus = "email_pending"
	PostPublishStatusEmailSkipped PostPublishStatus = "email_skipped"
	PostPublishStatusEmailSent    PostPublishStatus = "email_sent"
)

func SubscriptionPost_GetNextUnpublished(
	tx pgw.Queryable, subscriptionId SubscriptionId, count int,
) ([]SubscriptionBlogPost, error) {
	rows, err := tx.Query(`
		select subscription_posts.id, title, random_id, published_at from subscription_posts 
		join blog_posts on subscription_posts.blog_post_id = blog_posts.id
		where subscription_id = $1 and published_at is null
		order by index asc
		limit $2
	`, subscriptionId, count)
	if err != nil {
		return nil, err
	}

	var result []SubscriptionBlogPost
	for rows.Next() {
		var p SubscriptionBlogPost
		err := rows.Scan(&p.Id, &p.Title, &p.RandomId, &p.PublishedAt)
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

func SubscriptionPost_GetLastPublishedDesc(
	tx pgw.Queryable, subscriptionId SubscriptionId, count int,
) ([]SubscriptionBlogPost, error) {
	rows, err := tx.Query(`
		select subscription_posts.id, title, random_id, published_at from subscription_posts
		join blog_posts on subscription_posts.blog_post_id = blog_posts.id
		where subscription_id = $1 and published_at is not null
		order by index desc
		limit $2
	`, subscriptionId, count)
	if err != nil {
		return nil, err
	}

	var result []SubscriptionBlogPost
	for rows.Next() {
		var p SubscriptionBlogPost
		err := rows.Scan(&p.Id, &p.Title, &p.RandomId, &p.PublishedAt)
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

func SubscriptionPost_UpdatePublished(
	tx pgw.Queryable, postId SubscriptionPostId, publishedAt time.Time, publishedAtLocalDate util.Date,
	publishStatus PostPublishStatus,
) error {
	_, err := tx.Exec(`
		update subscription_posts
		set published_at = $1, published_at_local_date = $2, publish_status = $3
		where id = $4
	`, publishedAt, publishedAtLocalDate, publishStatus, postId)
	return err
}

func SubscriptionPost_GetPublishedCount(tx pgw.Queryable, subscriptionId SubscriptionId) (int, error) {
	row := tx.QueryRow(`
		select count(1) from subscription_posts where subscription_id = $1 and published_at is not null
	`, subscriptionId)
	var result int
	err := row.Scan(&result)
	return result, err
}

func SubscriptionPost_GetUnpublishedCount(tx pgw.Queryable, subscriptionId SubscriptionId) (int, error) {
	row := tx.QueryRow(`
		select count(1) from subscription_posts where subscription_id = $1 and published_at is null
	`, subscriptionId)
	var result int
	err := row.Scan(&result)
	return result, err
}

type SubscriptionBlogPostBestUrl struct {
	Url            string
	SubscriptionId SubscriptionId
	BlogBestUrl    string
	ProductUserId  ProductUserId
}

func SubscriptionPost_GetByRandomId(
	tx pgw.Queryable, randomId SubscriptionPostRandomId,
) (*SubscriptionBlogPostBestUrl, error) {
	row := tx.QueryRow(`
		select
			(select url from blog_posts where blog_posts.id = subscription_posts.blog_post_id),
			subscription_id,
			(select coalesce(url, feed_url) from blogs where blogs.id = (
				select blog_id from blog_posts where blog_posts.id = subscription_posts.blog_post_id
			)),
			(select product_user_id from users where users.id = (
				select user_id from subscriptions_with_discarded
				where subscriptions_with_discarded.id = subscription_id
			))
		from subscription_posts
		where random_id = $1
	`, randomId)
	var p SubscriptionBlogPostBestUrl
	err := row.Scan(&p.Url, &p.SubscriptionId, &p.BlogBestUrl, &p.ProductUserId)
	if err != nil {
		return nil, err
	}
	return &p, nil
}
