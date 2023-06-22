package routes

import (
	"feedrewind/routes/rutil"
	"fmt"
	"net/http"
)

func Dashboard(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Dash")
	currentUser := rutil.CurrentUser(r)
	if currentUser != nil {
		_, err := w.Write([]byte("You are " + currentUser.Email))
		if err != nil {
			panic(err)
		}
	} else {
		_, err := w.Write([]byte("Anon, you are seen!"))
		if err != nil {
			panic(err)
		}
	}
}
