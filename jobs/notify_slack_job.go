package jobs

import (
	"feedrewind/db/pgw"
	"strings"
)

func NotifySlackJob_MustPerformLater(tx pgw.Queryable, text string) {
	mustPerformLater(tx, "NotifySlackJob", defaultQueue, strToYaml(text))
}

func NotifySlackJob_Escape(text string) string {
	text = strings.ReplaceAll(text, "&", "&amp;")
	text = strings.ReplaceAll(text, "<", "&lt;")
	text = strings.ReplaceAll(text, ">", "&gt;")
	return text
}
