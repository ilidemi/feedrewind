package jobs

import "feedrewind/db/pgw"

func ProcessPostmarkBounceJob_PerformNow(tx pgw.Queryable, bounceId int64) error {
	return performNow(tx, "ProcessPostmarkBounceJob", defaultQueue, int64ToYaml(bounceId))
}
