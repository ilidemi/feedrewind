package main

import (
	"feedrewind/log"
	frmiddleware "feedrewind/middleware"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

func main() {
	r := chi.NewRouter()
	r.Use(frmiddleware.Logger)
	r.Use(frmiddleware.Recoverer)
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		_, err := w.Write([]byte("FeedRewind"))
		if err != nil {
			panic(err)
		}
	})
	log.Info().Msg("Started")
	if err := http.ListenAndServe(":3000", r); err != nil {
		panic(err)
	}

}
