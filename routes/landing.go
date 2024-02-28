package routes

import (
	"feedrewind/models"
	"feedrewind/routes/rutil"
	"feedrewind/templates"
	"feedrewind/util"
	"feedrewind/util/schedule"
	"net/http"
)

func Landing_Index(w http.ResponseWriter, r *http.Request) {
	if rutil.CurrentUser(r) != nil {
		http.Redirect(w, r, "/subscriptions", http.StatusFound)
		return
	}

	conn := rutil.DBConn(r)
	pc := models.NewProductEventContext(conn, r, rutil.CurrentProductUserId(r))
	models.ProductEvent_MustEmitVisitAddPage(pc, "/", true, nil)

	type ScheduleCell struct {
		IsAdd      bool
		IsSelected bool
	}
	type Screenshot struct {
		Links           []util.ScreenshotLink
		LinksCount      int
		DaysOfWeek      []schedule.DayOfWeek
		ScheduleColumns [][]ScheduleCell
	}
	type LandingResult struct {
		Session     *util.Session
		Screenshot  Screenshot
		Suggestions util.Suggestions
	}
	templates.MustWrite(w, "landing/index", LandingResult{
		Session: rutil.Session(r),
		Screenshot: Screenshot{
			Links:      util.ScreenshotLinks,
			LinksCount: len(util.ScreenshotLinks),
			DaysOfWeek: schedule.DaysOfWeek,
			ScheduleColumns: [][]ScheduleCell{
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
		Suggestions: util.Suggestions{
			Session:             rutil.Session(r),
			SuggestedCategories: util.SuggestedCategories,
			MiscellaneousBlogs:  util.MiscellaneousBlogs,
			WidthClass:          "max-w-[531px]",
		},
	})
}
