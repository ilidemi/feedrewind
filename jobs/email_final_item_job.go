package jobs

import (
	"context"
	"errors"

	"feedrewind.com/db/pgw"
	"feedrewind.com/models"
	"feedrewind.com/oops"
	"feedrewind.com/util"
	"feedrewind.com/util/schedule"

	"github.com/jackc/pgx/v5"
)

func init() {
	registerJobNameFunc(
		"EmailFinalItemJob",
		func(ctx context.Context, id JobId, pool *pgw.Pool, args []any) error {
			if len(args) != 3 {
				return oops.Newf("Expected 3 args, got %d: %v", len(args), args)
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

			subscriptionIdInt64, ok := args[1].(int64)
			if !ok {
				subscriptionIdInt, ok := args[1].(int)
				if !ok {
					return oops.Newf("Failed to parse subscriptionId (expected int64 or int): %v", args[1])
				}
				subscriptionIdInt64 = int64(subscriptionIdInt)
			}
			subscriptionId := models.SubscriptionId(subscriptionIdInt64)

			scheduledFor, ok := args[2].(string)
			if !ok {
				return oops.Newf("Failed to parse scheduledFor (expected string): %v", args[2])
			}

			return EmailFinalItemJob_Perform(ctx, pool, userId, subscriptionId, scheduledFor)
		},
	)
}

func EmailFinalItemJob_PerformAt(
	qu pgw.Queryable, runAt schedule.Time, userId models.UserId, subscriptionId models.SubscriptionId,
	scheduledFor string,
) error {
	return performAt(
		qu, runAt, "EmailFinalItemJob", defaultQueue, int64ToYaml(int64(userId)),
		int64ToYaml(int64(subscriptionId)), strToYaml(scheduledFor),
	)
}

func EmailFinalItemJob_Perform(
	ctx context.Context, pool *pgw.Pool, userId models.UserId, subscriptionId models.SubscriptionId,
	scheduledFor string,
) error {
	return util.Tx(pool, func(tx *pgw.Tx, pool util.Clobber) error {
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

		row := tx.QueryRow(`
			select name, final_item_publish_status, (select email from users_with_discarded where id = $1)
			from subscriptions_without_discarded
			where id = $2
		`, userId, subscriptionId)
		var name string
		var finalItemPublishStatus models.PostPublishStatus
		var userEmail string
		err = row.Scan(&name, &finalItemPublishStatus, &userEmail)
		if errors.Is(err, pgx.ErrNoRows) {
			logger.Info().Msgf("Subscription %d not found", subscriptionId)
			return nil
		} else if err != nil {
			return err
		}

		if finalItemPublishStatus != models.PostPublishStatusEmailPending {
			logger.Warn().Msg("Final email already sent? Nothing to do")
			return nil
		}

		logger.Info().Msg("Sending final email")
		client, maybeTestMetadata := GetPostmarkClientAndMaybeMetadata(tx)
		finalEmail := newFinalEmail(
			userId, userEmail, subscriptionId, name, maybeTestMetadata, scheduledFor,
		)
		response, err := client.SendEmail(ctx, finalEmail)
		if err != nil {
			return oops.Wrapf(err, "Error sending email %v", finalEmail.Metadata)
		}

		logger.
			Info().
			Any("metadata", finalEmail.Metadata).
			Any("response", response).
			Msg("Final email sent")
		_, err = tx.Exec(`
			insert into postmark_messages (message_id, message_type, subscription_id, subscription_post_id)
			values ($1, 'sub_final', $2, null)
		`, response.MessageID, subscriptionId)
		if err != nil {
			return err
		}

		_, err = tx.Exec(`
			update subscriptions_without_discarded
			set final_item_publish_status = 'email_sent'
			where id = $1
		`, subscriptionId)
		if err != nil {
			return err
		}

		return nil
	})
}
