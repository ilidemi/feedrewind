package jobs

import (
	"feedrewind/db/pgw"
	"feedrewind/models"
	"feedrewind/oops"

	"github.com/goccy/go-json"
)

type GuidedCrawlingJobArgs struct {
	StartFeedId models.StartFeedId `json:"start_feed_id"`
}

func GuidedCrawlingJob_PerformNow(
	tx pgw.Queryable, blogId models.BlogId, startFeedId models.StartFeedId,
) error {
	args := GuidedCrawlingJobArgs{
		StartFeedId: startFeedId,
	}
	argsJson, err := json.Marshal(&args)
	if err != nil {
		return oops.Wrap(err)
	}
	return performNow(
		tx, "GuidedCrawlingJob", defaultQueue, int64ToYaml(int64(blogId)), strToYaml(string(argsJson)),
	)
}
