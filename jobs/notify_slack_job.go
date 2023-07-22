package jobs

import (
	"feedrewind/db/pgw"
	"strings"
)

func NotifySlackJob_PerformNow(tx pgw.Queryable, text string) error {
	return performNow(tx, "NotifySlackJob", defaultQueue, strToYaml(text))
}

func NotifySlackJob_Escape(text string) string {
	text = strings.ReplaceAll(text, "&", "&amp;")
	text = strings.ReplaceAll(text, "<", "&lt;")
	text = strings.ReplaceAll(text, ">", "&gt;")
	return text
}
