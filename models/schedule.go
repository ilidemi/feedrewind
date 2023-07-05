package models

import (
	"bytes"
	"feedrewind/db/pgw"
	"feedrewind/util"
	"fmt"
)

func Schedule_MustGetCounts(tx pgw.Queryable, subscriptionId SubscriptionId) map[util.DayOfWeek]int {
	rows, err := tx.Query(`
		select day_of_week, count
		from schedules
		where subscription_id = $1
	`, subscriptionId)
	if err != nil {
		panic(err)
	}

	countByDay := make(map[util.DayOfWeek]int)
	for rows.Next() {
		var day util.DayOfWeek
		var count int
		err := rows.Scan(&day, &count)
		if err != nil {
			panic(err)
		}

		countByDay[day] = count
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}

	return countByDay
}

func Schedule_MustUpdate(
	tx pgw.Queryable, subscriptionId SubscriptionId, countsByDay map[util.DayOfWeek]int,
) {
	var queryBuf bytes.Buffer
	queryBuf.WriteString(`
		update schedules as s set count = n.count
		from (values
	`)
	isFirst := true
	for dayOfWeek, count := range countsByDay {
		if isFirst {
			isFirst = false
		} else {
			queryBuf.WriteString(",")
		}
		queryBuf.WriteString(fmt.Sprintf("('%s'::day_of_week, %d)", dayOfWeek, count))
	}
	queryBuf.WriteString(`
		) as n(day_of_week, count)
		where s.day_of_week = n.day_of_week and subscription_id = $1
	`)
	query := queryBuf.String()

	tx.MustExec(query, subscriptionId)
}
