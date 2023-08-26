package jobs

import (
	"feedrewind/db/pgw"
	"feedrewind/models"
	"feedrewind/util"
)

func EmailPostsJob_PerformNow(
	tx pgw.Queryable, userId models.UserId, date util.Date, scheduledFor string,
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
