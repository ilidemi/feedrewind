package routes

import (
	"feedrewind/config"
	"feedrewind/jobs"
	"feedrewind/models"
	"feedrewind/oops"
	"feedrewind/routes/rutil"
	"feedrewind/util"
	"io"
	"net/http"

	"github.com/goccy/go-json"
	"github.com/mrz1836/postmark"
	"github.com/pkg/errors"
	"github.com/stripe/stripe-go/v78"
	"github.com/stripe/stripe-go/v78/webhook"
)

func Webhooks_PostmarkReportBounce(w http.ResponseWriter, r *http.Request) {
	logger := rutil.Logger(r)
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

	pool := rutil.DBPool(r)
	tx, err := pool.Begin()
	if err != nil {
		panic(err)
	}
	defer util.CommitOrRollbackOnPanic(tx)

	exists, err := models.PostmarkBounce_Exists(tx, bounce.ID)
	if err != nil {
		panic(err)
	}

	if exists {
		logger.Info().Msgf("Bounce already seen: %d", bounce.ID)
	} else {
		logger.Warn().Msgf("New bounce: %d", bounce.ID)
		err := models.PostmarkBounce_CreateIfNotExists(tx, bounce)
		if err != nil {
			panic(err)
		}
		err = jobs.ProcessPostmarkBounceJob_PerformNow(tx, bounce.ID)
		if err != nil {
			panic(err)
		}
	}
}

func Webhooks_Stripe(w http.ResponseWriter, r *http.Request) {
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		panic(err)
	}

	signatureHeader := r.Header.Get("Stripe-Signature")
	event, err := webhook.ConstructEvent(payload, signatureHeader, config.Cfg.StripeWebhookSecret)
	if err != nil {
		panic(errors.Wrapf(err, "Invalid webhook signature: %q", signatureHeader))
	}

	logger := rutil.Logger(r)
	logger.Info().Msgf(
		"Event:%s id:%s object:%v", string(event.Type), event.ID, event.Data.Object["id"],
	)
	switch event.Type {
	case stripe.EventTypeCustomerSubscriptionCreated,
		stripe.EventTypeCustomerSubscriptionUpdated,
		stripe.EventTypeCustomerSubscriptionDeleted,
		stripe.EventTypeCheckoutSessionCompleted,
		stripe.EventTypeInvoiceCreated,
		stripe.EventTypeInvoicePaid:

		pool := rutil.DBPool(r)
		_, err := pool.Exec(`
			insert into stripe_webhook_events (id, payload) values ($1, $2)
		`, event.ID, payload)
		if util.ViolatesUnique(err, "stripe_webhook_events_pkey") {
			logger.Info().Msgf("Duplicate event: %s", event.ID)
			break
		} else if err != nil {
			panic(err)
		}
		err = jobs.StripeWebhookJob_PerformNow(pool, event.ID)
		if err != nil {
			panic(err)
		}
	}
}
