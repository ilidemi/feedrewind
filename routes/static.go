package routes

import (
	"feedrewind/models"
	"feedrewind/routes/rutil"
	"feedrewind/util"
	"net/http"
)

func Static_File(w http.ResponseWriter, r *http.Request) {
	logger := rutil.Logger(r)
	pool := rutil.DBPool(r)
	models.ProductEvent_DummyEmitOrLog(pool, r, false, "static asset", map[string]any{
		"path": r.URL.Path,
	}, logger)

	staticFile, err := util.GetStaticFile(r.URL.Path)
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
