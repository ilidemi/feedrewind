package routes

import (
	"net/http"

	"feedrewind.com/models"
	"feedrewind.com/routes/rutil"
	"feedrewind.com/templates"
	"feedrewind.com/util"
	"feedrewind.com/util/schedule"
)

func Landing_Index(w http.ResponseWriter, r *http.Request) {
	if rutil.CurrentUser(r) != nil {
		http.Redirect(w, r, "/subscriptions", http.StatusSeeOther)
		return
	}

	pool := rutil.DBPool(r)
	pc := models.NewProductEventContext(pool, r, rutil.CurrentProductUserId(r))
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
			//nolint:exhaustruct
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
			IsPlayful:           true,
		},
	})
}

func Landing_HMN(w http.ResponseWriter, r *http.Request) {
	type HmnResult struct {
		Session      *util.Session
		Categories   []util.HmnCategory
		TryItOutPath string
	}
	tryItOutPath := "/#try_it_out"
	if rutil.CurrentUser(r) != nil {
		tryItOutPath = "/subscriptions/add"
	}
	templates.MustWrite(w, "landing/hmn", HmnResult{
		Session:      rutil.Session(r),
		Categories:   util.HmnCategories,
		TryItOutPath: tryItOutPath,
	})
}
