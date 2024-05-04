package models

import (
	"bytes"
	"feedrewind/db/pgw"
	"feedrewind/models/mutil"
	"feedrewind/util/schedule"
	"fmt"
	"strings"

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

func Subscription_SetUserId(tx pgw.Queryable, id SubscriptionId, userId UserId) error {
	_, err := tx.Exec("update subscriptions_without_discarded set user_id = $1 where id = $2", userId, id)
	return err
}

func Subscription_SetIsPaused(tx pgw.Queryable, subscriptionId SubscriptionId, isPaused bool) error {
	_, err := tx.Exec(`
		update subscriptions_without_discarded set is_paused = $1 where id = $2
	`, isPaused, subscriptionId)
	return err
}

func Subscription_Delete(tx pgw.Queryable, subscriptionId SubscriptionId) error {
	// Has to be with_discarded because adding timestamp to without_discarded is a constraint violation
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
) (map[schedule.DayOfWeek][]string, error) {
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

	result := make(map[schedule.DayOfWeek][]string)
	for rows.Next() {
		var name string
		var dayOfWeek schedule.DayOfWeek
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
	PublishDate schedule.Date
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
		var maybeUrl, maybeTitle *string
		var maybePublishDate *schedule.Date
		var maybeCount *int
		err := rows.Scan(&tag, &maybeUrl, &maybeTitle, &maybePublishDate, &maybeCount)
		if err != nil {
			return nil, err
		}

		switch tag {
		case "prev_post":
			result.PrevPosts = append(result.PrevPosts, SchedulePreviewPrevPost{
				Url:         *maybeUrl,
				Title:       *maybeTitle,
				PublishDate: *maybePublishDate,
			})
		case "next_post":
			result.NextPosts = append(result.NextPosts, SchedulePreviewNextPost{
				Url:   *maybeUrl,
				Title: *maybeTitle,
			})
		case "published_count":
			publishedCount = *maybeCount
		case "total_count":
			totalCount = *maybeCount
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
		) values ($1, $2, $3, $4, $5, $6, $7, $8)
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
	finishedSetupAt schedule.Time, scheduleVersion int, isAddedPastMidnight bool,
) error {
	_, err := tx.Exec(`
		update subscriptions_without_discarded
		set name = $1, status = $2, finished_setup_at = $3, schedule_version = $4, is_added_past_midnight = $5
		where id = $6
	`, name, status, finishedSetupAt, scheduleVersion, isAddedPastMidnight, subscriptionId)
	return err
}

type SubscriptionToPublish struct {
	Id                        SubscriptionId
	Name                      string
	IsPaused                  bool
	FinishedSetupAt           schedule.Time
	MaybeFinalItemPublishedAt *schedule.Time
	BlogId                    BlogId
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
		err := rows.Scan(
			&s.Id, &s.Name, &s.IsPaused, &s.FinishedSetupAt, &s.MaybeFinalItemPublishedAt, &s.BlogId,
		)
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

// SubscriptionPost

type SubscriptionPostId int64
type SubscriptionPostRandomId string

type SubscriptionBlogPost struct {
	Id               SubscriptionPostId
	Title            string
	RandomId         SubscriptionPostRandomId
	MaybePublishedAt *schedule.Time
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
		err := rows.Scan(&p.Id, &p.Title, &p.RandomId, &p.MaybePublishedAt)
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

type PublishedSubscriptionBlogPost struct {
	Id          SubscriptionPostId
	Title       string
	RandomId    SubscriptionPostRandomId
	PublishedAt schedule.Time
}

func SubscriptionPost_GetLastPublishedDesc(
	tx pgw.Queryable, subscriptionId SubscriptionId, count int,
) ([]PublishedSubscriptionBlogPost, error) {
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

	var result []PublishedSubscriptionBlogPost
	for rows.Next() {
		var p PublishedSubscriptionBlogPost
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
	tx pgw.Queryable, postId SubscriptionPostId, publishedAt schedule.Time,
	publishedAtLocalDate schedule.Date, publishStatus PostPublishStatus,
) error {
	_, err := tx.Exec(`
		update subscription_posts
		set published_at = $1, published_at_local_date = $2, publish_status = $3
		where id = $4
	`, publishedAt, publishedAtLocalDate, publishStatus, postId)
	return err
}

func SubscriptionPost_GetUnpublishedCount(tx pgw.Queryable, subscriptionId SubscriptionId) (int, error) {
	row := tx.QueryRow(`
		select count(1) from subscription_posts where subscription_id = $1 and published_at is null
	`, subscriptionId)
	var result int
	err := row.Scan(&result)
	return result, err
}

// Schedule

func Schedule_Create(
	tx pgw.Queryable, subscriptionId SubscriptionId, countsByDay map[schedule.DayOfWeek]int,
) error {
	var valuesSql strings.Builder
	for dayOfWeek, count := range countsByDay {
		if valuesSql.Len() > 0 {
			fmt.Fprint(&valuesSql, ", ")
		}
		fmt.Fprintf(&valuesSql, "(%d, '%s', %d)", subscriptionId, dayOfWeek, count)
	}
	_, err := tx.Exec(`
		insert into schedules (subscription_id, day_of_week, count)
		values ` + valuesSql.String() + `
	`)
	return err
}

func Schedule_GetCount(
	tx pgw.Queryable, subscriptionId SubscriptionId, dayOfWeek schedule.DayOfWeek,
) (int, error) {
	row := tx.QueryRow(`
		select count from schedules where subscription_id = $1 and day_of_week = $2
	`, subscriptionId, dayOfWeek)
	var result int
	err := row.Scan(&result)
	return result, err
}

func Schedule_GetCountsByDay(
	tx pgw.Queryable, subscriptionId SubscriptionId,
) (map[schedule.DayOfWeek]int, error) {
	rows, err := tx.Query(`
		select day_of_week, count
		from schedules
		where subscription_id = $1
	`, subscriptionId)
	if err != nil {
		return nil, err
	}

	countByDay := make(map[schedule.DayOfWeek]int)
	for rows.Next() {
		var day schedule.DayOfWeek
		var count int
		err := rows.Scan(&day, &count)
		if err != nil {
			return nil, err
		}

		countByDay[day] = count
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return countByDay, nil
}

func Schedule_Update(
	tx pgw.Queryable, subscriptionId SubscriptionId, countsByDay map[schedule.DayOfWeek]int,
) error {
	var queryBuf bytes.Buffer
	queryBuf.WriteString(`
		update schedules as s set count = n.count
		from (values
	`)
	isFirst := true
	for dayOfWeek, count := range countsByDay {
		if isFirst {
			isFirst = false
		} else {
			queryBuf.WriteString(",")
		}
		queryBuf.WriteString(fmt.Sprintf("('%s'::day_of_week, %d)", dayOfWeek, count))
	}
	queryBuf.WriteString(`
		) as n(day_of_week, count)
		where s.day_of_week = n.day_of_week and subscription_id = $1
	`)
	query := queryBuf.String()

	_, err := tx.Exec(query, subscriptionId)
	return err
}
