package util

import (
	"html/template"
	"time"

	"github.com/goccy/go-json"
)

type DayOfWeek string

var DaysOfWeek = []DayOfWeek{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}
var DaysOfWeekJson template.JS

func init() {
	daysOfWeekJsonBytes, err := json.Marshal(DaysOfWeek)
	if err != nil {
		panic(err)
	}
	DaysOfWeekJson = template.JS(string(daysOfWeekJsonBytes))
}

type Date string

func Schedule_Date(time time.Time) Date {
	return Date(time.Format("2006-01-02"))
}

func Schedule_MustDateInLocation(date Date, location *time.Location) time.Time {
	time, err := time.ParseInLocation("2006-01-02", string(date), location)
	if err != nil {
		panic(err)
	}

	return time
}

func Schedule_IsEarlyMorning(localTime time.Time) bool {
	return localTime.Hour() < 5
}

func Schedule_MustUTCStr(time time.Time) string {
	if time.Location() != nil {
		panic("Expected UTC time")
	}

	return time.Format("2006-01-02 15:04:05")
}
