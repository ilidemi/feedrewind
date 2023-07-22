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
	blogName, blogStatus, err := models.Blog_GetNameStatusById(conn, blogId)
	if err != nil {
		panic(err)
	}
	if !(blogStatus == models.BlogStatusCrawlFailed || blogStatus == models.BlogStatusCrawledLooksWrong) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	type unsupportedResult struct {
		Session  *util.Session
		BlogName string
	}
	result := unsupportedResult{
		Session:  rutil.Session(r),
		BlogName: blogName,
	}
	templates.MustWrite(w, "blogs/unsupported", result)
}
