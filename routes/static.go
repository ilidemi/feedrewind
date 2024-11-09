package routes

import (
	"net/http"

	"feedrewind.com/models"
	"feedrewind.com/util"
)

func Static_File(w http.ResponseWriter, r *http.Request) {
	static_Path(w, r, r.URL.Path)
}

func Static_RobotsTxt(w http.ResponseWriter, r *http.Request) {
	hashedPath, err := util.StaticHashedPath("robots.txt")
	if err != nil {
		panic(err)
	}
	static_Path(w, r, hashedPath)
}

func static_Path(w http.ResponseWriter, r *http.Request, path string) {
	models.ProductEvent_QueueDummyEmit(r, false, "static asset", map[string]any{
		"path": path,
	})

	staticFile, err := util.GetStaticFile(path)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", staticFile.ContentType)
	w.Header().Set("Last-Modified", staticFile.LastModified)
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(staticFile.Content)
	if err != nil {
		panic(err)
	}
}
