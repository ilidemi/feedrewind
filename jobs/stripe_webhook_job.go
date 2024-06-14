package jobs

import (
	"context"
	"errors"
	"feedrewind/db/pgw"
	"feedrewind/models"
	"feedrewind/oops"
	"feedrewind/util"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/goccy/go-json"
	"github.com/jackc/pgx/v5"
	"github.com/stripe/stripe-go/v78"
	"github.com/stripe/stripe-go/v78/checkout/session"
	"github.com/stripe/stripe-go/v78/subscription"
)

func init() {
	registerJobNameFunc("StripeWebhookJob",
		func(ctx context.Context, conn *pgw.Conn, args []any) error {
			if len(args) != 1 {
				return oops.Newf("Expected 1 arg, got %d: %v", len(args), args)
			}

			eventId, ok := args[0].(string)
			if !ok {
				return oops.Newf("Failed to parse eventId (expected string): %v", args[0])
			}

			return StripeWebhookJob_Perform(ctx, conn, eventId)
		},
	)
}

func StripeWebhookJob_PerformNow(tx pgw.Queryable, eventId string) error {
	return performNow(tx, "StripeWebhookJob", defaultQueue, strToYaml(eventId))
}

func StripeWebhookJob_Perform(ctx context.Context, conn *pgw.Conn, eventId string) error {
	logger := conn.Logger()

	var payload []byte
	row := conn.QueryRow(`select payload from stripe_webhook_events where id = $1`, eventId)
	err := row.Scan(&payload)
	if errors.Is(err, pgx.ErrNoRows) {
		logger.Error().Msgf("Couldn't find event: %s", eventId)
		return nil
	} else if err != nil {
		return err
	}

	event := stripe.Event{} //nolint:exhaustruct
	err = json.Unmarshal(payload, &event)
	if err != nil {
		return oops.Wrap(err)
	}

	logger.Info().Msgf("Processing event: %s (%s)", string(event.Type), event.Data.Object["id"].(string))

	// When adding new cases, update the webhook handler too
	switch event.Type {
	case stripe.EventTypeCustomerSubscriptionCreated:
		var sub stripe.Subscription
		err := json.Unmarshal(event.Data.Raw, &sub)
		if err != nil {
			return oops.Wrap(err)
		}
		row := conn.QueryRow(`
			select plan_id from pricing_offers
			where stripe_monthly_price_id = $1 or stripe_yearly_price_id = $1
		`, sub.Items.Data[0].Price.ID)
		var planId models.PlanId
		err = row.Scan(&planId)
		if err != nil {
			return err
		}
		var slackMessage string
		switch planId {
		case models.PlanIdSupporter:
			slackMessage = "💰 New Supporter"
		case models.PlanIdPatron:
			slackMessage = "💰💰💰 New Patron"
		default:
			panic(fmt.Errorf("Unknown plan id: %s", planId))
		}
		err = NotifySlackJob_PerformNow(conn, slackMessage)
		if err != nil {
			return err
		}
	case stripe.EventTypeCheckoutSessionCompleted:
		var sesh stripe.CheckoutSession
		err := json.Unmarshal(event.Data.Raw, &sesh)
		if err != nil {
			return oops.Wrap(err)
		}
		userIdStr, ok := sesh.Metadata["user_id"]
		if !ok {
			break
		}
		userId, err := strconv.ParseInt(userIdStr, 10, 64)
		if err != nil {
			return oops.Wrap(err)
		}
		sub, err := subscription.Get(sesh.Subscription.ID, nil)
		if err != nil {
			return oops.Wrap(err)
		}
		err = util.Tx(conn, func(tx *pgw.Tx, conn util.Clobber) error {
			stripeProductId := sub.Items.Data[0].Price.Product.ID
			row := tx.QueryRow(`
				select id, plan_id from pricing_offers where stripe_product_id = $1
			`, stripeProductId)
			var offerId models.OfferId
			var planId models.PlanId
			err = row.Scan(&offerId, &planId)
			if err != nil {
				return err
			}
			stripePriceId := sub.Items.Data[0].Price.ID
			billingInterval, err := models.BillingInterval_GetByOffer(tx, stripeProductId, stripePriceId)
			row = tx.QueryRow(`
				update users_without_discarded
				set offer_id = $1,
					stripe_subscription_id = $2,
					stripe_customer_id = $3,
					billing_interval = $4
				where id = $5
				returning product_user_id
			`, offerId, sub.ID, sub.Customer.ID, billingInterval, userId)
			if errors.Is(err, pgx.ErrNoRows) {
				logger.Warn().Msgf("Couldn't find user: %d", userId)
				return nil
			} else if err != nil {
				return err
			}
			var productUserId models.ProductUserId
			err = row.Scan(&productUserId)
			if err != nil {
				return err
			}
			err = models.ProductEvent_Emit(tx, productUserId, "upgrade to paid", map[string]any{
				"pricing_plan":     planId,
				"pricing_offer":    offerId,
				"billing_interval": billingInterval,
			}, map[string]any{
				"pricing_plan":     planId,
				"pricing_offer":    offerId,
				"billing_interval": billingInterval,
			})
			if err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			return err
		}
	case stripe.EventTypeCustomerSubscriptionUpdated:
		var sub stripe.Subscription
		err := json.Unmarshal(event.Data.Raw, &sub)
		if err != nil {
			return oops.Wrap(err)
		}
		if sub.Status == stripe.SubscriptionStatusActive {
			err := util.Tx(conn, func(tx *pgw.Tx, conn util.Clobber) error {
				if sub.CanceledAt != 0 {
					if _, ok := getPreviousValue(&event, "canceled_at"); !ok {
						cancelAt := time.Unix(sub.CancelAt, 0).UTC()
						row := tx.QueryRow(`
							update users_with_discarded
							set stripe_cancel_at = $1 where stripe_subscription_id = $2
							returning product_user_id
						`, cancelAt, sub.ID)
						var productUserId models.ProductUserId
						err := row.Scan(&productUserId)
						if errors.Is(err, pgx.ErrNoRows) {
							logger.Warn().Msgf("Couldn't find user with stripe subscription: %s", sub.ID)
							return nil
						} else if err != nil {
							return err
						}
						err = models.ProductEvent_Emit(
							tx, productUserId, "cancel stripe subscription", nil, nil,
						)
						if err != nil {
							return err
						}

						var sb strings.Builder
						sb.WriteString("Stripe subscription canceled. Reason: ")
						sb.WriteString(string(sub.CancellationDetails.Reason))
						if sub.CancellationDetails.Feedback != "" {
							sb.WriteString(". Feedback: ")
							sb.WriteString(string(sub.CancellationDetails.Feedback))
						}
						if sub.CancellationDetails.Comment != "" {
							sb.WriteString(". Comment: ")
							sb.WriteString(NotifySlackJob_Escape(sub.CancellationDetails.Comment))
						}
						err = NotifySlackJob_PerformNow(tx, sb.String())
						if err != nil {
							return err
						}
					}
				}

				if sub.CanceledAt == 0 {
					if _, ok := getPreviousValue(&event, "canceled_at"); ok {
						row := tx.QueryRow(`
							update users_with_discarded
							set stripe_cancel_at = null
							where stripe_subscription_id = $1
							returning product_user_id
						`, sub.ID)
						var productUserId models.ProductUserId
						err := row.Scan(&productUserId)
						if errors.Is(err, pgx.ErrNoRows) {
							logger.Warn().Msgf("Couldn't find user with stripe subscription: %s", sub.ID)
							return nil
						} else if err != nil {
							return err
						}
						err = models.ProductEvent_Emit(
							tx, productUserId, "renew stripe subscription", nil, nil,
						)
						if err != nil {
							return err
						}
						err = NotifySlackJob_PerformNow(tx, "Stripe subscription renewed")
						if err != nil {
							return err
						}
					}
				}

				if _, ok := getPreviousValue(&event, "items", "data", "0", "price", "id"); ok {
					newProductId := sub.Items.Data[0].Price.Product.ID
					row := tx.QueryRow(`
						select id, plan_id from pricing_offers where stripe_product_id = $1
					`, newProductId)
					var newOfferId models.OfferId
					var newPlanId models.PlanId
					err := row.Scan(&newOfferId, &newPlanId)
					if err != nil {
						return err
					}

					row = tx.QueryRow(`
						select
							id, product_user_id, billing_interval,
							(select plan_id from pricing_offers where id = offer_id)
						from users_without_discarded
						where stripe_subscription_id = $1
					`, sub.ID)
					var userId models.UserId
					var productUserId models.ProductUserId
					var oldBillingInterval models.BillingInterval
					var oldPlanId models.PlanId
					err = row.Scan(&userId, &productUserId, &oldBillingInterval, &oldPlanId)
					if err != nil {
						return err
					}

					newInterval := sub.Items.Data[0].Price.Recurring.Interval
					var newBillingInterval models.BillingInterval
					switch newInterval {
					case stripe.PriceRecurringIntervalMonth:
						newBillingInterval = models.BillingIntervalMonthly
					case stripe.PriceRecurringIntervalYear:
						newBillingInterval = models.BillingIntervalYearly
					default:
						return oops.Newf("Unknown plan interval: %s", newInterval)
					}

					_, err = tx.Exec(`
						update users_without_discarded
						set billing_interval = $1, offer_id = $2 where id = $3
					`, newBillingInterval, newOfferId, userId)
					if err != nil {
						return err
					}

					if newPlanId != oldPlanId {
						err := models.ProductEvent_Emit(
							tx, productUserId, "update stripe subscription", map[string]any{
								"pricing_plan":  newPlanId,
								"pricing_offer": newOfferId,
							}, map[string]any{
								"pricing_plan":  newPlanId,
								"pricing_offer": newOfferId,
							},
						)
						if err != nil {
							return err
						}
						logger.Info().Msgf(
							"Updated plan from %s to %s for user %d", oldPlanId, newPlanId, userId,
						)
					}

					if newBillingInterval != oldBillingInterval {
						_, err = tx.Exec(`
							update users_without_discarded
							set billing_interval = $1 where id = $2
						`, newBillingInterval, userId)
						if err != nil {
							return err
						}
						err = models.ProductEvent_Emit(
							tx, productUserId, "update stripe subscription", map[string]any{
								"billing_interval": newBillingInterval,
							}, map[string]any{
								"billing_interval": newBillingInterval,
							},
						)
						if err != nil {
							return err
						}
						logger.Info().Msgf(
							"Updated billing interval to %s for user %d", newBillingInterval, userId,
						)
						if newPlanId == models.PlanIdPatron {
							logger.Warn().Msg(
								"Proration for patrons is not implemented, please do that promptly",
							)
						}
					}
				}

				return nil
			})
			if err != nil {
				return err
			}
		} else if sub.Status == stripe.SubscriptionStatusUnpaid {
			logger.Warn().Msgf("Stripe subscription switched to unpaid: %s", sub.ID)
		}
	case stripe.EventTypeCustomerSubscriptionDeleted:
		var sub stripe.Subscription
		err := json.Unmarshal(event.Data.Raw, &sub)
		if err != nil {
			return oops.Wrap(err)
		}
		err = util.Tx(conn, func(tx *pgw.Tx, conn util.Clobber) error {
			row := tx.QueryRow(`
				update users_with_discarded
				set billing_interval = null,
					offer_id = (select default_offer_id from pricing_plans where id = $1),
					stripe_subscription_id = null
				where stripe_subscription_id = $2
				returning id, product_user_id, offer_id, stripe_cancel_at
			`, models.PlanIdFree, sub.ID)
			var userId models.UserId
			var productUserId models.ProductUserId
			var offerId models.OfferId
			var stripeCancelAt *time.Time
			err = row.Scan(&userId, &productUserId, &offerId, &stripeCancelAt)
			if errors.Is(err, pgx.ErrNoRows) {
				logger.Warn().Msgf("Couldn't find user: %d", userId)
				return nil
			} else if err != nil {
				return err
			}
			if stripeCancelAt != nil {
				_, err := tx.Exec(`
					update users_with_discarded set stripe_cancel_at = null where id = $1
				`, userId)
				if err != nil {
					return err
				}
			} else {
				err := NotifySlackJob_PerformNow(tx, "Stripe subscription cancelled and deleted")
				if err != nil {
					return err
				}
			}

			err = models.ProductEvent_Emit(
				tx, productUserId, "delete stripe subscription", map[string]any{
					"pricing_plan":     models.PlanIdFree,
					"pricing_offer":    offerId,
					"billing_interval": nil,
				}, map[string]any{
					"pricing_plan":     models.PlanIdFree,
					"pricing_offer":    offerId,
					"billing_interval": nil,
				},
			)
			if err != nil {
				return err
			}
			logger.Info().Msgf("Deleted Stripe subscription for user %d", userId)
			return nil
		})
		if err != nil {
			return err
		}
	case stripe.EventTypeInvoiceCreated:
		var invoice stripe.Invoice
		err := json.Unmarshal(event.Data.Raw, &invoice)
		if err != nil {
			return oops.Wrap(err)
		}
		sub, err := subscription.Get(invoice.Subscription.ID, nil)
		if err != nil {
			return oops.Wrap(err)
		}
		currentPeriodEnd := time.Unix(sub.CurrentPeriodEnd, 0).UTC()
		result, err := conn.Exec(`
			update users_with_discarded set stripe_current_period_end = $1 where stripe_subscription_id = $2
		`, currentPeriodEnd, sub.ID)
		if err != nil {
			return err
		}
		if result.RowsAffected() > 0 {
			logger.Info().Msgf("Updated current period end to %v for Stripe sub %s", currentPeriodEnd, sub.ID)
		} else {
			logger.Info().Msgf(
				"Couldn't find the user for Stripe sub %s, assuming the sign-up hasn't gone through yet",
				sub.ID,
			)
		}
	case stripe.EventTypeInvoicePaid:
		var invoice stripe.Invoice
		err := json.Unmarshal(event.Data.Raw, &invoice)
		if err != nil {
			return oops.Wrap(err)
		}
		err = util.Tx(conn, func(tx *pgw.Tx, conn util.Clobber) error {
			var priceId string
			for _, lineItem := range invoice.Lines.Data {
				if lineItem.Amount > 0 {
					priceId = lineItem.Price.ID
				}
			}
			if priceId == "" {
				logger.Warn().Msgf(
					"Couldn't find the line item that was actually paid (%s), bailing", eventId,
				)
			}
			row := tx.QueryRow(`
				select $1::int from pricing_offers
				where stripe_monthly_price_id = $3 and plan_id = $4
				union
				select $2::int from pricing_offers
				where stripe_yearly_price_id = $3 and plan_id = $4
			`, models.PatronCreditsMonthly, models.PatronCreditsYearly, priceId, models.PlanIdPatron)
			var creditsBump int
			err := row.Scan(&creditsBump)
			if errors.Is(err, pgx.ErrNoRows) {
				// Not a patron invoice
				return nil
			} else if err != nil {
				return err
			}

			row = tx.QueryRow(`
				select id from users_with_discarded where stripe_subscription_id = $1
			`, invoice.Subscription.ID)
			var userId models.UserId
			err = row.Scan(&userId)
			if errors.Is(err, pgx.ErrNoRows) {
				// Special handling for a free user upgrading. The /upgrade wasn't called yet but we need to
				// bump the credits

				//nolint:exhaustruct
				sessionIter := session.List(&stripe.CheckoutSessionListParams{
					Customer: stripe.String(invoice.Customer.ID),
				})
				found := false
				for sessionIter.Next() {
					sesh := sessionIter.CheckoutSession()
					if userIdStr, ok := sesh.Metadata["user_id"]; ok {
						userIdInt, err := strconv.ParseInt(userIdStr, 10, 64)
						if err != nil {
							return oops.Wrap(err)
						}
						userId = models.UserId(userIdInt)
						found = true
					}
				}
				if err := sessionIter.Err(); err != nil {
					return oops.Wrap(err)
				}
				if !found {
					logger.Info().Msgf(
						"Couldn't find the user for Stripe sub %s, assuming the sign-up hasn't gone through yet",
						invoice.Subscription.ID,
					)
					return nil
				}

				stripeProductId := invoice.Lines.Data[0].Price.Product.ID
				row := tx.QueryRow(`
					select id, plan_id from pricing_offers where stripe_product_id = $1
				`, stripeProductId)
				var offerId models.OfferId
				var planId models.PlanId
				err = row.Scan(&offerId, &planId)
				if err != nil {
					return err
				}
				stripePriceId := invoice.Lines.Data[0].Price.ID
				billingInterval, err := models.BillingInterval_GetByOffer(tx, stripeProductId, stripePriceId)
				if err != nil {
					return err
				}
				_, err = tx.Exec(`
					update users_without_discarded
					set offer_id = $1,
						stripe_subscription_id = $2,
						stripe_customer_id = $3,
						billing_interval = $4
					where id = $5
				`, offerId, invoice.Subscription.ID, invoice.Customer.ID, billingInterval, userId)
				if err != nil {
					return err
				}
				logger.Info().Msgf("Initialized Stripe fields for user %d", userId)
			} else if err != nil {
				return err
			}

			result, err := tx.Exec(`
				insert into patron_credits (user_id, count) values ($1, 0)
				on conflict do nothing
			`, userId)
			if err != nil {
				return err
			}
			if result.RowsAffected() > 0 {
				logger.Info().Msgf("Initialized patron_credits for user %d", userId)
			}

			row = tx.QueryRow(`select count from patron_credits where user_id = $1`, userId)
			var oldCreditsCount int
			err = row.Scan(&oldCreditsCount)
			if err != nil {
				return err
			}

			_, err = tx.Exec(`insert into patron_invoices (id) values ($1)`, invoice.ID)
			if util.ViolatesUnique(err, "patron_invoices_pkey") {
				logger.Info().Msgf("Invoice already processed, bailing: %s", invoice.ID)
				return nil
			} else if err != nil {
				return err
			}

			row = tx.QueryRow(`
				update patron_credits set count = count + $1 where user_id = $2 returning count
			`, creditsBump, userId)
			var newCreditCount int
			err = row.Scan(&newCreditCount)
			if err != nil {
				return err
			}

			logger.Info().Msgf(
				"Bumped the credits from %d to %d for user %d", oldCreditsCount, newCreditCount, userId,
			)
			return nil
		})
		if err != nil {
			return err
		}
	default:
		return oops.Newf("Unknown event type: %v", event.Type)
	}

	_, err = conn.Exec(`delete from stripe_webhook_events where id = $1`, eventId)
	if err != nil {
		return err
	}

	return nil
}

func getPreviousValue(event *stripe.Event, keys ...string) (any, bool) {
	node, ok := event.Data.PreviousAttributes[keys[0]]
	if !ok {
		return nil, false
	}

	for i := 1; i < len(keys); i++ {
		key := keys[i]

		sliceNode, ok := node.([]interface{})
		if ok {
			intKey, err := strconv.Atoi(key)
			if err != nil {
				panic(fmt.Sprintf("Cannot access nested slice element with non-integer key: %s", key))
			}
			if len(sliceNode) <= intKey {
				return nil, false
			}
			node = sliceNode[intKey]
			continue
		}

		mapNode, ok := node.(map[string]interface{})
		if ok {
			node, ok = mapNode[key]
			if !ok {
				return nil, false
			}
			continue
		}

		panic(fmt.Sprintf("Cannot descend into non-map non-slice object with key: %s", key))
	}

	if node == nil {
		return nil, false
	}

	return node, true
}
