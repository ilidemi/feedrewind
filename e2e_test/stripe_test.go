//go:build stripetesting

package e2etest

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/stretchr/testify/require"
)

func TestStripeFree(t *testing.T) {
	email := "test_onboarding@feedrewind.com"

	l := launcher.New().Headless(false)
	browserUrl := l.MustLaunch()
	browser := rod.New().ControlURL(browserUrl).MustConnect()

	page := visitAdminf(browser, "destroy_user?email=%s", email)
	require.Contains(t, []string{"OK", "NotFound"}, pageText(page))

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
	currentPlan := page.MustElement("#current_plan")
	require.Equal(t, "Current plan: Free", currentPlan.MustText())

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
	require.Equal(t, "OK", pageText(page))

	browser.MustClose()
	l.Cleanup()
}

func TestStripeSupporter(t *testing.T) {
	email := "test_onboarding@feedrewind.com"
	l, browser := mustSetupStripeBrowser()

	for _, interval := range []string{"monthly", "yearly"} {
		page := visitAdminf(browser, "destroy_user?email=%s", email)
		require.Contains(t, []string{"OK", "NotFound"}, pageText(page))

		page = visitAdminf(browser, "ensure_stripe_listen")
		require.Equal(t, "OK", pageText(page))

		// Pricing
		page = visitDev(browser, "pricing")
		page.MustElement("#signup_supporter_" + interval).MustClick()

		// Stripe checkout
		page.MustElement("#email").MustInput(email)
		page.MustElement("#cardNumber").MustInput("4242424242424242")
		page.MustElement("#cardExpiry").MustInput("242")
		page.MustElement("#cardCvc").MustInput("424")
		page.MustElement("#billingName").MustInput("2 42")
		page.MustElement("#billingPostalCode").MustInput("42424")
		enableStripePass := page.MustElement("#enableStripePass")
		if enableStripePass.MustProperty("checked").Bool() {
			enableStripePass.MustClick()
		}
		page.MustElement("button[type='submit']").MustClick()

		// Create user
		page.MustElement("#new-password").MustInput("tz123456")
		page.MustElementR("input", "Sign up").MustClick()

		// Settings
		page.MustWaitLoad()
		page = visitDev(browser, "settings")
		currentPlan := page.MustElement("#current_plan")
		require.Equal(t, "Current plan: Supporter", currentPlan.MustText())

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
		require.Equal(t, "OK", pageText(page))
	}

	browser.MustClose()
	l.Cleanup()
}

func TestStripeSupporterDoubleCheckout(t *testing.T) {
	email := "test_onboarding@feedrewind.com"
	l, browser := mustSetupStripeBrowser()
	stalePricingPage := visitDev(browser, "pricing")
	mustSetupMonthlySupporter(t, browser, email)

	// Stale pricing page
	stalePricingPage.MustActivate()
	stalePricingPage.MustElement("#signup_supporter_monthly").MustClick()
	stalePricingPage.MustWaitLoad()

	// Checkout
	errorText := stalePricingPage.MustElement("#error-information-popup-content").MustText()
	require.Equal(t, "HTTP ERROR 403", errorText) // not ideal but at least not a double checkout

	// Cleanup
	page := visitAdminf(browser, "destroy_user?email=%s", email)
	require.Equal(t, "OK", pageText(page))

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
	require.True(t, strings.HasPrefix(currentPlanText, "Current plan: Supporter (ends on"))
	require.True(t,
		strings.HasSuffix(currentPlanText, fmt.Sprintf(", %d)", time.Now().Year())) ||
			strings.HasSuffix(currentPlanText, fmt.Sprintf(", %d)", time.Now().Year()+1)),
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
	require.NoError(t, err)
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
	currentPlan := page.MustElement("#current_plan")
	require.Equal(t, "Current plan: Supporter", currentPlan.MustText())

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
	require.Equal(t, "OK", pageText(page))

	browser.MustClose()
	l.Cleanup()
}

func TestStripeSupporterCancelExpire(t *testing.T) {
	email := "test_onboarding@feedrewind.com"
	l, browser := mustSetupStripeBrowser()

	// Enable test clock
	page := visitAdmin(browser, "set_test_singleton?key=test_clock&value=yes")
	require.Equal(t, "OK", pageText(page))

	mustSetupMonthlySupporter(t, browser, email)

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
	page = visitAdminf(browser, "forward_stripe_customer_45_days?email=%s", email)
	require.Equal(t, "OK", pageText(page))

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
	currentPlan := page.MustElement("#current_plan")
	require.Equal(t, "Current plan: Free", currentPlan.MustText())

	// Stale settings, link to the billing portal doesn't do anything
	staleSettingsPage.Activate()
	staleSettingsPage.MustElement("#manage_billing").MustClick()
	staleSettingsPage.MustWaitLoad()
	require.Equal(t, "http://localhost:3000/settings", staleSettingsPage.MustInfo().URL)
	currentPlan = staleSettingsPage.MustElement("#current_plan")
	require.Equal(t, "Current plan: Free", currentPlan.MustText())

	// Cleanup
	page = visitAdmin(browser, "delete_stripe_clocks")
	require.Equal(t, "OK", pageText(page))
	page = visitAdmin(browser, "delete_test_singleton?key=test_clock")
	require.Equal(t, "OK", pageText(page))
	page = visitAdminf(browser, "destroy_user?email=%s", email)
	require.Equal(t, "OK", pageText(page))

	browser.MustClose()
	l.Cleanup()
}

func TestStripeSupporterDeleteOnBackend(t *testing.T) {
	email := "test_onboarding@feedrewind.com"
	l, browser := mustSetupStripeBrowser()
	mustSetupMonthlySupporter(t, browser, email)

	// Delete
	page := visitAdminf(browser, "delete_stripe_subscription?email=%s", email)
	require.Equal(t, "OK", pageText(page))

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
	currentPlan := page.MustElement("#current_plan")
	require.Equal(t, "Current plan: Free", currentPlan.MustText())

	// Cleanup
	page = visitAdminf(browser, "destroy_user?email=%s", email)
	require.Equal(t, "OK", pageText(page))

	browser.MustClose()
	l.Cleanup()
}

func TestStripeSupporterDeleteCanceledOnBackend(t *testing.T) {
	email := "test_onboarding@feedrewind.com"
	l, browser := mustSetupStripeBrowser()
	mustSetupMonthlySupporter(t, browser, email)

	// Pricing
	page := visitDev(browser, "pricing")
	page.MustElement("#current_supporter_monthly")
	page.MustElement("#current_supporter_yearly")
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
	require.Equal(t, "OK", pageText(page))

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
	currentPlan := page.MustElement("#current_plan")
	require.Equal(t, "Current plan: Free", currentPlan.MustText())

	// Cleanup
	page = visitAdminf(browser, "destroy_user?email=%s", email)
	require.Equal(t, "OK", pageText(page))

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
	require.Equal(t, "OK", pageText(page))

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
	require.Equal(t, "OK", pageText(page))

	browser.MustClose()
	l.Cleanup()
}

func TestStripeFreeUpgradeToMonthly(t *testing.T) {
	email := "test_onboarding@feedrewind.com"
	l, browser := mustSetupStripeBrowser()

	page := visitAdminf(browser, "destroy_user?email=%s", email)
	require.Contains(t, []string{"OK", "NotFound"}, pageText(page))

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
	page.MustElement("#upgrade_supporter_monthly").MustClick()

	// Stripe checkout
	page.MustElement("#cardNumber").MustInput("4242424242424242")
	page.MustElement("#cardExpiry").MustInput("242")
	page.MustElement("#cardCvc").MustInput("424")
	page.MustElement("#billingName").MustInput("2 42")
	page.MustElement("#billingPostalCode").MustInput("42424")
	enableStripePass := page.MustElement("#enableStripePass")
	if enableStripePass.MustProperty("checked").Bool() {
		enableStripePass.MustClick()
	}
	page.MustElement("button[type='submit']").MustClick()

	// Settings
	currentPlan := page.MustElement("#current_plan")
	require.Equal(t, "Current plan: Supporter", currentPlan.MustText())

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
	require.Equal(t, nil, userProperties[0]["stripe_cancel_at"])

	// Cleanup
	page = visitAdminf(browser, "destroy_user?email=%s", email)
	require.Equal(t, "OK", pageText(page))

	browser.MustClose()
	l.Cleanup()
}

func TestStripeFreeUpgradeToYearly(t *testing.T) {
	email := "test_onboarding@feedrewind.com"
	l, browser := mustSetupStripeBrowser()

	page := visitAdminf(browser, "destroy_user?email=%s", email)
	require.Contains(t, []string{"OK", "NotFound"}, pageText(page))

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
	page.MustElement("#upgrade_supporter_yearly").MustClick()

	// Stripe checkout
	page.MustElement("#cardNumber").MustInput("4242424242424242")
	page.MustElement("#cardExpiry").MustInput("242")
	page.MustElement("#cardCvc").MustInput("424")
	page.MustElement("#billingName").MustInput("2 42")
	page.MustElement("#billingPostalCode").MustInput("42424")
	enableStripePass := page.MustElement("#enableStripePass")
	if enableStripePass.MustProperty("checked").Bool() {
		enableStripePass.MustClick()
	}
	page.MustElement("button[type='submit']").MustClick()

	// Settings
	currentPlan := page.MustElement("#current_plan")
	require.Equal(t, "Current plan: Supporter", currentPlan.MustText())

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
	require.Equal(t, "OK", pageText(page))

	browser.MustClose()
	l.Cleanup()
}

func mustSetupStripeBrowser() (*launcher.Launcher, *rod.Browser) {
	l := launcher.New().Headless(false).Preferences(`{"autofill":{"credit_card_enabled": false}}`)
	browserUrl := l.MustLaunch()
	browser := rod.New().ControlURL(browserUrl).MustConnect()
	return l, browser
}

func mustSetupMonthlySupporter(t *testing.T, browser *rod.Browser, email string) {
	page := visitAdminf(browser, "destroy_user?email=%s", email)
	require.Contains(t, []string{"OK", "NotFound"}, pageText(page))

	page = visitAdminf(browser, "ensure_stripe_listen")
	require.Equal(t, "OK", pageText(page))

	// Pricing
	page = visitDev(browser, "pricing")
	page.MustElement("#signup_supporter_monthly").MustClick()

	// Stripe checkout
	page.MustElement("#email").MustInput(email)
	page.MustElement("#cardNumber").MustInput("4242424242424242")
	page.MustElement("#cardExpiry").MustInput("242")
	page.MustElement("#cardCvc").MustInput("424")
	page.MustElement("#billingName").MustInput("2 42")
	page.MustElement("#billingPostalCode").MustInput("42424")
	enableStripePass := page.MustElement("#enableStripePass")
	if enableStripePass.MustProperty("checked").Bool() {
		enableStripePass.MustClick()
	}
	page.MustElement("button[type='submit']").MustClick()

	// Create user
	page.MustElement("#new-password").MustInput("tz123456")
	page.MustElementR("input", "Sign up").MustClick()
	page.MustWaitLoad()
}
