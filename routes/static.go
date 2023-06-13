package routes

import (
	"feedrewind/helpers"
	"net/http"
)

func StaticFile(w http.ResponseWriter, r *http.Request) {
	staticFile, err := helpers.GetStaticFile(r.URL.Path)
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
