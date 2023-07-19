package models

import (
	"errors"
	"feedrewind/db/pgw"
	"feedrewind/models/mutil"
	"feedrewind/util"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type SubscriptionId int64

type SubscriptionStatus string

const (
	SubscriptionStatusWaitingForBlog = "waiting_for_blog"
	SubscriptionStatusSetup          = "setup"
	SubscriptionStatusLive           = "live"
)

func Subscription_MustExists(tx pgw.Queryable, id SubscriptionId) bool {
	row := tx.QueryRow("select 1 from subscriptions where id = $1", id)
	var one int
	err := row.Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return false
	} else if err != nil {
		panic(err)
	}

	return true
}

func Subscription_MustSetUserId(tx pgw.Queryable, id SubscriptionId, userId UserId) {
	tx.MustExec("update subscriptions set user_id = $1 where id = $2", userId, id)
}

type SubscriptionWithPostCounts struct {
	Id             SubscriptionId
	Name           string
	Status         SubscriptionStatus
	IsPaused       bool
	PublishedCount int
	TotalCount     int
}

func Subscription_MustListWithPostCounts(tx pgw.Queryable, userId UserId) []SubscriptionWithPostCounts {
	rows, err := tx.Query(`
		with user_subscriptions as (
			select id, name, status, is_paused, finished_setup_at, created_at from subscriptions
			where user_id = $1 and discarded_at is null
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
		panic(err)
	}

	var result []SubscriptionWithPostCounts
	for rows.Next() {
		var s SubscriptionWithPostCounts
		var publishedCount, totalCount *int
		err := rows.Scan(&s.Id, &s.Name, &s.Status, &s.IsPaused, &publishedCount, &totalCount)
		if err != nil {
			panic(err)
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
		panic(err)
	}

	return result
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

func Subscription_MustGetWithPostCounts(
	tx pgw.Queryable, subscriptionId SubscriptionId, userId UserId,
) (SubscriptionFullWithPostCounts, bool) {
	row := tx.QueryRow(`
		select id, name, is_paused, status, schedule_version, is_added_past_midnight,
			(select url from blogs where id = blog_id) as url,
			(
				select count(published_at) from subscription_posts where subscription_id = subscriptions.id
			) as published_count,
			(select count(1) from subscription_posts where subscription_id = subscriptions.id) as total_count
		from subscriptions
		where id = $1 and user_id = $2 and discarded_at is null
	`, subscriptionId, userId)

	var s SubscriptionFullWithPostCounts
	err := row.Scan(
		&s.Id, &s.Name, &s.IsPaused, &s.Status, &s.ScheduleVersion, &s.IsAddedPastMidnight, &s.Url,
		&s.PublishedCount, &s.TotalCount,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return SubscriptionFullWithPostCounts{}, false //nolint:exhaustruct
	} else if err != nil {
		panic(err)
	}

	return s, true
}

type SubscriptionUserIdBlogBestUrl struct {
	UserId      *UserId
	Status      SubscriptionStatus
	BlogBestUrl string
}

func Subscription_MustGetUserIdStatusBlogBestUrl(
	tx pgw.Queryable, subscriptionId SubscriptionId,
) (SubscriptionUserIdBlogBestUrl, bool) {
	row := tx.QueryRow(`
		select user_id, status, (
			select coalesce(url, feed_url) from blogs
			where blogs.id = subscriptions.blog_id
		) from subscriptions where id = $1
	`, subscriptionId)
	var s SubscriptionUserIdBlogBestUrl
	err := row.Scan(&s.UserId, &s.Status, &s.BlogBestUrl)
	if errors.Is(err, pgx.ErrNoRows) {
		return SubscriptionUserIdBlogBestUrl{}, false //nolint:exhaustruct
	} else if err != nil {
		panic(err)
	}

	return s, true
}

func Subscription_MustSetIsPaused(tx pgw.Queryable, subscriptionId SubscriptionId, isPaused bool) {
	tx.MustExec("update subscriptions set is_paused = $1 where id = $2", isPaused, subscriptionId)
}

func Subscription_MustDelete(tx pgw.Queryable, subscriptionId SubscriptionId) {
	tx.MustExec("delete from subscriptions where id = $1", subscriptionId)
}

func Subscription_MustGetOtherNamesByDay(
	tx pgw.Queryable, currentSubscriptionId SubscriptionId, userId UserId,
) map[util.DayOfWeek][]string {
	rows, err := tx.Query(`
		with user_subscriptions as (
			select id, name, created_at from subscriptions
			where user_id = $1 and
			status = 'live' and
			discarded_at is null
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
		panic(err)
	}

	result := make(map[util.DayOfWeek][]string)
	for rows.Next() {
		var name string
		var dayOfWeek util.DayOfWeek
		var count int
		err := rows.Scan(&name, &dayOfWeek, &count)
		if err != nil {
			panic(err)
		}

		for i := 0; i < count; i++ {
			result[dayOfWeek] = append(result[dayOfWeek], name)
		}
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}

	return result
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

func Subscription_MustGetSchedulePreview(tx pgw.Queryable, subscriptionId SubscriptionId) SchedulePreview {
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
		panic(err)
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
			panic(err)
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
		panic(err)
	}

	for i, j := 0, len(result.PrevPosts)-1; i < j; i, j = i+1, j-1 {
		result.PrevPosts[i], result.PrevPosts[j] = result.PrevPosts[j], result.PrevPosts[i]
	}

	unpublishedCount := totalCount - publishedCount

	prevHasMore := (publishedCount - len(result.PrevPosts)) > 0
	if prevHasMore {
		// Always show 2 lines: either all 2 prev posts or ellipsis and a post
		result.PrevPosts = result.PrevPosts[1:]
	}

	nextHasMore := (unpublishedCount - len(result.NextPosts)) > 0
	if nextHasMore {
		// Always show 5 lines: either all 5 next posts or 4 posts and ellipsis
		result.NextPosts = result.NextPosts[:len(result.NextPosts)-1]
	}

	return result
}

type SubscriptionUserIdStatusScheduleVersionBlogBestUrl struct {
	UserId          *UserId
	Status          SubscriptionStatus
	ScheduleVersion int64
	BlogBestUrl     string
}

func Subscription_MustGetUserIdStatusScheduleVersion(
	tx pgw.Queryable, subscriptionId SubscriptionId,
) (SubscriptionUserIdStatusScheduleVersionBlogBestUrl, bool) {
	row := tx.QueryRow(`
		select user_id, status, schedule_version, (
			select coalesce(url, feed_url) from blogs
			where blogs.id = subscriptions.blog_id
		) from subscriptions where id = $1
	`, subscriptionId)
	var s SubscriptionUserIdStatusScheduleVersionBlogBestUrl
	err := row.Scan(&s.UserId, &s.Status, &s.ScheduleVersion, &s.BlogBestUrl)
	if errors.Is(err, pgx.ErrNoRows) {
		return SubscriptionUserIdStatusScheduleVersionBlogBestUrl{}, false //nolint:exhaustruct
	} else if err != nil {
		panic(err)
	}

	return s, true
}

func Subscription_MustUpdateScheduleVersion(
	tx pgw.Queryable, subscriptionId SubscriptionId, scheduleVersion int64,
) {
	tx.MustExec(`
		update subscriptions set schedule_version = $1 where id = $2
	`, scheduleVersion, subscriptionId)
}

type SubscriptionCreateResult struct {
	Id          SubscriptionId
	BlogBestUrl string
	BlogStatus  BlogStatus
}

func Subscription_MustCreateForBlog(
	tx pgw.Queryable, blog Blog, currentUser *User, productUserId ProductUserId,
) (SubscriptionCreateResult, bool) {
	if BlogFailedStatuses[blog.Status] {
		return SubscriptionCreateResult{}, false //nolint:exhaustruct
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

		return Subscription_MustCreate(
			tx, userId, anonProductUserId, blog, SubscriptionStatusWaitingForBlog, false, 0,
		), true
	}
}

func Subscription_MustCreate(
	tx pgw.Queryable, userId *UserId, anonProductUserId *ProductUserId, blog Blog, status SubscriptionStatus,
	isPaused bool, scheduleVersion int64,
) SubscriptionCreateResult {
	id := SubscriptionId(mutil.MustGenerateRandomId(tx, "subscriptions"))
	tx.MustExec(`
		insert into subscriptions(
			id, user_id, anon_product_user_id, blog_id, name, status, is_paused, schedule_version
		) values (
			$1, $2, $3, $4, $5, $6, $7, $8
		)
	`, id, userId, anonProductUserId, blog.Id, blog.Name, status, isPaused, scheduleVersion)

	return SubscriptionCreateResult{
		Id:          id,
		BlogBestUrl: blog.BestUrl,
		BlogStatus:  blog.Status,
	}
}
