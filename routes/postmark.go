package routes

import (
	"feedrewind/config"
	"feedrewind/jobs"
	"feedrewind/log"
	"feedrewind/models"
	"feedrewind/oops"
	"feedrewind/routes/rutil"
	"feedrewind/util"
	"io"
	"net/http"

	"github.com/goccy/go-json"
	"github.com/mrz1836/postmark"
)

func Postmark_ReportBounce(w http.ResponseWriter, r *http.Request) {
	webhookSecret := r.Header.Get("webhook-secret")
	if webhookSecret != config.Cfg.PostmarkWebhookSecret {
		panic(oops.Newf("Webhook secret not matching: %s", webhookSecret))
	}

	bounceStr, err := io.ReadAll(r.Body)
	if err != nil {
		panic(err)
	}

	var bounce postmark.Bounce
	err = json.Unmarshal(bounceStr, &bounce)
	if err != nil {
		panic(err)
	}

	conn := rutil.DBConn(r)
	tx, err := conn.Begin()
	if err != nil {
		panic(err)
	}
	defer util.CommitOrRollbackOnPanic(tx)

	exists, err := models.PostmarkBounce_Exists(tx, bounce.ID)
	if err != nil {
		panic(err)
	}

	if exists {
		log.Info().Msgf("Bounce already seen: %d", bounce.ID)
	} else {
		log.Warn().Msgf("New bounce: %d", bounce.ID)
		err := models.PostmarkBounce_Create(tx, bounce, string(bounceStr))
		if err != nil {
			panic(err)
		}
		err = jobs.ProcessPostmarkBounceJob_PerformNow(tx, bounce.ID)
		if err != nil {
			panic(err)
		}
	}
}
