package models

import (
	"bytes"
	"feedrewind/db/pgw"
	"feedrewind/log"
	"feedrewind/util"
	"fmt"
)

func MustInit(tx pgw.Queryable) {
	r := tx.Request()
	var timezoneInExpr bytes.Buffer
	timezoneInExpr.WriteString("('")
	isFirst := true
	for groupId := range util.GroupIdByTimezoneId {
		if isFirst {
			isFirst = false
		} else {
			timezoneInExpr.WriteString("', '")
		}
		timezoneInExpr.WriteString(groupId)
	}
	for groupId := range util.UnfriendlyGroupIds {
		timezoneInExpr.WriteString("', '")
		timezoneInExpr.WriteString(groupId)
	}
	timezoneInExpr.WriteString("')")
	query := fmt.Sprintf(
		"select user_id, timezone from user_settings where timezone not in %s", timezoneInExpr.String(),
	)

	log.Info(r).Msg("Ensuring user timezones")
	rows, err := tx.Query(query)
	if err != nil {
		panic(err)
	}
	for rows.Next() {
		var userId UserId
		var timezone string
		err := rows.Scan(&userId, &timezone)
		if err != nil {
			panic(err)
		}
		log.Warn(r).Msgf("User timezone not found in tzdb: %s (user %d)", timezone, userId)
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}
}
