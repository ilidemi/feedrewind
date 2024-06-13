//go:build stripetesting

package e2etest

import (
	"feedrewind/config"
	"feedrewind/oops"
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/goccy/go-json"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v78"
	"github.com/stripe/stripe-go/v78/invoice"
)

//
// Free
//

func TestStripeFree(t *testing.T) {
	email := "test_onboarding@feedrewind.com"

	l := launcher.New().Headless(false)
	browserUrl := l.MustLaunch()
	browser := rod.New().ControlURL(browserUrl).MustConnect()

	page := visitAdminf(browser, "destroy_user?email=%s", email)
	require.Contains(t, []string{"OK", "NotFound"}, mustPageText(page))

	// Landing
	page = visitDev(browser, "")
	page.MustElement("#get_started").MustClick()

	// Pricing
	page.MustElement("#signup_free").MustClick()

	// Sign up
	page.MustElement("#email").MustInput(email)
	page.MustElement("#new-password").MustInput("tz123456")
	page.MustElementR("input", "Sign up").MustClick()

	// Dashboard
	page.MustElement("#user_button").MustClick()
	page.MustElement("#item_settings").MustClick()

	// Settings
	currentPlanText := page.MustElement("#current_plan").MustText()
	require.Equal(t, "Current plan: Free", currentPlanText)
	mustRequireNoElement(t, page, "#plan_timestamp")

	// DB
	userProperties := visitAdminSql(browser, `
		select offer_id, stripe_subscription_id, stripe_customer_id, billing_interval, stripe_cancel_at
		from users_without_discarded
		where email = '`+email+`'
	`)
	require.Equal(t, "free_2024-04-16", userProperties[0]["offer_id"])
	require.Equal(t, nil, userProperties[0]["stripe_subscription_id"])
	require.Equal(t, nil, userProperties[0]["stripe_customer_id"])
	require.Equal(t, nil, userProperties[0]["billing_interval"])
	require.Equal(t, nil, userProperties[0]["stripe_cancel_at"])

	// Cleanup
	page = visitAdminf(browser, "destroy_user?email=%s", email)
	require.Equal(t, "OK", mustPageText(page))

	browser.MustClose()
	l.Cleanup()
}

//
// Supporter
//

func TestStripeSupporter(t *testing.T) {
	email := "test_onboarding@feedrewind.com"
	l, browser := mustSetupStripeBrowser()

	for _, interval := range []string{"monthly", "yearly"} {
		page := visitAdminf(browser, "destroy_user?email=%s", email)
		require.Contains(t, []string{"OK", "NotFound"}, mustPageText(page))

		page = visitAdminf(browser, "ensure_stripe_listen")
		require.Equal(t, "OK", mustPageText(page))

		// Pricing
		page = visitDev(browser, "pricing")
		if interval == "yearly" {
			page.MustElement("#billing_interval_toggle").MustClick()
		}
		page.MustElement("#signup_supporter").MustClick()

		// Stripe checkout
		page.MustElement("#email").MustInput(email)
		page.MustElement("#cardNumber").MustInput("4242424242424242")
		page.MustElement("#cardExpiry").MustInput("242")
		page.MustElement("#cardCvc").MustInput("424")
		page.MustElement("#billingName").MustInput("2 42")
		page.MustElement("#billingAddressLine1").MustInput("400 Broad Street")
		page.MustElement("#billingLocality").MustInput("Seattle")
		page.MustElement("#billingPostalCode").MustInput("98109")
		enableStripePass := page.MustElement("#enableStripePass")
		if enableStripePass.MustProperty("checked").Bool() {
			enableStripePass.MustClick()
		}
		page.MustElement(".SubmitButton--complete").MustClick()

		// Create user
		page.MustElement("#new-password").MustInput("tz123456")
		page.MustElementR("input", "Sign up").MustClick()

		// Settings
		page.MustWaitLoad()
		page = visitDev(browser, "settings")
		currentPlanText := page.MustElement("#current_plan").MustText()
		require.Equal(t, "Current plan: Supporter", currentPlanText)
		planTimestampText := page.MustElement("#plan_timestamp").MustText()
		require.True(t, strings.HasPrefix(planTimestampText, "Renews on:"))
		if interval == "monthly" {
			require.True(t,
				strings.HasSuffix(planTimestampText, fmt.Sprintf(", %d", time.Now().Year())) ||
					(time.Now().Month() == time.December &&
						strings.Contains(planTimestampText, "Jan") &&
						strings.HasSuffix(planTimestampText, fmt.Sprintf(", %d", time.Now().Year()+1))),
			)
		} else {
			require.True(t, strings.HasSuffix(planTimestampText, fmt.Sprintf(", %d", time.Now().Year()+1)))
		}

		// DB
		userProperties := visitAdminSql(browser, `
			select offer_id, stripe_subscription_id, stripe_customer_id, billing_interval, stripe_cancel_at
			from users_without_discarded
			where email = '`+email+`'
		`)
		require.Equal(t, "supporter_2024-04-16", userProperties[0]["offer_id"])
		require.True(t, strings.HasPrefix(userProperties[0]["stripe_subscription_id"].(string), "sub_"))
		require.True(t, strings.HasPrefix(userProperties[0]["stripe_customer_id"].(string), "cus_"))
		require.Equal(t, interval, userProperties[0]["billing_interval"])
		require.Equal(t, nil, userProperties[0]["stripe_cancel_at"])

		// Cleanup
		page = visitAdminf(browser, "destroy_user?email=%s", email)
		require.Equal(t, "OK", mustPageText(page))
	}

	browser.MustClose()
	l.Cleanup()
}

func TestStripeSupporterChangeBillingInterval(t *testing.T) {
	email := "test_onboarding@feedrewind.com"
	l, browser := mustSetupStripeBrowser()
	mustSetupMonthlySupporter(t, browser, email)

	// Settings
	page := visitDev(browser, "settings")
	page.MustElement("#manage_billing").MustClick()

	// Billing portal
	page.MustElement("a[data-test='update-subscription']").MustClick()
	page.MustElement("input[data-testid='pricing-table-interval-year_1']").MustClick()
	page.MustElement("a[data-testid='continue-button']").MustClick()
	page.MustElement("[data-test='next']")
	page.MustElement("button[data-testid='confirm']").MustClick()
	page.MustElement("a[data-test='update-subscription']")

	// DB, poll until yearly
	pollCount := 0
	for {
		userProperties := visitAdminSql(browser, `
			select billing_interval	from users_without_discarded where email = '`+email+`'
		`)
		if userProperties[0]["billing_interval"] == "yearly" {
			break
		}

		time.Sleep(time.Second)
		pollCount++
		require.Less(t, pollCount, 10)
	}

	// Settings
	page = visitDev(browser, "settings")
	page.MustElement("#manage_billing").MustClick()

	// Billing portal
	page.MustElement("a[data-test='update-subscription']").MustClick()
	page.MustElement("input[data-testid='pricing-table-interval-month_1']").MustClick()
	page.MustElement("a[data-testid='continue-button']").MustClick()
	page.MustElement("[data-test='next']")
	page.MustElement("button[data-testid='confirm']").MustClick()
	page.MustElement("a[data-test='update-subscription']")

	// DB, poll until monthly
	pollCount = 0
	for {
		userProperties := visitAdminSql(browser, `
			select billing_interval	from users_without_discarded where email = '`+email+`'
		`)
		if userProperties[0]["billing_interval"] == "monthly" {
			break
		}

		time.Sleep(time.Second)
		pollCount++
		require.Less(t, pollCount, 10)
	}

	// Cleanup
	page = visitAdminf(browser, "destroy_user?email=%s", email)
	require.Equal(t, "OK", mustPageText(page))

	browser.MustClose()
	l.Cleanup()
}

func TestStripeSupporterCancelRenew(t *testing.T) {
	email := "test_onboarding@feedrewind.com"
	l, browser := mustSetupStripeBrowser()
	mustSetupMonthlySupporter(t, browser, email)

	// Settings
	page := visitDev(browser, "settings")
	page.MustElement("#manage_billing").MustClick()

	// Billing portal
	page.MustElement("a[data-test='cancel-subscription']").MustClick()
	page.MustElement("button[data-test='confirm']").MustClick()
	page.MustElement("button[data-testid='cancellation_reason_cancel']").MustClick()
	page.MustElement("a[data-test='renew-subscription']")
	page.MustElement("a[data-testid='return-to-business-link']").MustClick()

	// Settings
	currentPlanText := page.MustElement("#current_plan").MustText()
	require.Equal(t, "Current plan: Supporter", currentPlanText)
	planTimestampText := page.MustElement("#plan_timestamp").MustText()
	require.True(t, strings.HasPrefix(planTimestampText, "Ends on:"))
	require.True(t,
		strings.HasSuffix(planTimestampText, fmt.Sprintf(", %d", time.Now().Year())) ||
			(time.Now().Month() == time.December &&
				strings.Contains(planTimestampText, "Jan") &&
				strings.HasSuffix(planTimestampText, fmt.Sprintf(", %d", time.Now().Year()+1))),
	)

	// DB
	userProperties := visitAdminSql(browser, `
		select offer_id, stripe_subscription_id, stripe_customer_id, billing_interval, stripe_cancel_at
		from users_without_discarded
		where email = '`+email+`'
	`)
	require.Equal(t, "supporter_2024-04-16", userProperties[0]["offer_id"])
	require.True(t, strings.HasPrefix(userProperties[0]["stripe_subscription_id"].(string), "sub_"))
	require.True(t, strings.HasPrefix(userProperties[0]["stripe_customer_id"].(string), "cus_"))
	require.Equal(t, "monthly", userProperties[0]["billing_interval"])
	stripeCancelAt, err := time.Parse("2006-01-02T15:04:05", userProperties[0]["stripe_cancel_at"].(string))
	oops.RequireNoError(t, err)
	require.WithinRange(t, stripeCancelAt, time.Now(), time.Now().AddDate(0, 2, 0))

	// Settings
	page = visitDev(browser, "settings")
	page.MustElement("#manage_billing").MustClick()

	// Billing portal
	page.MustElement("a[data-test='renew-subscription']").MustClick()
	page.MustElement("button[data-test='confirm']").MustClick()
	page.MustElement("a[data-test='cancel-subscription']")
	page.MustElement("a[data-testid='return-to-business-link']").MustClick()

	// Settings
	currentPlanText = page.MustElement("#current_plan").MustText()
	require.Equal(t, "Current plan: Supporter", currentPlanText)

	// DB
	userProperties = visitAdminSql(browser, `
		select offer_id, stripe_subscription_id, stripe_customer_id, billing_interval, stripe_cancel_at
		from users_without_discarded
		where email = '`+email+`'
	`)
	require.Equal(t, "supporter_2024-04-16", userProperties[0]["offer_id"])
	require.True(t, strings.HasPrefix(userProperties[0]["stripe_subscription_id"].(string), "sub_"))
	require.True(t, strings.HasPrefix(userProperties[0]["stripe_customer_id"].(string), "cus_"))
	require.Equal(t, "monthly", userProperties[0]["billing_interval"])
	require.Equal(t, nil, userProperties[0]["stripe_cancel_at"])

	// Cleanup
	page = visitAdminf(browser, "destroy_user?email=%s", email)
	require.Equal(t, "OK", mustPageText(page))

	browser.MustClose()
	l.Cleanup()
}

func TestStripeSupporterDeleteOnBackend(t *testing.T) {
	email := "test_onboarding@feedrewind.com"
	l, browser := mustSetupStripeBrowser()
	mustSetupMonthlySupporter(t, browser, email)

	// Delete
	page := visitAdminf(browser, "delete_stripe_subscription?email=%s", email)
	require.Equal(t, "OK", mustPageText(page))

	// DB, poll until deleted
	pollCount := 0
	var userProperties []map[string]any
	for {
		userProperties = visitAdminSql(browser, `
			select offer_id, stripe_subscription_id, stripe_customer_id, billing_interval, stripe_cancel_at
			from users_without_discarded
			where email = '`+email+`'
		`)
		if userProperties[0]["stripe_subscription_id"] == nil {
			break
		}

		time.Sleep(time.Second)
		pollCount++
		require.Less(t, pollCount, 10)
	}
	require.Equal(t, "free_2024-04-16", userProperties[0]["offer_id"])
	require.Equal(t, nil, userProperties[0]["stripe_subscription_id"])
	require.True(t, strings.HasPrefix(userProperties[0]["stripe_customer_id"].(string), "cus_"))
	require.Equal(t, nil, userProperties[0]["billing_interval"])
	require.Equal(t, nil, userProperties[0]["stripe_cancel_at"])

	// Settings
	page = visitDev(browser, "settings")
	currentPlanText := page.MustElement("#current_plan").MustText()
	require.Equal(t, "Current plan: Free", currentPlanText)

	// Cleanup
	page = visitAdminf(browser, "destroy_user?email=%s", email)
	require.Equal(t, "OK", mustPageText(page))

	browser.MustClose()
	l.Cleanup()
}

func TestStripeSupporterDeleteCanceledOnBackend(t *testing.T) {
	email := "test_onboarding@feedrewind.com"
	l, browser := mustSetupStripeBrowser()
	mustSetupMonthlySupporter(t, browser, email)

	// Pricing
	page := visitDev(browser, "pricing")
	page.MustElement("#current_supporter")
	page.MustElement("#downgrade_to_free").MustClick()

	// Billing portal
	page.MustElement("a[data-test='cancel-subscription']").MustClick()
	page.MustElement("button[data-test='confirm']").MustClick()
	page.MustElement("button[data-testid='cancellation_reason_cancel']").MustClick()
	page.MustElement("a[data-test='renew-subscription']")

	// DB, poll until canceled
	pollCount := 0
	for {
		userProperties := visitAdminSql(browser, `
			select offer_id, stripe_subscription_id, stripe_customer_id, billing_interval, stripe_cancel_at
			from users_without_discarded
			where email = '`+email+`'
		`)
		if userProperties[0]["stripe_cancel_at"] != nil {
			break
		}

		time.Sleep(time.Second)
		pollCount++
		require.Less(t, pollCount, 10)
	}

	// Delete
	page = visitAdminf(browser, "delete_stripe_subscription?email=%s", email)
	require.Equal(t, "OK", mustPageText(page))

	// DB, poll until deleted
	pollCount = 0
	var userProperties []map[string]any
	for {
		userProperties = visitAdminSql(browser, `
			select offer_id, stripe_subscription_id, stripe_customer_id, billing_interval, stripe_cancel_at
			from users_without_discarded
			where email = '`+email+`'
		`)
		if userProperties[0]["stripe_subscription_id"] == nil {
			break
		}

		time.Sleep(time.Second)
		pollCount++
		require.Less(t, pollCount, 10)
	}
	require.Equal(t, "free_2024-04-16", userProperties[0]["offer_id"])
	require.Equal(t, nil, userProperties[0]["stripe_subscription_id"])
	require.True(t, strings.HasPrefix(userProperties[0]["stripe_customer_id"].(string), "cus_"))
	require.Equal(t, nil, userProperties[0]["billing_interval"])
	require.Equal(t, nil, userProperties[0]["stripe_cancel_at"])

	// Settings
	page = visitDev(browser, "settings")
	currentPlanText := page.MustElement("#current_plan").MustText()
	require.Equal(t, "Current plan: Free", currentPlanText)

	// Cleanup
	page = visitAdminf(browser, "destroy_user?email=%s", email)
	require.Equal(t, "OK", mustPageText(page))

	browser.MustClose()
	l.Cleanup()
}

func TestStripeSupporterDeleteAccount(t *testing.T) {
	email := "test_onboarding@feedrewind.com"
	l, browser := mustSetupStripeBrowser()
	mustSetupMonthlySupporter(t, browser, email)

	// Settings
	page := visitDev(browser, "settings")
	page.MustElement("#delete_account_button").MustClick()
	page.MustElement("#delete_popup_delete_button").MustClick()
	page.MustWaitLoad()

	// DB, poll until deleted
	pollCount := 0
	var userProperties []map[string]any
	for {
		userProperties = visitAdminSql(browser, `
			select discarded_at, offer_id, stripe_subscription_id, stripe_customer_id, billing_interval,
				stripe_cancel_at
			from users_with_discarded
			where email = '`+email+`'
		`)
		if userProperties[0]["stripe_subscription_id"] == nil {
			break
		}

		time.Sleep(time.Second)
		pollCount++
		require.Less(t, pollCount, 10)
	}
	require.Equal(t, "free_2024-04-16", userProperties[0]["offer_id"])
	require.Equal(t, nil, userProperties[0]["stripe_subscription_id"])
	require.True(t, strings.HasPrefix(userProperties[0]["stripe_customer_id"].(string), "cus_"))
	require.Equal(t, nil, userProperties[0]["billing_interval"])
	require.Equal(t, nil, userProperties[0]["stripe_cancel_at"])

	// Cleanup
	page = visitAdminf(browser, "destroy_user?email=%s", email)
	require.Equal(t, "OK", mustPageText(page))

	browser.MustClose()
	l.Cleanup()
}

func TestStripeFreeUpgradeToYearlySupporter(t *testing.T) {
	email := "test_onboarding@feedrewind.com"
	l, browser := mustSetupStripeBrowser()

	page := visitAdminf(browser, "destroy_user?email=%s", email)
	require.Contains(t, []string{"OK", "NotFound"}, mustPageText(page))

	// Landing
	page = visitDev(browser, "")
	page.MustElement("#get_started").MustClick()

	// Pricing
	page.MustElement("#signup_free").MustClick()

	// Sign up
	page.MustElement("#email").MustInput(email)
	page.MustElement("#new-password").MustInput("tz123456")
	page.MustElementR("input", "Sign up").MustClick()
	page.MustWaitLoad()

	// Settings
	page = visitDev(browser, "settings")
	page.MustElement("#upgrade").MustClick()

	// Pricing
	page.MustElement("#billing_interval_toggle").MustClick()
	page.MustElement("#upgrade_to_supporter").MustClick()

	// Stripe checkout
	page.MustElement("#cardNumber").MustInput("4242424242424242")
	page.MustElement("#cardExpiry").MustInput("242")
	page.MustElement("#cardCvc").MustInput("424")
	page.MustElement("#billingName").MustInput("2 42")
	page.MustElement("#billingAddressLine1").MustInput("400 Broad Street")
	page.MustElement("#billingLocality").MustInput("Seattle")
	page.MustElement("#billingPostalCode").MustInput("98109")
	enableStripePass := page.MustElement("#enableStripePass")
	if enableStripePass.MustProperty("checked").Bool() {
		enableStripePass.MustClick()
	}
	page.MustElement(".SubmitButton--complete").MustClick()

	// Settings
	currentPlanText := page.MustElement("#current_plan").MustText()
	require.Equal(t, "Current plan: Supporter", currentPlanText)

	// DB
	userProperties := visitAdminSql(browser, `
		select offer_id, stripe_subscription_id, stripe_customer_id, billing_interval, stripe_cancel_at
		from users_without_discarded
		where email = '`+email+`'
	`)
	require.Equal(t, "supporter_2024-04-16", userProperties[0]["offer_id"])
	require.True(t, strings.HasPrefix(userProperties[0]["stripe_subscription_id"].(string), "sub_"))
	require.True(t, strings.HasPrefix(userProperties[0]["stripe_customer_id"].(string), "cus_"))
	require.Equal(t, "yearly", userProperties[0]["billing_interval"])
	require.Equal(t, nil, userProperties[0]["stripe_cancel_at"])

	// Cleanup
	page = visitAdminf(browser, "destroy_user?email=%s", email)
	require.Equal(t, "OK", mustPageText(page))

	browser.MustClose()
	l.Cleanup()
}

//
// Patron
//

func TestStripePatron(t *testing.T) {
	email := "test_onboarding@feedrewind.com"
	l, browser := mustSetupStripeBrowser()

	for _, interval := range []string{"monthly", "yearly"} {
		page := visitAdminf(browser, "destroy_user?email=%s", email)
		require.Contains(t, []string{"OK", "NotFound"}, mustPageText(page))

		page = visitAdminf(browser, "ensure_stripe_listen")
		require.Equal(t, "OK", mustPageText(page))

		// Pricing
		page = visitDev(browser, "pricing")
		if interval == "yearly" {
			page.MustElement("#billing_interval_toggle").MustClick()
		}
		page.MustElement("#signup_patron").MustClick()

		// Stripe checkout
		page.MustElement("#email").MustInput(email)
		page.MustElement("#cardNumber").MustInput("4242424242424242")
		page.MustElement("#cardExpiry").MustInput("242")
		page.MustElement("#cardCvc").MustInput("424")
		page.MustElement("#billingName").MustInput("2 42")
		page.MustElement("#billingAddressLine1").MustInput("400 Broad Street")
		page.MustElement("#billingLocality").MustInput("Seattle")
		page.MustElement("#billingPostalCode").MustInput("98109")
		enableStripePass := page.MustElement("#enableStripePass")
		if enableStripePass.MustProperty("checked").Bool() {
			enableStripePass.MustClick()
		}
		page.MustElement(".SubmitButton--complete").MustClick()

		// Create user
		page.MustElement("#new-password").MustInput("tz123456")
		page.MustElementR("input", "Sign up").MustClick()

		// Settings
		page.MustWaitLoad()
		page = visitDev(browser, "settings")
		currentPlanText := page.MustElement("#current_plan").MustText()
		require.Equal(t, "Current plan: Patron", currentPlanText)
		patronCreditsText := page.MustElement("#patron_credits").MustText()
		if interval == "monthly" {
			require.Equal(t, "Credits available: 1", patronCreditsText)
		} else {
			require.Equal(t, "Credits available: 12", patronCreditsText)
		}
		planTimestampText := page.MustElement("#plan_timestamp").MustText()
		require.True(t, strings.HasPrefix(planTimestampText, "Renews on:"))
		if interval == "monthly" {
			require.True(t,
				strings.HasSuffix(planTimestampText, fmt.Sprintf(", %d", time.Now().Year())) ||
					(time.Now().Month() == time.December &&
						strings.Contains(planTimestampText, "Jan") &&
						strings.HasSuffix(planTimestampText, fmt.Sprintf(", %d", time.Now().Year()+1))),
			)
		} else {
			require.True(t, strings.HasSuffix(planTimestampText, fmt.Sprintf(", %d", time.Now().Year()+1)))
		}

		// DB
		userProperties := visitAdminSql(browser, `
			select offer_id, stripe_subscription_id, stripe_customer_id, billing_interval, stripe_cancel_at
			from users_without_discarded
			where email = '`+email+`'
		`)
		require.Equal(t, "patron_2024-04-16", userProperties[0]["offer_id"])
		require.True(t, strings.HasPrefix(userProperties[0]["stripe_subscription_id"].(string), "sub_"))
		require.True(t, strings.HasPrefix(userProperties[0]["stripe_customer_id"].(string), "cus_"))
		require.Equal(t, interval, userProperties[0]["billing_interval"])
		require.Equal(t, nil, userProperties[0]["stripe_cancel_at"])

		// Cleanup
		page = visitAdminf(browser, "destroy_user?email=%s", email)
		require.Equal(t, "OK", mustPageText(page))
	}

	browser.MustClose()
	l.Cleanup()
}

func TestStripePatronChangeBillingInterval(t *testing.T) {
	email := "test_onboarding@feedrewind.com"
	l, browser := mustSetupStripeBrowser()
	mustSetupMonthlyPatron(t, browser, email)

	// Settings
	page := visitDev(browser, "settings")
	patronCreditsText := page.MustElement("#patron_credits").MustText()
	require.Equal(t, "Credits available: 1", patronCreditsText)
	page.MustElement("#manage_billing").MustClick()

	// Billing portal
	page.MustElement("a[data-test='update-subscription']").MustClick()
	page.MustElement("input[data-testid='pricing-table-interval-year_1']").MustClick()
	page.MustElement("a[data-testid='continue-button']").MustClick()
	page.MustElement("[data-test='next']")
	page.MustElement("button[data-testid='confirm']").MustClick()
	page.MustElement("a[data-test='update-subscription']")

	// DB, poll until yearly
	pollCount := 0
	for {
		userProperties := visitAdminSql(browser, `
			select billing_interval	from users_without_discarded where email = '`+email+`'
		`)
		if userProperties[0]["billing_interval"] == "yearly" {
			break
		}

		time.Sleep(time.Second)
		pollCount++
		require.Less(t, pollCount, 10)
	}

	// Settings
	page = visitDev(browser, "settings")
	patronCreditsText = page.MustElement("#patron_credits").MustText()
	require.Equal(t, "Credits available: 13", patronCreditsText)
	page.MustElement("#manage_billing").MustClick()

	// Billing portal
	page.MustElement("a[data-test='update-subscription']").MustClick()
	page.MustElement("input[data-testid='pricing-table-interval-month_1']").MustClick()
	page.MustElement("a[data-testid='continue-button']").MustClick()
	page.MustElement("[data-test='next']")
	page.MustElement("button[data-testid='confirm']").MustClick()
	page.MustElement("a[data-test='update-subscription']")

	// DB, poll until monthly
	pollCount = 0
	for {
		userProperties := visitAdminSql(browser, `
			select billing_interval	from users_without_discarded where email = '`+email+`'
		`)
		if userProperties[0]["billing_interval"] == "monthly" {
			break
		}

		time.Sleep(time.Second)
		pollCount++
		require.Less(t, pollCount, 10)
	}

	// Settings
	page = visitDev(browser, "settings")
	patronCreditsText = page.MustElement("#patron_credits").MustText()
	require.Equal(t, "Credits available: 14", patronCreditsText)

	// Cleanup
	page = visitAdminf(browser, "destroy_user?email=%s", email)
	require.Equal(t, "OK", mustPageText(page))

	browser.MustClose()
	l.Cleanup()
}

func TestStripePatronNoCreditsCap(t *testing.T) {
	email := "test_onboarding@feedrewind.com"
	l, browser := mustSetupStripeBrowser()
	stripe.Key = config.Cfg.StripeApiKey

	advanceDaysByInterval := map[string]int{
		"monthly": 32,
		"yearly":  367,
	}
	expectCreditsByInterval := map[string][]string{
		"monthly": {"2", "3", "4"},
		"yearly":  {"24", "36", "48"},
	}

	for _, interval := range []string{"monthly", "yearly"} {
		advanceDays := advanceDaysByInterval[interval]
		expectCredits := expectCreditsByInterval[interval]

		// Enable test clock
		page := visitAdmin(browser, "set_test_singleton?key=test_clock&value=yes")
		require.Equal(t, "OK", mustPageText(page))

		mustSetupPaidUser(t, browser, email, "#signup_patron", interval == "yearly")

		// Pricing, tacked on
		page = visitDev(browser, "pricing")
		page.MustElement("#current_patron")

		// Forward time
		page = visitAdminf(browser, "forward_stripe_customer?email=%s&days=%d", email, advanceDays)
		require.Equal(t, "OK", mustPageText(page))

		// DB get stripe customer id
		userProperties := visitAdminSql(browser, `
		select stripe_customer_id from users_without_discarded where email = '`+email+`'
	`)
		stripeCustomerId := userProperties[0]["stripe_customer_id"].(string)

		// Pay the invoice (only the first seems to need payment)
		pollCount := 0
	pollInvoice1:
		for {
			//nolint:exhaustruct
			invoiceIter := invoice.List(&stripe.InvoiceListParams{
				Customer: stripe.String(stripeCustomerId),
				Status:   stripe.String(string(stripe.InvoiceStatusDraft)),
			})
			for invoiceIter.Next() {
				inv := invoiceIter.Invoice()
				_, err := invoice.Pay(inv.ID, nil)
				oops.RequireNoError(t, err)
				break pollInvoice1
			}
			oops.RequireNoError(t, invoiceIter.Err())

			time.Sleep(time.Second)
			pollCount++
			require.Less(t, pollCount, 10)
		}

		// Settings, poll until credits are added
		pollCount = 0
		for {
			page := visitDev(browser, "settings")
			patronCreditsText := page.MustElement("#patron_credits").MustText()
			if patronCreditsText == "Credits available: "+expectCredits[0] {
				break
			}

			time.Sleep(time.Second)
			pollCount++
			require.Less(t, pollCount, 20)
		}

		// Forward time
		page = visitAdminf(browser, "forward_stripe_customer?email=%s&days=%d", email, advanceDays)
		require.Equal(t, "OK", mustPageText(page))

		// Settings, poll until credits are added
		pollCount = 0
		for {
			page := visitDev(browser, "settings")
			patronCreditsText := page.MustElement("#patron_credits").MustText()
			if patronCreditsText == "Credits available: "+expectCredits[1] {
				break
			}

			time.Sleep(time.Second)
			pollCount++
			require.Less(t, pollCount, 20)
		}

		// Forward time
		page = visitAdminf(browser, "forward_stripe_customer?email=%s&days=%d", email, advanceDays)
		require.Equal(t, "OK", mustPageText(page))

		// Poll Stripe until 4 invoices show up
		var stripeInvoiceIds []string
		pollCount = 0
		for {
			stripeInvoiceIds = nil
			//nolint:exhaustruct
			invoiceIter := invoice.List(&stripe.InvoiceListParams{
				Customer: stripe.String(stripeCustomerId),
			})
			for invoiceIter.Next() {
				stripeInvoiceIds = append(stripeInvoiceIds, invoiceIter.Invoice().ID)
			}
			oops.RequireNoError(t, invoiceIter.Err())
			if len(stripeInvoiceIds) == 4 {
				break
			}

			time.Sleep(time.Second)
			pollCount++
			require.Less(t, pollCount, 10)
		}

		// DB, poll until all invoices are processed
		var stripeInvoiceIdsSb strings.Builder
		stripeInvoiceIdsSb.WriteString("('")
		for i, stripeInvoiceId := range stripeInvoiceIds {
			if i > 0 {
				stripeInvoiceIdsSb.WriteString("', '")
			}
			stripeInvoiceIdsSb.WriteString(stripeInvoiceId)
		}
		stripeInvoiceIdsSb.WriteString("')")
		pollCount = 0
		for {
			stripeInvoicesCount := visitAdminSql(
				browser, `select count(1) from patron_invoices where id in `+stripeInvoiceIdsSb.String(),
			)
			if stripeInvoicesCount[0]["count"].(json.Number).String() == "4" {
				break
			}

			time.Sleep(time.Second)
			pollCount++
			require.Less(t, pollCount, 10)
		}

		// Settings, 4x credits
		page = visitDev(browser, "settings")
		patronCreditsText := page.MustElement("#patron_credits").MustText()
		require.Equal(t, "Credits available: "+expectCredits[2], patronCreditsText)

		// Cleanup
		page = visitAdmin(browser, "delete_stripe_clocks")
		require.Equal(t, "OK", mustPageText(page))
		page = visitAdmin(browser, "delete_test_singleton?key=test_clock")
		require.Equal(t, "OK", mustPageText(page))
		page = visitAdminf(browser, "destroy_user?email=%s", email)
		require.Equal(t, "OK", mustPageText(page))
	}

	browser.MustClose()
	l.Cleanup()
}

func TestStripeFreeUpgrade(t *testing.T) {
	email := "test_onboarding@feedrewind.com"
	l, browser := mustSetupStripeBrowser()

	for _, paidPlan := range []string{"supporter", "patron"} {
		for _, interval := range []string{"monthly", "yearly"} {
			page := visitAdminf(browser, "destroy_user?email=%s", email)
			require.Contains(t, []string{"OK", "NotFound"}, mustPageText(page))

			// Landing
			page = visitDev(browser, "")
			page.MustElement("#get_started").MustClick()

			// Pricing
			page.MustElement("#signup_free").MustClick()

			// Sign up
			page.MustElement("#email").MustInput(email)
			page.MustElement("#new-password").MustInput("tz123456")
			page.MustElementR("input", "Sign up").MustClick()
			page.MustWaitLoad()

			// Settings
			page = visitDev(browser, "settings")
			page.MustElement("#upgrade").MustClick()

			// Pricing
			page.MustElement("#current_free")
			if interval == "yearly" {
				page.MustElement("#billing_interval_toggle").MustClick()
			}
			upgradeSelector := fmt.Sprintf("#upgrade_to_%s", paidPlan)
			page.MustElement(upgradeSelector).MustClick()

			// Stripe checkout
			page.MustElement("#cardNumber").MustInput("4242424242424242")
			page.MustElement("#cardExpiry").MustInput("242")
			page.MustElement("#cardCvc").MustInput("424")
			page.MustElement("#billingName").MustInput("2 42")
			page.MustElement("#billingAddressLine1").MustInput("400 Broad Street")
			page.MustElement("#billingLocality").MustInput("Seattle")
			page.MustElement("#billingPostalCode").MustInput("98109")
			enableStripePass := page.MustElement("#enableStripePass")
			if enableStripePass.MustProperty("checked").Bool() {
				enableStripePass.MustClick()
			}
			page.MustElement(".SubmitButton--complete").MustClick()

			// Settings
			if paidPlan == "supporter" {
				currentPlanText := page.MustElement("#current_plan").MustText()
				require.Equal(t, "Current plan: Supporter", currentPlanText)
			} else {
				currentPlanText := page.MustElement("#current_plan").MustText()
				require.Equal(t, "Current plan: Patron", currentPlanText)

				var expectedCreditsText string
				if interval == "monthly" {
					expectedCreditsText = "Credits available: 1"
				} else {
					expectedCreditsText = "Credits available: 12"
				}
				pollCount := 0
				for {
					page := visitDev(browser, "settings")
					patronCreditsText := page.MustElement("#patron_credits").MustText()
					if patronCreditsText == expectedCreditsText {
						break
					}

					time.Sleep(time.Second)
					pollCount++
					require.Less(t, pollCount, 10)
				}
			}

			// DB
			userProperties := visitAdminSql(browser, `
				select
					offer_id, stripe_subscription_id, stripe_customer_id, billing_interval, stripe_cancel_at
				from users_without_discarded
				where email = '`+email+`'
			`)
			require.Equal(t, paidPlan+"_2024-04-16", userProperties[0]["offer_id"])
			require.True(t, strings.HasPrefix(userProperties[0]["stripe_subscription_id"].(string), "sub_"))
			require.True(t, strings.HasPrefix(userProperties[0]["stripe_customer_id"].(string), "cus_"))
			require.Equal(t, interval, userProperties[0]["billing_interval"])
			require.Equal(t, nil, userProperties[0]["stripe_cancel_at"])

			// Cleanup
			page = visitAdminf(browser, "destroy_user?email=%s", email)
			require.Equal(t, "OK", mustPageText(page))
		}
	}

	browser.MustClose()
	l.Cleanup()
}

func TestStripePaidCancelExpire(t *testing.T) {
	email := "test_onboarding@feedrewind.com"
	l, browser := mustSetupStripeBrowser()

	for _, paidPlan := range []string{"supporter", "patron"} {
		// Enable test clock
		page := visitAdmin(browser, "set_test_singleton?key=test_clock&value=yes")
		require.Equal(t, "OK", mustPageText(page))

		mustSetupPaidUser(t, browser, email, "#signup_"+paidPlan, false)

		// Prepare stale settings
		staleSettingsPage := visitDev(browser, "settings")

		// Settings
		page = visitDev(browser, "settings")
		page.MustElement("#manage_billing").MustClick()

		// Billing portal
		page.MustElement("a[data-test='cancel-subscription']").MustClick()
		page.MustElement("button[data-test='confirm']").MustClick()
		page.MustElement("button[data-testid='cancellation_reason_cancel']").MustClick()
		page.MustElement("a[data-test='renew-subscription']")
		page.MustElement("a[data-testid='return-to-business-link']").MustClick()

		// DB, poll until canceled
		pollCount := 0
		for {
			userProperties := visitAdminSql(browser, `
				select stripe_cancel_at from users_without_discarded where email = '`+email+`'
			`)
			if userProperties[0]["stripe_cancel_at"] != nil {
				break
			}

			time.Sleep(time.Second)
			pollCount++
			require.Less(t, pollCount, 10)
		}

		// Forward time
		page = visitAdminf(browser, "forward_stripe_customer?email=%s&days=45", email)
		require.Equal(t, "OK", mustPageText(page))

		// DB, poll until deleted
		pollCount = 0
		var userProperties []map[string]any
		for {
			userProperties = visitAdminSql(browser, `
				select offer_id, stripe_subscription_id, stripe_customer_id, billing_interval, stripe_cancel_at
				from users_without_discarded
				where email = '`+email+`'
			`)
			if userProperties[0]["stripe_subscription_id"] == nil {
				break
			}

			time.Sleep(time.Second)
			pollCount++
			require.Less(t, pollCount, 10)
		}
		require.Equal(t, "free_2024-04-16", userProperties[0]["offer_id"])
		require.Equal(t, nil, userProperties[0]["stripe_subscription_id"])
		require.True(t, strings.HasPrefix(userProperties[0]["stripe_customer_id"].(string), "cus_"))
		require.Equal(t, nil, userProperties[0]["billing_interval"])
		require.Equal(t, nil, userProperties[0]["stripe_cancel_at"])

		// Settings
		page = visitDev(browser, "settings")
		currentPlanText := page.MustElement("#current_plan").MustText()
		require.Equal(t, "Current plan: Free", currentPlanText)

		// Stale settings, link to the billing portal doesn't do anything
		staleSettingsPage.MustActivate()
		staleSettingsPage.MustElement("#manage_billing").MustClick()
		staleSettingsPage.MustWaitLoad()
		require.Equal(t, "http://localhost:3000/settings", staleSettingsPage.MustInfo().URL)
		currentPlanText = staleSettingsPage.MustElement("#current_plan").MustText()
		require.Equal(t, "Current plan: Free", currentPlanText)

		// Cleanup
		page = visitAdmin(browser, "delete_stripe_clocks")
		require.Equal(t, "OK", mustPageText(page))
		page = visitAdmin(browser, "delete_test_singleton?key=test_clock")
		require.Equal(t, "OK", mustPageText(page))
		page = visitAdminf(browser, "destroy_user?email=%s", email)
		require.Equal(t, "OK", mustPageText(page))
	}

	browser.MustClose()
	l.Cleanup()
}

func TestStripeSupporterToPatronAndBack(t *testing.T) {
	email := "test_onboarding@feedrewind.com"
	l, browser := mustSetupStripeBrowser()
	mustSetupMonthlySupporter(t, browser, email)

	// Pricing
	page := visitDev(browser, "pricing")
	page.MustElement("#upgrade_to_patron").MustClick()

	// Billing portal, upgrade to patron
	page.MustElement("a[data-test='update-subscription']").MustClick()
	page.MustElementR("a[role='button']", "Select").MustClick()
	page.MustElement("a[data-testid='continue-button']").MustClick()
	page.MustElement("[data-test='next']")
	page.MustElement("button[data-testid='confirm']").MustClick()
	page.MustElement("a[data-test='update-subscription']")

	// Settings, poll until plan is updated
	pollCount := 0
	for {
		page = visitDev(browser, "settings")
		currentPlanText := page.MustElement("#current_plan").MustText()
		if currentPlanText == "Current plan: Patron" {
			break
		}

		time.Sleep(time.Second)
		pollCount++
		require.Less(t, pollCount, 10)
	}
	patronCreditsText := page.MustElement("#patron_credits").MustText()
	require.Equal(t, "Credits available: 1", patronCreditsText)

	// Pricing
	page = visitDev(browser, "pricing")
	page.MustElement("#downgrade_to_supporter").MustClick()

	// Billing portal, downgrade to supporter
	page.MustElement("a[data-test='update-subscription']").MustClick()
	page.MustElementR("a[role='button']", "Select").MustClick()
	page.MustElement("a[data-testid='continue-button']").MustClick()
	page.MustElement("[data-test='next']")
	page.MustElement("button[data-testid='confirm']").MustClick()
	page.MustElement("a[data-test='update-subscription']")

	// Settings, poll until plan is updated
	pollCount = 0
	for {
		page = visitDev(browser, "settings")
		currentPlanText := page.MustElement("#current_plan").MustText()
		if currentPlanText == "Current plan: Supporter" {
			break
		}

		time.Sleep(time.Second)
		pollCount++
		require.Less(t, pollCount, 10)
	}

	// Pricing
	page = visitDev(browser, "pricing")
	page.MustElement("#upgrade_to_patron").MustClick()

	// Billing portal, upgrade to patron
	page.MustElement("a[data-test='update-subscription']").MustClick()
	page.MustElementR("a[role='button']", "Select").MustClick()
	page.MustElement("a[data-testid='continue-button']").MustClick()
	page.MustElement("[data-test='next']")
	page.MustElement("button[data-testid='confirm']").MustClick()
	page.MustElement("a[data-test='update-subscription']")

	// Settings, poll until plan is updated
	pollCount = 0
	for {
		page = visitDev(browser, "settings")
		currentPlanText := page.MustElement("#current_plan").MustText()
		if currentPlanText == "Current plan: Patron" {
			break
		}

		time.Sleep(time.Second)
		pollCount++
		require.Less(t, pollCount, 10)
	}
	patronCreditsText = page.MustElement("#patron_credits").MustText()
	require.Equal(t, "Credits available: 2", patronCreditsText)

	// Cleanup
	page = visitAdminf(browser, "destroy_user?email=%s", email)
	require.Equal(t, "OK", mustPageText(page))

	browser.MustClose()
	l.Cleanup()
}

//
// Custom blog request
//

func TestStripePatronWithCreditsCustomBlogRequest(t *testing.T) {
	email := "test_onboarding@feedrewind.com"
	l, browser := mustSetupStripeBrowser()
	mustSetupMonthlyPatron(t, browser, email)

	// Enable slack dump
	page := visitAdmin(browser, "set_test_singleton?key=slack_dump&value=yes")
	require.Equal(t, "OK", mustPageText(page))

	// Add failing blog
	page = visitDev(browser, "subscriptions/add")
	page.MustElement("#start_url").MustInput("https://ilidemi.github.io/dummy-blogs/1fa/")
	page.MustElement("#discover_go").MustClick()

	// Request support
	page.MustElement("#request_button").MustClick()
	why := fmt.Sprintf("why_%x", rand.Uint64())
	page.MustElement("#why").MustInput(why)
	creditsAvailableText := page.MustElement("#credits_available").MustText()
	require.Equal(t, "Credits available: 1", creditsAvailableText)
	mustRequireNoElement(t, page, "#credits_renew_on")
	page.MustElement("#submit").MustClick()

	// Ensure ack
	page.MustElement("#request_ack")

	// Ensure shows up on the dashboard
	subscriptionId := mustParseSubscriptionId(page)
	page = visitDev(browser, "subscriptions")
	page.MustElement("a[href='/subscriptions/" + subscriptionId + "/setup']")

	// Ensure makes it into db
	customBlogRequest := visitAdminSql(browser, `
		select feed_url, why from custom_blog_requests where subscription_id = `+subscriptionId)
	require.Equal(t, "https://ilidemi.github.io/dummy-blogs/1fa/rss.xml", customBlogRequest[0]["feed_url"])
	require.Equal(t, why, customBlogRequest[0]["why"])

	// Ensure credits reduced
	page = visitDev(browser, "settings")
	patronCreditsText := page.MustElement("#patron_credits").MustText()
	require.Equal(t, "Credits available: 0", patronCreditsText)

	// Ensure slack message
	page = visitAdmin(browser, "get_test_singleton?key=slack_last_message")
	require.Equal(t, "Custom blog requested for subscription "+subscriptionId, mustPageText(page))

	// Ensure the subscription can't be deleted while the request is there
	page = visitAdmin(browser, `
		execute_sql?query=delete from subscriptions_with_discarded where id = `+subscriptionId+` returning id
	`)
	require.Equal(t,
		`ERROR: ERROR: update or delete on table "subscriptions" violates foreign key constraint "custom_blog_requests_subscription_id_fkey" on table "custom_blog_requests" (SQLSTATE 23503)`,
		mustPageText(page),
	)

	// Cleanup
	page = visitAdmin(browser, "delete_test_singleton?key=slack_dump")
	require.Equal(t, "OK", mustPageText(page))
	page = visitAdmin(browser, "delete_test_singleton?key=slack_last_message")
	require.Equal(t, "OK", mustPageText(page))
	page = visitAdminf(browser, "destroy_user?email=%s", email)
	require.Equal(t, "OK", mustPageText(page))

	browser.MustClose()
	l.Cleanup()
}

func TestStripePatronWithoutCreditsCustomBlogRequest(t *testing.T) {
	email := "test_onboarding@feedrewind.com"
	l, browser := mustSetupStripeBrowser()
	mustSetupMonthlyPatron(t, browser, email)

	// Subtract credits
	visitAdminSql(browser, `
		update patron_credits
		set count = 0
		where user_id = (select id from users_without_discarded where email = '`+email+`')
		returning count
	`)

	// Enable slack dump
	page := visitAdmin(browser, "set_test_singleton?key=slack_dump&value=yes")
	require.Equal(t, "OK", mustPageText(page))

	// Add failing blog
	page = visitDev(browser, "subscriptions/add")
	page.MustElement("#start_url").MustInput("https://ilidemi.github.io/dummy-blogs/1fa/")
	page.MustElement("#discover_go").MustClick()

	// Request support
	page.MustElement("#request_button").MustClick()
	why := fmt.Sprintf("why_%x", rand.Uint64())
	page.MustElement("#why").MustInput(why)
	mustRequireNoElement(t, page, "#credits_available")
	creditsRenewOnText := page.MustElement("#credits_renew_on").MustText()
	require.True(t, strings.HasPrefix(creditsRenewOnText, "Your patron credits will renew on "))
	require.True(t,
		strings.HasSuffix(creditsRenewOnText, fmt.Sprintf(", %d", time.Now().Year())) ||
			(time.Now().Month() == time.December &&
				strings.Contains(creditsRenewOnText, "Jan") &&
				strings.HasSuffix(creditsRenewOnText, fmt.Sprintf(", %d", time.Now().Year()+1))),
	)
	page.MustElement("#submit").MustClick()

	// Stripe checkout
	page.MustElement("#cardNumber").MustInput("4242424242424242")
	page.MustElement("#cardExpiry").MustInput("242")
	page.MustElement("#cardCvc").MustInput("424")
	page.MustElement("#billingName").MustInput("2 42")
	page.MustElement("#billingAddressLine1").MustInput("400 Broad Street")
	page.MustElement("#billingLocality").MustInput("Seattle")
	page.MustElement("#billingPostalCode").MustInput("98109")
	enableStripePass := page.MustElement("#enableStripePass")
	if enableStripePass.MustProperty("checked").Bool() {
		enableStripePass.MustClick()
	}
	page.MustElement(".SubmitButton--complete").MustClick()

	// Ensure ack
	page.MustElement("#request_ack")

	// Ensure shows up on the dashboard
	subscriptionId := mustParseSubscriptionId(page)
	page = visitDev(browser, "subscriptions")
	page.MustElement("a[href='/subscriptions/" + subscriptionId + "/setup']")

	// Ensure makes it into db
	customBlogRequest := visitAdminSql(browser, `
		select feed_url, why, stripe_payment_intent_id from custom_blog_requests
		where subscription_id = `+subscriptionId+`
	`)
	require.Equal(t, "https://ilidemi.github.io/dummy-blogs/1fa/rss.xml", customBlogRequest[0]["feed_url"])
	require.Equal(t, why, customBlogRequest[0]["why"])
	require.True(t, strings.HasPrefix(customBlogRequest[0]["stripe_payment_intent_id"].(string), "pi_"))

	// Ensure credits still at 0
	page = visitDev(browser, "settings")
	patronCreditsText := page.MustElement("#patron_credits").MustText()
	require.Equal(t, "Credits available: 0", patronCreditsText)

	// Ensure slack message
	page = visitAdmin(browser, "get_test_singleton?key=slack_last_message")
	require.Equal(t, "Custom blog requested for subscription "+subscriptionId, mustPageText(page))

	// Cleanup
	page = visitAdmin(browser, "delete_test_singleton?key=slack_dump")
	require.Equal(t, "OK", mustPageText(page))
	page = visitAdmin(browser, "delete_test_singleton?key=slack_last_message")
	require.Equal(t, "OK", mustPageText(page))
	page = visitAdminf(browser, "destroy_user?email=%s", email)
	require.Equal(t, "OK", mustPageText(page))

	browser.MustClose()
	l.Cleanup()
}

func TestStripeSupporterCustomBlogRequest(t *testing.T) {
	email := "test_onboarding@feedrewind.com"
	l, browser := mustSetupStripeBrowser()
	mustSetupMonthlySupporter(t, browser, email)

	// Enable slack dump
	page := visitAdmin(browser, "set_test_singleton?key=slack_dump&value=yes")
	require.Equal(t, "OK", mustPageText(page))

	// Add failing blog
	page = visitDev(browser, "subscriptions/add")
	page.MustElement("#start_url").MustInput("https://ilidemi.github.io/dummy-blogs/1fa/")
	page.MustElement("#discover_go").MustClick()

	// Request support
	page.MustElement("#request_button").MustClick()
	why := fmt.Sprintf("why_%x", rand.Uint64())
	page.MustElement("#why").MustInput(why)
	mustRequireNoElement(t, page, "#credits_available")
	mustRequireNoElement(t, page, "#credits_renew_on")
	page.MustElement("#submit").MustClick()

	// Stripe checkout
	page.MustElement("#cardNumber").MustInput("4242424242424242")
	page.MustElement("#cardExpiry").MustInput("242")
	page.MustElement("#cardCvc").MustInput("424")
	page.MustElement("#billingName").MustInput("2 42")
	page.MustElement("#billingAddressLine1").MustInput("400 Broad Street")
	page.MustElement("#billingLocality").MustInput("Seattle")
	page.MustElement("#billingPostalCode").MustInput("98109")
	enableStripePass := page.MustElement("#enableStripePass")
	if enableStripePass.MustProperty("checked").Bool() {
		enableStripePass.MustClick()
	}
	page.MustElement(".SubmitButton--complete").MustClick()

	// Ensure ack
	page.MustElement("#request_ack")

	// Ensure shows up on the dashboard
	subscriptionId := mustParseSubscriptionId(page)
	page = visitDev(browser, "subscriptions")
	page.MustElement("a[href='/subscriptions/" + subscriptionId + "/setup']")

	// Ensure makes it into db
	customBlogRequest := visitAdminSql(browser, `
		select feed_url, why, stripe_payment_intent_id from custom_blog_requests
		where subscription_id = `+subscriptionId+`
	`)
	require.Equal(t, "https://ilidemi.github.io/dummy-blogs/1fa/rss.xml", customBlogRequest[0]["feed_url"])
	require.Equal(t, why, customBlogRequest[0]["why"])
	require.True(t, strings.HasPrefix(customBlogRequest[0]["stripe_payment_intent_id"].(string), "pi_"))

	// Ensure slack message
	page = visitAdmin(browser, "get_test_singleton?key=slack_last_message")
	require.Equal(t, "Custom blog requested for subscription "+subscriptionId, mustPageText(page))

	// Cleanup
	page = visitAdmin(browser, "delete_test_singleton?key=slack_dump")
	require.Equal(t, "OK", mustPageText(page))
	page = visitAdmin(browser, "delete_test_singleton?key=slack_last_message")
	require.Equal(t, "OK", mustPageText(page))
	page = visitAdminf(browser, "destroy_user?email=%s", email)
	require.Equal(t, "OK", mustPageText(page))

	browser.MustClose()
	l.Cleanup()
}

func TestStripeFreeCustomBlogRequest(t *testing.T) {
	email := "test_onboarding@feedrewind.com"
	l, browser := mustSetupStripeBrowser()

	page := visitAdminf(browser, "destroy_user?email=%s", email)
	require.Contains(t, []string{"OK", "NotFound"}, mustPageText(page))

	// Landing
	page = visitDev(browser, "")
	page.MustElement("#get_started").MustClick()

	// Pricing
	page.MustElement("#signup_free").MustClick()

	// Sign up
	page.MustElement("#email").MustInput(email)
	page.MustElement("#new-password").MustInput("tz123456")
	page.MustElementR("input", "Sign up").MustClick()
	page.MustWaitDOMStable()

	// Enable slack dump
	page = visitAdmin(browser, "set_test_singleton?key=slack_dump&value=yes")
	require.Equal(t, "OK", mustPageText(page))

	// Add failing blog
	page = visitDev(browser, "subscriptions/add")
	page.MustElement("#start_url").MustInput("https://ilidemi.github.io/dummy-blogs/1fa/")
	page.MustElement("#discover_go").MustClick()

	// Request support
	page.MustElement("#request_button").MustClick()
	why := fmt.Sprintf("why_%x", rand.Uint64())
	page.MustElement("#why").MustInput(why)
	mustRequireNoElement(t, page, "#credits_available")
	mustRequireNoElement(t, page, "#credits_renew_on")
	page.MustElement("#submit").MustClick()

	// Stripe checkout
	page.MustElement("#cardNumber").MustInput("4242424242424242")
	page.MustElement("#cardExpiry").MustInput("242")
	page.MustElement("#cardCvc").MustInput("424")
	page.MustElement("#billingName").MustInput("2 42")
	page.MustElement("#billingAddressLine1").MustInput("400 Broad Street")
	page.MustElement("#billingLocality").MustInput("Seattle")
	page.MustElement("#billingPostalCode").MustInput("98109")
	enableStripePass := page.MustElement("#enableStripePass")
	if enableStripePass.MustProperty("checked").Bool() {
		enableStripePass.MustClick()
	}
	page.MustElement(".SubmitButton--complete").MustClick()

	// Ensure ack
	page.MustElement("#request_ack")

	// Ensure shows up on the dashboard
	subscriptionId := mustParseSubscriptionId(page)
	page = visitDev(browser, "subscriptions")
	page.MustElement("a[href='/subscriptions/" + subscriptionId + "/setup']")

	// Ensure makes it into db
	customBlogRequest := visitAdminSql(browser, `
		select feed_url, why, stripe_payment_intent_id from custom_blog_requests
		where subscription_id = `+subscriptionId+`
	`)
	require.Equal(t, "https://ilidemi.github.io/dummy-blogs/1fa/rss.xml", customBlogRequest[0]["feed_url"])
	require.Equal(t, why, customBlogRequest[0]["why"])
	require.True(t, strings.HasPrefix(customBlogRequest[0]["stripe_payment_intent_id"].(string), "pi_"))

	// Ensure slack message
	page = visitAdmin(browser, "get_test_singleton?key=slack_last_message")
	require.Equal(t, "Custom blog requested for subscription "+subscriptionId, mustPageText(page))

	// Cleanup
	page = visitAdmin(browser, "delete_test_singleton?key=slack_dump")
	require.Equal(t, "OK", mustPageText(page))
	page = visitAdmin(browser, "delete_test_singleton?key=slack_last_message")
	require.Equal(t, "OK", mustPageText(page))
	page = visitAdminf(browser, "destroy_user?email=%s", email)
	require.Equal(t, "OK", mustPageText(page))

	browser.MustClose()
	l.Cleanup()
}

//
// Double navigation
//

func TestStripeSupporterDoubleCheckout(t *testing.T) {
	email := "test_onboarding@feedrewind.com"
	l, browser := mustSetupStripeBrowser()
	stalePricingPage := visitDev(browser, "pricing")
	mustSetupMonthlySupporter(t, browser, email)

	// Stale pricing page
	stalePricingPage.MustActivate()
	stalePricingPage.MustElement("#signup_supporter").MustClick()
	stalePricingPage.MustWaitLoad()

	// Checkout
	errorText := stalePricingPage.MustElement("#error-information-popup-content").MustText()
	require.Equal(t, "HTTP ERROR 403", errorText) // not ideal but at least not a double checkout

	// Cleanup
	page := visitAdminf(browser, "destroy_user?email=%s", email)
	require.Equal(t, "OK", mustPageText(page))

	browser.MustClose()
	l.Cleanup()
}

func TestStripePatronRequestCustomBlogTwiceWithCredits(t *testing.T) {
	email := "test_onboarding@feedrewind.com"
	l, browser := mustSetupStripeBrowser()
	mustSetupMonthlyPatron(t, browser, email)

	// Add failing blog
	page := visitDev(browser, "subscriptions/add")
	page.MustElement("#start_url").MustInput("https://ilidemi.github.io/dummy-blogs/1fa/")
	page.MustElement("#discover_go").MustClick()

	// Fill out form 1
	page.MustElement("#request_button").MustClick()
	creditsAvailableText := page.MustElement("#credits_available").MustText()
	require.Equal(t, "Credits available: 1", creditsAvailableText)

	// Fill out form 2
	page2 := browser.MustPage(page.MustInfo().URL)
	creditsAvailableText2 := page2.MustElement("#credits_available").MustText()
	require.Equal(t, "Credits available: 1", creditsAvailableText2)

	// Submit form 1
	page.MustActivate()
	page.MustElement("#submit").MustClick()
	page.MustElement("#request_ack")

	// Submit form 2
	page2.MustActivate()
	page2.MustElement("#submit").MustClick()
	page2.MustElement("#request_ack")

	// Ensure single custom_blog_request
	count := visitAdminSql(browser, `
		select count(1) from custom_blog_requests
		where user_id = (select id from users_without_discarded where email = '`+email+`')
	`)
	require.Equal(t, json.Number("1"), count[0]["count"])

	// Cleanup
	page = visitAdminf(browser, "destroy_user?email=%s", email)
	require.Equal(t, "OK", mustPageText(page))

	browser.MustClose()
	l.Cleanup()
}

func TestStripePatronDoubleSpendCredits(t *testing.T) {
	email := "test_onboarding@feedrewind.com"
	l, browser := mustSetupStripeBrowser()
	mustSetupMonthlyPatron(t, browser, email)

	// Add failing blog 1
	page := visitDev(browser, "subscriptions/add")
	page.MustElement("#start_url").MustInput("https://ilidemi.github.io/dummy-blogs/1fa/")
	page.MustElement("#discover_go").MustClick()

	// Fill out form 1
	page.MustElement("#request_button").MustClick()
	creditsAvailableText := page.MustElement("#credits_available").MustText()
	require.Equal(t, "Credits available: 1", creditsAvailableText)

	// Add failing blog 2
	page2 := visitDev(browser, "subscriptions/add")
	page2.MustElement("#start_url").MustInput("https://ilidemi.github.io/dummy-blogs/1fa/")
	page2.MustElement("#discover_go").MustClick()

	// Fill out form 2
	page2.MustElement("#request_button").MustClick()
	creditsAvailableText2 := page2.MustElement("#credits_available").MustText()
	require.Equal(t, "Credits available: 1", creditsAvailableText2)

	// Submit form 1
	page.MustActivate()
	page.MustElement("#submit").MustClick()
	page.MustElement("#request_ack")

	// Submit form 2
	page2.MustActivate()
	page2.MustElement("#submit").MustClick()
	page2.MustElement("#feedrewind_error")

	// Cleanup
	page = visitAdminf(browser, "destroy_user?email=%s", email)
	require.Equal(t, "OK", mustPageText(page))

	browser.MustClose()
	l.Cleanup()
}

func TestStripeRequestCustomBlogDoubleRedirectAfterCheckout(t *testing.T) {
	email := "test_onboarding@feedrewind.com"
	l, browser := mustSetupStripeBrowser()

	page := visitAdminf(browser, "destroy_user?email=%s", email)
	require.Contains(t, []string{"OK", "NotFound"}, mustPageText(page))

	// Landing
	page = visitDev(browser, "")
	page.MustElement("#get_started").MustClick()

	// Pricing
	page.MustElement("#signup_free").MustClick()

	// Sign up
	page.MustElement("#email").MustInput(email)
	page.MustElement("#new-password").MustInput("tz123456")
	page.MustElementR("input", "Sign up").MustClick()
	page.MustWaitDOMStable()

	// Add failing blog
	page = visitDev(browser, "subscriptions/add")
	page.MustElement("#start_url").MustInput("https://ilidemi.github.io/dummy-blogs/1fa/")
	page.MustElement("#discover_go").MustClick()

	// Request support
	page.MustElement("#request_button").MustClick()
	page.MustElement("#submit").MustClick()

	// Stripe checkout without submitting
	page.MustElement("#cardNumber").MustInput("4242424242424242")
	page.MustElement("#cardExpiry").MustInput("242")
	page.MustElement("#cardCvc").MustInput("424")
	page.MustElement("#billingName").MustInput("2 42")
	page.MustElement("#billingAddressLine1").MustInput("400 Broad Street")
	page.MustElement("#billingLocality").MustInput("Seattle")
	page.MustElement("#billingPostalCode").MustInput("98109")
	enableStripePass := page.MustElement("#enableStripePass")
	if enableStripePass.MustProperty("checked").Bool() {
		enableStripePass.MustClick()
	}

	// Set up hijack for the redirect
	hijackRouter := page.HijackRequests()
	var redirectUrl string
	err := hijackRouter.Add("*", proto.NetworkResourceTypeDocument, func(h *rod.Hijack) {
		url := h.Request.URL()
		if strings.HasPrefix(url.RawQuery, "session_id=") {
			redirectUrl = url.String()
		}
		h.MustLoadResponse()
	})
	oops.RequireNoError(t, err)
	go hijackRouter.Run()

	// Submit the form
	page.MustElement(".SubmitButton--complete").MustClick()

	// Ensure ack
	page.MustElement("#request_ack")

	// Second redirect
	require.NotEmpty(t, redirectUrl)
	page = browser.MustPage(redirectUrl)
	page.MustElement("#request_ack")

	// Cleanup
	err = hijackRouter.Stop()
	oops.RequireNoError(t, err)
	page = visitAdminf(browser, "destroy_user?email=%s", email)
	require.Equal(t, "OK", mustPageText(page))

	browser.MustClose()
	l.Cleanup()
}

func TestStripeCustomBlogRequestDoubleCheckout(t *testing.T) {
	email := "test_onboarding@feedrewind.com"
	l, browser := mustSetupStripeBrowser()

	page := visitAdminf(browser, "destroy_user?email=%s", email)
	require.Contains(t, []string{"OK", "NotFound"}, mustPageText(page))

	// Landing
	page = visitDev(browser, "")
	page.MustElement("#get_started").MustClick()

	// Pricing
	page.MustElement("#signup_free").MustClick()

	// Sign up
	page.MustElement("#email").MustInput(email)
	page.MustElement("#new-password").MustInput("tz123456")
	page.MustElementR("input", "Sign up").MustClick()
	page.MustWaitDOMStable()

	// Enable slack dump
	page = visitAdmin(browser, "set_test_singleton?key=slack_dump&value=yes")
	require.Equal(t, "OK", mustPageText(page))

	// Add failing blog
	page = visitDev(browser, "subscriptions/add")
	page.MustElement("#start_url").MustInput("https://ilidemi.github.io/dummy-blogs/1fa/")
	page.MustElement("#discover_go").MustClick()

	// Request support 1
	page.MustElement("#request_button").MustClick()
	page.MustWaitLoad()
	requestUrl := page.MustInfo().URL
	page.MustElement("#submit").MustClick()

	// Request support 2
	page2 := browser.MustPage(requestUrl)
	page2.MustElement("#submit").MustClick()

	// Stripe checkout 1
	page.MustActivate()
	page.MustElement("#cardNumber").MustInput("4242424242424242")
	page.MustElement("#cardExpiry").MustInput("242")
	page.MustElement("#cardCvc").MustInput("424")
	page.MustElement("#billingName").MustInput("2 42")
	page.MustElement("#billingAddressLine1").MustInput("400 Broad Street")
	page.MustElement("#billingLocality").MustInput("Seattle")
	page.MustElement("#billingPostalCode").MustInput("98109")
	enableStripePass := page.MustElement("#enableStripePass")
	if enableStripePass.MustProperty("checked").Bool() {
		enableStripePass.MustClick()
	}
	page.MustElement(".SubmitButton--complete").MustClick()

	// Ensure ack 1
	page.MustElement("#request_ack")

	// Stripe checkout 2
	page2.MustActivate()
	page2.MustElement("#cardNumber").MustInput("4242424242424242")
	page2.MustElement("#cardExpiry").MustInput("242")
	page2.MustElement("#cardCvc").MustInput("424")
	page2.MustElement("#billingName").MustInput("2 42")
	page2.MustElement("#billingAddressLine1").MustInput("400 Broad Street")
	page2.MustElement("#billingLocality").MustInput("Seattle")
	page2.MustElement("#billingPostalCode").MustInput("98109")
	enableStripePass = page2.MustElement("#enableStripePass")
	if enableStripePass.MustProperty("checked").Bool() {
		enableStripePass.MustClick()
	}
	page2.MustElement(".SubmitButton--complete").MustClick()

	// Ensure ack 2
	page2.MustElement("#request_ack")

	// Ensure slack message
	page.MustActivate()
	page = visitAdmin(browser, "get_test_singleton?key=slack_last_message")
	require.True(t, strings.HasPrefix(
		mustPageText(page),
		"Double payment for the same custom blog request, contact customer asap: ",
	))

	// Cleanup
	page = visitAdmin(browser, "delete_test_singleton?key=slack_dump")
	require.Equal(t, "OK", mustPageText(page))
	page = visitAdmin(browser, "delete_test_singleton?key=slack_last_message")
	require.Equal(t, "OK", mustPageText(page))
	page = visitAdminf(browser, "destroy_user?email=%s", email)
	require.Equal(t, "OK", mustPageText(page))

	browser.MustClose()
	l.Cleanup()
}

//
// helpers
//

func mustSetupStripeBrowser() (*launcher.Launcher, *rod.Browser) {
	l := launcher.New().Headless(false).Preferences(
		`{"autofill":{"credit_card_enabled": false, "profile_enabled": false}}`,
	)
	browserUrl := l.MustLaunch()
	browser := rod.New().ControlURL(browserUrl).MustConnect()
	return l, browser
}

func mustSetupMonthlySupporter(t *testing.T, browser *rod.Browser, email string) {
	mustSetupPaidUser(t, browser, email, "#signup_supporter", false)
}

func mustSetupMonthlyPatron(t *testing.T, browser *rod.Browser, email string) {
	mustSetupPaidUser(t, browser, email, "#signup_patron", false)
}

func mustSetupPaidUser(
	t *testing.T, browser *rod.Browser, email string, checkoutButtonSelector string, yearly bool,
) {
	page := visitAdminf(browser, "destroy_user?email=%s", email)
	require.Contains(t, []string{"OK", "NotFound"}, mustPageText(page))

	page = visitAdminf(browser, "ensure_stripe_listen")
	require.Equal(t, "OK", mustPageText(page))

	// Pricing
	page = visitDev(browser, "pricing")
	if yearly {
		page.MustElement("#billing_interval_toggle").MustClick()
	}
	page.MustElement(checkoutButtonSelector).MustClick()

	// Stripe checkout
	page.MustElement("#email").MustInput(email)
	page.MustElement("#cardNumber").MustInput("4242424242424242")
	page.MustElement("#cardExpiry").MustInput("242")
	page.MustElement("#cardCvc").MustInput("424")
	page.MustElement("#billingName").MustInput("2 42")
	page.MustElement("#billingAddressLine1").MustInput("400 Broad Street")
	page.MustElement("#billingLocality").MustInput("Seattle")
	page.MustElement("#billingPostalCode").MustInput("98109")
	enableStripePass := page.MustElement("#enableStripePass")
	if enableStripePass.MustProperty("checked").Bool() {
		enableStripePass.MustClick()
	}
	page.MustElement(".SubmitButton--complete").MustClick()

	// Create user
	page.MustElement("#new-password").MustInput("tz123456")
	page.MustElementR("input", "Sign up").MustClick()
	page.MustWaitLoad()
}
