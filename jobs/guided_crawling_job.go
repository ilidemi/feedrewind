package jobs

import (
	"feedrewind/db/pgw"
	"feedrewind/models"

	"github.com/goccy/go-json"
)

type GuidedCrawlingJobArgs struct {
	StartFeedId models.StartFeedId `json:"start_feed_id"`
}

func GuidedCrawlingJob_MustPerformNow(
	tx pgw.Queryable, blogId models.BlogId, startFeedId models.StartFeedId,
) {
	args := GuidedCrawlingJobArgs{
		StartFeedId: startFeedId,
	}
	argsJson, err := json.Marshal(&args)
	if err != nil {
		panic(err)
	}
	mustPerformNow(
		tx, "GuidedCrawlingJob", defaultQueue, int64ToYaml(int64(blogId)), strToYaml(string(argsJson)),
	)
}
