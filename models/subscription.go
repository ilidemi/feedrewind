package models

import (
	"context"
	"errors"
	"feedrewind/db/pgw"

	"github.com/jackc/pgx/v5"
)

type SubscriptionId int64

type SubscriptionStatus string

const (
	SubscriptionStatusCrawlInProgress   = "crawl_in_progress"
	SubscriptionStatusCrawled           = "crawled"
	SubscriptionStatusConfirmed         = "confirmed"
	SubscriptionStatusLive              = "live"
	SubscriptionStatusCrawlFailed       = "crawl_failed"
	SubscriptionStatusCrawledLooksWrong = "crawled_looks_wrong"
)

func Subscription_MustExists(tx pgw.Queryable, id SubscriptionId) bool {
	ctx := context.Background()
	row := tx.QueryRow(ctx, "select 1 from subscriptions where id = $1", id)
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
	ctx := context.Background()
	tx.MustExec(ctx, "update subscriptions set user_id = $1 where id = $2", userId, id)
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
	ctx := context.Background()
	rows, err := tx.Query(ctx, `
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

type SubscriptionUserIdBlogBestUrl struct {
	UserId      *UserId
	BlogBestUrl string
}

func Subscription_MustGetUserIdBlogBestUrl(
	tx pgw.Queryable, subscriptionId SubscriptionId,
) (SubscriptionUserIdBlogBestUrl, bool) {
	ctx := context.Background()
	row := tx.QueryRow(ctx, `
		select user_id, (
			select coalesce(url, feed_url) from blogs
			where blogs.id = subscriptions.blog_id
		) from subscriptions where id = $1
	`, subscriptionId)
	var s SubscriptionUserIdBlogBestUrl
	err := row.Scan(&s.UserId, &s.BlogBestUrl)
	if errors.Is(err, pgx.ErrNoRows) {
		return SubscriptionUserIdBlogBestUrl{nil, ""}, false
	} else if err != nil {
		panic(err)
	}

	return s, true
}

func Subscription_MustDelete(tx pgw.Queryable, subscriptionId SubscriptionId) {
	ctx := context.Background()
	tx.MustExec(ctx, "delete from subscriptions where id = $1", subscriptionId)
}
