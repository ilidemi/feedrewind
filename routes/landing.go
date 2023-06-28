package routes

import (
	"feedrewind/db"
	"feedrewind/models"
	"feedrewind/routes/rutil"
	"feedrewind/templates"
	"feedrewind/util"
	"net/http"
)

func LandingIndex(w http.ResponseWriter, r *http.Request) {
	if rutil.CurrentUser(r) != nil {
		http.Redirect(w, r, "/subscriptions", http.StatusFound)
		return
	}

	models.ProductEvent_MustEmitAddPage(db.Conn, r, rutil.CurrentProductUserId(r), "/", true)

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
	type landingIndexResult struct {
		Session     *util.Session
		Screenshot  screenshot
		Suggestions suggestions
	}

	result := landingIndexResult{
		Session: rutil.Session(r),
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
	}

	templates.MustWrite(w, "landing/index", result)
}
