package routes

import (
	"feedrewind/util"
	"net/http"
)

func Static_File(w http.ResponseWriter, r *http.Request) {
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
