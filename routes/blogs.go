package routes

import (
	"feedrewind/models"
	"feedrewind/routes/rutil"
	"feedrewind/templates"
	"feedrewind/util"
	"net/http"
)

func Blogs_Unsupported(w http.ResponseWriter, r *http.Request) {
	conn := rutil.DBConn(r)

	blogIdInt, ok := util.URLParamInt64(r, "id")
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	blogId := models.BlogId(blogIdInt)
	blogName, blogStatus, err := models.Blog_GetNameStatus(conn, blogId)
	if err != nil {
		panic(err)
	}
	if !models.BlogFailedStatuses[blogStatus] {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	type Result struct {
		Title    string
		Session  *util.Session
		BlogName string
	}
	templates.MustWrite(w, "blogs/unsupported", Result{
		Title:    util.DecorateTitle("Blog not supported"),
		Session:  rutil.Session(r),
		BlogName: blogName,
	})
}
