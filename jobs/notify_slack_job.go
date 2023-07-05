package jobs

import (
	"feedrewind/db/pgw"
	"strings"
)

func NotifySlackJob_MustPerformNow(tx pgw.Queryable, text string) {
	mustPerformNow(tx, "NotifySlackJob", defaultQueue, strToYaml(text))
}

func NotifySlackJob_Escape(text string) string {
	text = strings.ReplaceAll(text, "&", "&amp;")
	text = strings.ReplaceAll(text, "<", "&lt;")
	text = strings.ReplaceAll(text, ">", "&gt;")
	return text
}
