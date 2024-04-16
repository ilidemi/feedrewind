package jobs

import (
	"context"
	"errors"
	"feedrewind/db/pgw"
	"feedrewind/models"
	"feedrewind/oops"
	"feedrewind/util"
	"feedrewind/util/schedule"
	"time"

	"github.com/jackc/pgx/v5"
)

func init() {
	registerJobNameFunc("ProcessPostmarkBounceJob",
		func(ctx context.Context, conn *pgw.Conn, args []any) error {
			if len(args) != 1 {
				return oops.Newf("Expected 1 arg, got %d: %v", len(args), args)
			}

			bounceId, ok := args[0].(int64)
			if !ok {
				bounceIdInt, ok := args[0].(int)
				if !ok {
					return oops.Newf("Failed to parse bounceId (expected int64 or int): %v", args[0])
				}
				bounceId = int64(bounceIdInt)
			}

			return ProcessPostmarkBounceJob_Perform(ctx, conn, bounceId)
		},
	)
}

func ProcessPostmarkBounceJob_PerformNow(tx pgw.Queryable, bounceId int64) error {
	return performNow(tx, "ProcessPostmarkBounceJob", defaultQueue, int64ToYaml(bounceId))
}

func ProcessPostmarkBounceJob_Perform(ctx context.Context, conn *pgw.Conn, bounceId int64) error {
	logger := conn.Logger()

	bounce, err := models.PostmarkBounce_GetById(conn, bounceId)
	if err != nil {
		return err
	}

	if bounce.MaybeMessageId == nil {
		logger.Error().Msgf("Bounce %d came without message id", bounce.Id)
		return nil
	}
	messageId := *bounce.MaybeMessageId

	// If a bounce came before the message is saved, query will fail, the job will retry later and it's ok
	row := conn.QueryRow(`
		select message_type, subscription_id, subscription_post_id
		from postmark_messages
		where message_id = $1
	`, messageId)
	var messageType models.PostmarkMessageType
	var subscriptionId models.SubscriptionId
	var maybeSubscriptionPostId *models.SubscriptionPostId
	err = row.Scan(&messageType, &subscriptionId, &maybeSubscriptionPostId)
	if err != nil {
		return err
	}

	row = conn.QueryRow(`
		select user_id, (
			select product_user_id from users_with_discarded
			where users_with_discarded.id = subscriptions_with_discarded.user_id
		), (
			select coalesce(url, feed_url) from blogs
			where blogs.id = subscriptions_with_discarded.blog_id
		), (
			select count(1) from postmark_bounced_users
			where postmark_bounced_users.user_id = subscriptions_with_discarded.user_id
		)
		from subscriptions_with_discarded
		where id = $1
	`, subscriptionId)
	var userId models.UserId
	var productUserId models.ProductUserId
	var blogBestUrl string
	var isUserBounced int
	err = row.Scan(&userId, &productUserId, &blogBestUrl, &isUserBounced)
	if errors.Is(err, pgx.ErrNoRows) {
		logger.Info().Msgf("Subscription not found")
		return nil
	} else if err != nil {
		return err
	}

	if isUserBounced > 0 {
		logger.Info().Msgf("User already marked as bounced, no need to process")
		return nil
	}

	switch bounce.BounceType {
	case "Subscribe", "AutoResponder", "OpenRelayTest":
		logger.Info().Msgf("Bounce is noise (%s), skipping", bounce.BounceType)
		return nil
	case "Transient", "DnsError", "SoftBounce", "Undeliverable":
		logger.Info().Msgf("Soft bounce (%s)", bounce.BounceType)

		attemptCount, err := models.PostmarkMessage_GetAttemptCount(
			conn, messageType, subscriptionId, maybeSubscriptionPostId,
		)
		if err != nil {
			return err
		}

		waitTime, ok := map[int]time.Duration{
			1: 5 * time.Minute,
			2: 15 * time.Minute,
			3: 40 * time.Minute,
			4: 2 * time.Hour,
			5: 3 * time.Hour,
			6: 6 * time.Hour,
		}[attemptCount]
		if !ok {
			logger.Error().Msgf("Soft bounce after %d attempts, handling as hard bounce", attemptCount)
			err := markUserBounced(conn, subscriptionId, userId, productUserId, blogBestUrl, *bounce)
			return err
		}

		runAt := schedule.UTCNow().Add(waitTime)
		err = RetryBouncedEmailJob_PerformAt(conn, runAt, messageId)
		if err != nil {
			return err
		}
	case "SpamNotification", "SpamComplaint", "ChallengeVerification":
		logger.Error().Msgf("Spam complaint (%s), handling as hard bounce", bounce.BounceType)
		err := markUserBounced(conn, subscriptionId, userId, productUserId, blogBestUrl, *bounce)
		if err != nil {
			return err
		}
	default:
		logger.Error().Msgf("Hard bounce (%s)", bounce.BounceType)
		err := markUserBounced(conn, subscriptionId, userId, productUserId, blogBestUrl, *bounce)
		if err != nil {
			return err
		}
	}

	return nil
}

func markUserBounced(
	conn *pgw.Conn, subscriptionId models.SubscriptionId, userId models.UserId,
	productUserId models.ProductUserId, blogBestUrl string, bounce models.PostmarkBounce,
) error {
	logger := conn.Logger()
	err := util.Tx(conn, func(tx *pgw.Tx, conn util.Clobber) error {
		exists, err := models.PostmarkBouncedUser_Exists(tx, userId)
		if err != nil {
			return err
		}

		if exists {
			logger.Info().Msgf("User %d already marked as bounced, nothing to do", userId)
		} else {
			logger.Info().Msgf("Marking user %d as bounced", userId)
			err = models.PostmarkBouncedUser_Create(tx, userId, bounce.Id)
			if err != nil {
				return err
			}

			err = models.ProductEvent_Emit(
				tx, productUserId, "hard bounce email", map[string]any{
					"subscription_id": subscriptionId,
					"blog_url":        blogBestUrl,
				}, nil,
			)
			if err != nil {
				return err
			}
		}

		return nil
	})

	return err
}
