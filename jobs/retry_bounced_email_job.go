package jobs

import (
	"context"
	"errors"
	"feedrewind/db/pgw"
	"feedrewind/models"
	"feedrewind/oops"
	"feedrewind/util/schedule"

	"github.com/jackc/pgx/v5"
	"github.com/mrz1836/postmark"
)

func init() {
	registerJobNameFunc("RetryBouncedEmailJob", func(ctx context.Context, conn *pgw.Conn, args []any) error {
		if len(args) != 1 {
			return oops.Newf("Expected 1 arg, got %d: %v", len(args), args)
		}

		messageIdStr, ok := args[0].(string)
		if !ok {
			return oops.Newf("Failed to parse messageId (expected string): %v", args[0])
		}
		messageId := models.PostmarkMessageId(messageIdStr)

		return RetryBouncedEmailJob_Perform(ctx, conn, messageId)
	})
}

func RetryBouncedEmailJob_PerformAt(
	tx pgw.Queryable, runAt schedule.Time, messageId models.PostmarkMessageId,
) error {
	return performAt(tx, runAt, "RetryBouncedEmailJob", defaultQueue, strToYaml(string(messageId)))
}

func RetryBouncedEmailJob_Perform(
	ctx context.Context, conn *pgw.Conn, messageId models.PostmarkMessageId,
) error {
	logger := conn.Logger()
	row := conn.QueryRow(`
		select message_type, subscription_id, subscription_post_id
		from postmark_messages
		where message_id = $1
	`, messageId)
	var messageType models.PostmarkMessageType
	var subscriptionId models.SubscriptionId
	var maybeSubscriptionPostId *models.SubscriptionPostId
	err := row.Scan(&messageType, &subscriptionId, &maybeSubscriptionPostId)
	if err != nil {
		return err
	}

	row = conn.QueryRow(`
		select name, user_id, (
			select email from users_with_discarded
			where users_with_discarded.id = subscriptions_without_discarded.user_id
		)
		from subscriptions_without_discarded
		where id = $1
	`, subscriptionId)
	var subscriptionName string
	var userId models.UserId
	var userEmail string
	err = row.Scan(&subscriptionName, &userId, &userEmail)
	if errors.Is(err, pgx.ErrNoRows) {
		logger.Info().Msgf("Subscription %d was deleted", subscriptionId)
		return nil
	} else if err != nil {
		return err
	}

	client, maybeTestMetadata := GetPostmarkClientAndMaybeMetadata(conn)
	scheduledFor := schedule.UTCNow().MustUTCString()
	logger.Info().Msgf("Retrying message: %s", messageId)

	var email postmark.Email
	switch messageType {
	case models.PostmarkMessageSubInitial:
		email = newInitialEmail(
			userId, userEmail, subscriptionId, subscriptionName, maybeTestMetadata, scheduledFor,
		)
	case models.PostmarkMessageSubFinal:
		email = newFinalEmail(
			userId, userEmail, subscriptionId, subscriptionName, maybeTestMetadata, scheduledFor,
		)
	case models.PostmarkMessageSubPost:
		row := conn.QueryRow(`
			select random_id, (
				select title from blog_posts where blog_posts.id = subscription_posts.blog_post_id
			)
			from subscription_posts
			where id = $1
		`, *maybeSubscriptionPostId)
		var postRandomId models.SubscriptionPostRandomId
		var postTitle string
		err := row.Scan(&postRandomId, &postTitle)
		if err != nil {
			return err
		}

		email = newPostEmail(
			userId, userEmail, subscriptionId, subscriptionName, *maybeSubscriptionPostId, postTitle,
			postRandomId, maybeTestMetadata, scheduledFor,
		)
	default:
		return oops.Newf("Unexpected message type: %s", messageType)
	}

	response, err := client.SendEmail(ctx, email)
	if err != nil {
		return oops.Wrap(err)
	}

	logger.
		Info().
		Any("metadata", email.Metadata).
		Any("response", response).
		Msg("Sent email")
	_, err = conn.Exec(`
		insert into postmark_messages (message_id, message_type, subscription_id, subscription_post_id)
		values ($1, $2, $3, $4)
	`, response.MessageID, messageType, subscriptionId, maybeSubscriptionPostId)
	if err != nil {
		return err
	}

	return nil
}
