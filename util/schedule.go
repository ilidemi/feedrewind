package util

import (
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
