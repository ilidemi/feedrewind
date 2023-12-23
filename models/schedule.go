package models

import (
	"bytes"
	"feedrewind/db/pgw"
	"feedrewind/util/schedule"
	"fmt"
	"strings"
)

func Schedule_Create(
	tx pgw.Queryable, subscriptionId SubscriptionId, countsByDay map[schedule.DayOfWeek]int,
) error {
	var valuesSql strings.Builder
	for dayOfWeek, count := range countsByDay {
		if valuesSql.Len() > 0 {
			fmt.Fprint(&valuesSql, ", ")
		}
		fmt.Fprintf(&valuesSql, "(%d, '%s', %d)", subscriptionId, dayOfWeek, count)
	}
	_, err := tx.Exec(`
		insert into schedules (subscription_id, day_of_week, count)
		values ` + valuesSql.String() + `
	`)
	return err
}

func Schedule_GetCount(
	tx pgw.Queryable, subscriptionId SubscriptionId, dayOfWeek schedule.DayOfWeek,
) (int, error) {
	row := tx.QueryRow(`
		select count from schedules where subscription_id = $1 and day_of_week = $2
	`, subscriptionId, dayOfWeek)
	var result int
	err := row.Scan(&result)
	return result, err
}

func Schedule_GetCountsByDay(
	tx pgw.Queryable, subscriptionId SubscriptionId,
) (map[schedule.DayOfWeek]int, error) {
	rows, err := tx.Query(`
		select day_of_week, count
		from schedules
		where subscription_id = $1
	`, subscriptionId)
	if err != nil {
		return nil, err
	}

	countByDay := make(map[schedule.DayOfWeek]int)
	for rows.Next() {
		var day schedule.DayOfWeek
		var count int
		err := rows.Scan(&day, &count)
		if err != nil {
			return nil, err
		}

		countByDay[day] = count
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return countByDay, nil
}

func Schedule_Update(
	tx pgw.Queryable, subscriptionId SubscriptionId, countsByDay map[schedule.DayOfWeek]int,
) error {
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

	_, err := tx.Exec(query, subscriptionId)
	return err
}
