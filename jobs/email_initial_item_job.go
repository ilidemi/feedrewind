package jobs

import (
	"feedrewind/db/pgw"
	"feedrewind/models"
)

func EmailInitialItemJob_PerformNow(
	tx pgw.Queryable, userId models.UserId, subscriptionId models.SubscriptionId, scheduledFor string,
) error {
	return performNow(
		tx, "EmailInitialItemJob", defaultQueue, int64ToYaml(int64(userId)),
		int64ToYaml(int64(subscriptionId)), strToYaml(scheduledFor),
	)
}
