package jobs

import (
	"context"
	"feedrewind/db/pgw"
	"strings"
)

func NotifySlackJob_MustPerformNow(ctx context.Context, tx pgw.Queryable, text string) {
	mustPerformNow(ctx, tx, "NotifySlackJob", defaultQueue, strToYaml(text))
}

func NotifySlackJob_Escape(text string) string {
	text = strings.ReplaceAll(text, "&", "&amp;")
	text = strings.ReplaceAll(text, "<", "&lt;")
	text = strings.ReplaceAll(text, ">", "&gt;")
	return text
}
