package util

import (
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var DaysOfWeek = []string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}
var DaysOfWeekCapitalized []string

func init() {
	caser := cases.Title(language.AmericanEnglish)
	for _, day := range DaysOfWeek {
		DaysOfWeekCapitalized = append(DaysOfWeekCapitalized, caser.String(day))
	}
}

func Schedule_DateStr(date time.Time) string {
	return date.Format("2006-01-02")
}

func Schedule_MustUTCStr(time time.Time) string {
	if time.Location() != nil {
		panic("Expected UTC time")
	}

	return time.Format("2006-01-02 15:04:05")
}
