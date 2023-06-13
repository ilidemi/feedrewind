package routes

import (
	"bytes"
	"feedrewind/helpers"
	rhelpers "feedrewind/routes/helpers"
	"feedrewind/templates"
	"net/http"
)

func LandingIndex(w http.ResponseWriter, r *http.Request) {
	type scheduleCell struct {
		IsAdd      bool
		IsSelected bool
	}
	type screenshot struct {
		Links           []rhelpers.ScreenshotLink
		LinksCount      int
		DaysOfWeek      []string
		ScheduleColumns [][]scheduleCell
	}
	type suggestions struct {
		SuggestedCategories []rhelpers.SuggestedCategory
		MiscellaneousBlogs  []rhelpers.MiscellaneousBlog
		WidthClass          string
	}
	type result struct {
		Screenshot  screenshot
		Suggestions suggestions
	}

	var buf bytes.Buffer
	err := templates.Templates.ExecuteTemplate(&buf, "index.gohtml", result{
		Screenshot: screenshot{
			Links:      rhelpers.ScreenshotLinks,
			LinksCount: len(rhelpers.ScreenshotLinks),
			DaysOfWeek: helpers.DaysOfWeekCapitalized,
			ScheduleColumns: [][]scheduleCell{
				{
					{IsAdd: true},
				},
				{
					{IsAdd: true},
					{IsSelected: true},
				},
				{
					{IsAdd: true},
				},
				{
					{IsAdd: true},
					{IsSelected: true},
				},
				{
					{IsAdd: true},
				},
				{
					{IsAdd: true},
					{IsSelected: true},
				},
				{
					{IsAdd: true},
				},
			},
		},
		Suggestions: suggestions{
			SuggestedCategories: rhelpers.SuggestedCategories,
			MiscellaneousBlogs:  rhelpers.MiscellaneousBlogs,
			WidthClass:          "max-w-[531px]",
		},
	})
	if err != nil {
		panic(err)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, err = buf.WriteTo(w)
	if err != nil {
		panic(err)
	}
}
