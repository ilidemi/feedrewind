package util

import (
	"errors"
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

func Schedule_DateInLocation(date Date, location *time.Location) (time.Time, error) {
	parsed, err := time.ParseInLocation("2006-01-02", string(date), location)
	if err != nil {
		return time.Time{}, err //nolint:exhaustruct
	}

	return parsed, nil
}

func Schedule_IsEarlyMorning(localTime time.Time) bool {
	return localTime.Hour() < 5
}

func Schedule_ToUTCStr(time time.Time) (string, error) {
	if time.Location() != nil {
		return "", errors.New("Expected UTC time")
	}

	return time.Format("2006-01-02 15:04:05"), nil
}
