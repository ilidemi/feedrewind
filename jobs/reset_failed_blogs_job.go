package jobs

import "feedrewind/db/pgw"

func ResetFailedBlogsJob_PerformNow(tx pgw.Queryable, enqueueNext bool) error {
	return performNow(tx, "ResetFailedBlogsJob", "reset_failed_blogs", boolToYaml(enqueueNext))
}
