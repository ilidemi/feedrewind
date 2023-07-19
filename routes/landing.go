package routes

import (
	"feedrewind/models"
	"feedrewind/routes/rutil"
	"feedrewind/templates"
	"feedrewind/util"
	"net/http"
)

func Landing_Index(w http.ResponseWriter, r *http.Request) {
	if rutil.CurrentUser(r) != nil {
		http.Redirect(w, r, "/subscriptions", http.StatusFound)
		return
	}

	conn := rutil.DBConn(r)
	models.ProductEvent_MustEmitVisitAddPage(models.ProductEventVisitAddPageArgs{
		Tx:              conn,
		Request:         r,
		ProductUserId:   rutil.CurrentProductUserId(r),
		Path:            "/",
		UserIsAnonymous: true,
		Extra:           nil,
	})

	type scheduleCell struct {
		IsAdd      bool
		IsSelected bool
	}
	type screenshot struct {
		Links           []rutil.ScreenshotLink
		LinksCount      int
		DaysOfWeek      []util.DayOfWeek
		ScheduleColumns [][]scheduleCell
	}
	type landingIndexResult struct {
		Session     *util.Session
		Screenshot  screenshot
		Suggestions rutil.Suggestions
	}

	result := landingIndexResult{
		Session: rutil.Session(r),
		Screenshot: screenshot{
			Links:      rutil.ScreenshotLinks,
			LinksCount: len(rutil.ScreenshotLinks),
			DaysOfWeek: util.DaysOfWeek,
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
		Suggestions: rutil.Suggestions{
			SuggestedCategories: rutil.SuggestedCategories,
			MiscellaneousBlogs:  rutil.MiscellaneousBlogs,
			WidthClass:          "max-w-[531px]",
		},
	}

	templates.MustWrite(w, "landing/index", result)
}
