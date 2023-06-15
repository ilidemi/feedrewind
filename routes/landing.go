package routes

import (
	"bytes"
	rutil "feedrewind/routes/util"
	"feedrewind/templates"
	"feedrewind/util"
	"net/http"
)

func LandingIndex(w http.ResponseWriter, r *http.Request) {
	type scheduleCell struct {
		IsAdd      bool
		IsSelected bool
	}
	type screenshot struct {
		Links           []rutil.ScreenshotLink
		LinksCount      int
		DaysOfWeek      []string
		ScheduleColumns [][]scheduleCell
	}
	type suggestions struct {
		SuggestedCategories []rutil.SuggestedCategory
		MiscellaneousBlogs  []rutil.MiscellaneousBlog
		WidthClass          string
	}
	type result struct {
		Screenshot  screenshot
		Suggestions suggestions
	}

	var buf bytes.Buffer
	err := templates.Templates.ExecuteTemplate(&buf, "index.gohtml", result{
		Screenshot: screenshot{
			Links:      rutil.ScreenshotLinks,
			LinksCount: len(rutil.ScreenshotLinks),
			DaysOfWeek: util.DaysOfWeekCapitalized,
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
			SuggestedCategories: rutil.SuggestedCategories,
			MiscellaneousBlogs:  rutil.MiscellaneousBlogs,
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
