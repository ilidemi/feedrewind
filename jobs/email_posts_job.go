package jobs

import (
	"context"
	"feedrewind/db/pgw"
	"feedrewind/models"
	"feedrewind/oops"
	"feedrewind/publish"
	"feedrewind/util"
	"feedrewind/util/schedule"
	"time"

	"github.com/mrz1836/postmark"
)

func init() {
	registerJobNameFunc(
		"EmailPostsJob",
		func(ctx context.Context, id JobId, conn *pgw.Conn, args []any) error {
			if len(args) != 4 {
				return oops.Newf("Expected 4 args, got %d: %v", len(args), args)
			}

			userIdInt64, ok := args[0].(int64)
			if !ok {
				userIdInt, ok := args[0].(int)
				if !ok {
					return oops.Newf("Failed to parse userId (expected int64 or int): %v", args[0])
				}
				userIdInt64 = int64(userIdInt)
			}
			userId := models.UserId(userIdInt64)

			dateStr, ok := args[1].(string)
			if !ok {
				return oops.Newf("Failed to parse date (expected string): %v", args[1])
			}
			date := schedule.Date(dateStr)

			scheduledFor, ok := args[2].(string)
			if !ok {
				return oops.Newf("Failed to parse scheduledFor (expected string): %v", args[2])
			}

			finalItemSubscriptionIdsAny, ok := args[3].([]any)
			if !ok {
				return oops.Newf("Failed to parse finalItemSubscriptionIds (expected a slice): %v", args[3])
			}
			var finalItemSubscriptionIds []models.SubscriptionId
			for i, subIdAny := range finalItemSubscriptionIdsAny {
				subscriptionIdInt64, ok := subIdAny.(int64)
				if !ok {
					subscriptionIdInt, ok := subIdAny.(int)
					if !ok {
						return oops.Newf(
							"Failed to parse finalItemSubscriptionIds[%d] (expected int64 or int): %v",
							i, subIdAny,
						)
					}
					subscriptionIdInt64 = int64(subscriptionIdInt)
				}
				finalItemSubscriptionIds =
					append(finalItemSubscriptionIds, models.SubscriptionId(subscriptionIdInt64))
			}

			return EmailPostsJob_Perform(ctx, conn, userId, date, scheduledFor, finalItemSubscriptionIds)
		},
	)

	publish.EmailPostsJob_PerformNowFunc = EmailPostsJob_PerformNow
}

func EmailPostsJob_PerformNow(
	tx pgw.Queryable, userId models.UserId, date schedule.Date, scheduledFor string,
	finalItemSubscriptionIds []models.SubscriptionId,
) error {
	finalItemSubscriptionIdInts := make([]int64, 0, len(finalItemSubscriptionIds))
	for _, subscriptionId := range finalItemSubscriptionIds {
		finalItemSubscriptionIdInts = append(finalItemSubscriptionIdInts, int64(subscriptionId))
	}
	return performNow(
		tx, "EmailPostsJob", defaultQueue, int64ToYaml(int64(userId)), strToYaml(string(date)),
		strToYaml(scheduledFor), int64ListToYaml(finalItemSubscriptionIdInts),
	)
}

func EmailPostsJob_Perform(
	ctx context.Context, conn *pgw.Conn, userId models.UserId, date schedule.Date, scheduledFor string,
	finalItemSubscriptionIds []models.SubscriptionId,
) error {
	return util.Tx(conn, func(tx *pgw.Tx, conn util.Clobber) error {
		logger := tx.Logger()

		exists, err := models.User_Exists(tx, userId)
		if err != nil {
			return err
		}
		if !exists {
			logger.Info().Msgf("User %d not found", userId)
			return nil
		}

		bounced, err := models.PostmarkBouncedUser_Exists(tx, userId)
		if err != nil {
			return err
		}
		if bounced {
			logger.Info().Msgf("User %d marked as bounced, not sending anything", userId)
			return nil
		}

		row := tx.QueryRow(`select email, product_user_id from users_with_discarded where id = $1`, userId)
		var userEmail string
		var productUserId models.ProductUserId
		err = row.Scan(&userEmail, &productUserId)
		if err != nil {
			return err
		}

		type SubscriptionPost struct {
			Id                   models.SubscriptionPostId
			RandomId             models.SubscriptionPostRandomId
			SubscriptionId       models.SubscriptionId
			SubscriptionName     string
			BlogBestUrl          string
			PublishedAtLocalDate string
			Title                string
		}

		rows, err := tx.Query(`
			select
				id, random_id, subscription_id,
				(
					select name from subscriptions_without_discarded
					where subscriptions_without_discarded.id = subscription_posts.subscription_id
				),
				(
					select coalesce(url, feed_url) from blogs
					where blogs.id = (
						select blog_id from subscriptions_without_discarded
						where subscriptions_without_discarded.id = subscription_posts.subscription_id
					)
				),
				published_at_local_date,
				(select title from blog_posts where blog_posts.id = subscription_posts.blog_post_id)
			from subscription_posts
			where publish_status = 'email_pending' and
				subscription_id in (select id from subscriptions_without_discarded where user_id = $1)
		`, userId)
		if err != nil {
			return err
		}
		var postsToEmail []SubscriptionPost
		for rows.Next() {
			var p SubscriptionPost
			err := rows.Scan(
				&p.Id, &p.RandomId, &p.SubscriptionId, &p.SubscriptionName, &p.BlogBestUrl,
				&p.PublishedAtLocalDate, &p.Title,
			)
			if err != nil {
				return err
			}
			postsToEmail = append(postsToEmail, p)
		}
		if err := rows.Err(); err != nil {
			return err
		}

		if len(postsToEmail) == 0 {
			logger.Info().Msg("Nothing to do")
			return nil
		}

		postCountByDate := make(map[string]int)
		for _, postToEmail := range postsToEmail {
			postCountByDate[postToEmail.PublishedAtLocalDate]++
		}
		logger.Info().Msgf("Posts to email: %v", postCountByDate)

		client, maybeTestMetadata := GetPostmarkClientAndMaybeMetadata(tx)
		emails := make([]postmark.Email, len(postsToEmail))
		for i, postToEmail := range postsToEmail {
			emails[i] = newPostEmail(
				userId, userEmail, postToEmail.SubscriptionId, postToEmail.SubscriptionName, postToEmail.Id,
				postToEmail.Title, postToEmail.RandomId, maybeTestMetadata, scheduledFor,
			)
		}

		responses, err := client.SendEmailBatch(ctx, emails)
		if err != nil {
			return oops.Wrap(err)
		}

		type PostEmailResponse struct {
			Post     SubscriptionPost
			Email    postmark.Email
			Response postmark.EmailResponse
		}
		var sent []PostEmailResponse
		var notSent []PostEmailResponse
		for i, post := range postsToEmail {
			var collection *[]PostEmailResponse
			if responses[i].ErrorCode == 0 {
				collection = &sent
			} else {
				collection = &notSent
			}
			*collection = append(*collection, PostEmailResponse{
				Post:     post,
				Email:    emails[i],
				Response: responses[i],
			})
		}

		logger.Info().Msgf("Sent post messages: %d", len(sent))
		for _, per := range notSent {
			// Possible reasons for partial failure:
			// Rate limit exceeded
			// Not allowed to sent (ran out of credits)
			// Too many batch messages (?)
			logger.
				Warn().
				Any("response", per.Response).
				Any("metadata", per.Email.Metadata).
				Msg("Error sending email")
		}

		for _, per := range sent {
			logger.
				Info().
				Any("response", per.Response).
				Any("metadata", per.Email.Metadata).
				Msg("Sent post email")

			messageId := models.PostmarkMessageId(per.Response.MessageID)
			_, err := tx.Exec(`
				insert into postmark_messages (
					message_id, message_type, subscription_id, subscription_post_id
				)
				values ($1, $2, $3, $4)
			`, messageId, models.PostmarkMessageSubPost, per.Post.SubscriptionId, per.Post.Id)
			if err != nil {
				return err
			}

			_, err = tx.Exec(`
				update subscription_posts set publish_status = 'email_sent' where id = $1
			`, per.Post.Id)
			if err != nil {
				return err
			}

			err = models.ProductEvent_Emit(tx, productUserId, "send post email", map[string]any{
				"subscription_id": per.Post.SubscriptionId,
				"blog_url":        per.Post.BlogBestUrl,
			}, nil)
			if err != nil {
				return err
			}
		}

		if len(notSent) == 0 {
			// Schedule for a minute in the future so that the final email likely arrives last
			// Final item also depends on the email posts going out, so only sending it after a full success
			finalItemRunAt := schedule.UTCNow().Add(time.Minute)
			finalItemScheduledFor := finalItemRunAt.MustUTCString()
			for _, subscriptionId := range finalItemSubscriptionIds {
				err := EmailFinalItemJob_PerformAt(
					tx, finalItemRunAt, userId, subscriptionId, finalItemScheduledFor,
				)
				if err != nil {
					return err
				}
			}
			return nil
		} else {
			return oops.Newf("Messages not sent: %d", len(notSent))
		}
	})
}
