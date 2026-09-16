package ui

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func TestTenantSubscriptionPage_CooldownActive_DisablesPlanButtons(t *testing.T) {
	cooldownUntil := time.Now().Add(20 * 24 * time.Hour)
	cooldownUntilText := cooldownUntil.Format("2006-01-02")

	plans := []*billing.Plan{
		{
			ID:               1,
			Slug:             "starter",
			Name:             i18n.Text{"ar": "باقة البداية"},
			PriceMonth:       money.MustParse("500.00"),
			PriceYear:        money.MustParse("5000.00"),
			MaxLoginSessions: 2,
			MaxDevices:       2,
			AIPlanID:         "starter-ai",
		},
		{
			ID:               2,
			Slug:             "growth",
			Name:             i18n.Text{"ar": "باقة النمو"},
			PriceMonth:       money.MustParse("1000.00"),
			PriceYear:        money.MustParse("10000.00"),
			MaxLoginSessions: 5,
			MaxDevices:       5,
			AIPlanID:         "growth-ai",
		},
	}

	data := pages.TenantSubscriptionPageData{
		Subscription: &pages.OrgSubscriptionView{
			HasSubscription:  true,
			PlanName:         "باقة البداية",
			PlanSlug:         "starter",
			Status:           "نشط ومفعّل",
			ExpiresAt:        time.Now().Add(25 * 24 * time.Hour).Format("2006-01-02"),
			DaysRemaining:    25,
			HasDaysRemaining: true,
			RenewalCost:      money.MustParse("500.00"),
			MaxLoginSessions: 2,
			MaxDevices:       2,
			AIPlanID:         "starter-ai",
		},
		Plans:             plans,
		CurrentPlanID:     1,
		WalletBalance:     money.MustParse("5000.00"),
		AutoRenew:         true,
		BillingCycle:      "monthly",
		CooldownActive:    true,
		CooldownUntil:     cooldownUntil,
		CooldownUntilText: cooldownUntilText,
		CooldownDays:      25,
	}

	var buf bytes.Buffer
	err := pages.TenantSubscriptionPage(data, "customer", "ar", "rtl").Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("failed to render TenantSubscriptionPage: %v", err)
	}

	html := buf.String()

	// 1. Current plan should show active status pill
	if !strings.Contains(html, "باقتك الحالية المفعّلة (تتجدد تلقائياً)") {
		t.Errorf("expected active plan pill for current plan")
	}

	// 2. Non-current plan button must be disabled
	if !strings.Contains(html, "الترقية معطلة مؤقتاً") {
		t.Errorf("expected 'الترقية معطلة مؤقتاً' text for disabled button during cooldown")
	}

	if !strings.Contains(html, "disabled") {
		t.Errorf("expected disabled attribute on button during cooldown")
	}

	// 3. Must show inline explanation naming the date
	expectedMsg := "متاح تغيير الباقة بعد انتهاء فترة التهدئة في <strong>" + cooldownUntilText + "</strong>"
	if !strings.Contains(html, expectedMsg) {
		t.Errorf("expected cooldown explanation naming date: %s", expectedMsg)
	}
}

func TestTenantSubscriptionPage_CooldownInactive_EnablesPlanButtons(t *testing.T) {
	plans := []*billing.Plan{
		{
			ID:               1,
			Slug:             "free",
			Name:             i18n.Text{"ar": "الباقة الأساسية المجانية"},
			PriceMonth:       money.Zero,
			PriceYear:        money.Zero,
			IsDefault:        true,
			MaxLoginSessions: 1,
			MaxDevices:       1,
			AIPlanID:         "free-ai",
		},
		{
			ID:               2,
			Slug:             "growth",
			Name:             i18n.Text{"ar": "باقة النمو المتقدمة"},
			PriceMonth:       money.MustParse("1000.00"),
			PriceYear:        money.MustParse("10000.00"),
			MaxLoginSessions: 5,
			MaxDevices:       5,
			AIPlanID:         "growth-ai",
		},
	}

	data := pages.TenantSubscriptionPageData{
		Subscription: &pages.OrgSubscriptionView{
			HasSubscription:  true,
			PlanName:         "الباقة الأساسية المجانية",
			PlanSlug:         "free",
			IsDefaultPlan:    true,
			Status:           "نشط ومفعّل",
			MaxLoginSessions: 1,
			MaxDevices:       1,
			AIPlanID:         "free-ai",
		},
		Plans:          plans,
		CurrentPlanID:  1,
		WalletBalance:  money.MustParse("5000.00"),
		CooldownActive: false,
	}

	var buf bytes.Buffer
	err := pages.TenantSubscriptionPage(data, "vendor", "ar", "rtl").Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("failed to render TenantSubscriptionPage: %v", err)
	}

	html := buf.String()

	// Upgrading from free plan should not show cooldown disabled button
	if strings.Contains(html, "الترقية معطلة مؤقتاً") {
		t.Errorf("expected buttons to be enabled when cooldown is not active")
	}

	// Should show upgrade CTA with click handler
	if !strings.Contains(html, "openCheckout") || !strings.Contains(html, "data-plan-slug=\"growth\"") {
		t.Errorf("expected openCheckout handler on plan button for growth")
	}
	if !strings.Contains(html, "الاشتراك والترقية لهذه الباقة") {
		t.Errorf("expected 'الاشتراك والترقية لهذه الباقة' text on enabled button")
	}
}

func TestTenantSubscriptionModals_ConfirmationModalContent(t *testing.T) {
	data := pages.TenantSubscriptionPageData{
		WalletBalance: money.MustParse("2000.00"),
		CooldownDays:  25,
	}

	var buf bytes.Buffer
	err := pages.TenantSubscriptionModals(data, "customer").Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("failed to render TenantSubscriptionModals: %v", err)
	}

	html := buf.String()

	// 1. Check second confirmation modal exists
	if !strings.Contains(html, "confirm-plan-modal") {
		t.Errorf("expected confirm-plan-modal in rendered modals")
	}

	// 2. Check title
	if !strings.Contains(html, "تأكيد الاشتراك وتغيير الباقة") {
		t.Errorf("expected confirmation modal title 'تأكيد الاشتراك وتغيير الباقة'")
	}

	// 3. Check clean restatement items: Plan, Cycle, Amount, Cooldown finality notice
	if !strings.Contains(html, "الباقة المختارة:") {
		t.Errorf("expected 'الباقة المختارة:' in confirmation modal")
	}
	if !strings.Contains(html, "دورة الفوترة:") {
		t.Errorf("expected 'دورة الفوترة:' in confirmation modal")
	}
	if !strings.Contains(html, "المبلغ المخصوم من المحفظة:") {
		t.Errorf("expected 'المبلغ المخصوم من المحفظة:' in confirmation modal")
	}
	if !strings.Contains(html, "تنبيه نهائي: هذا التغيير نهائي ولا يمكن تغيير الباقة مرة أخرى طوال فترة التهدئة (25 يوماً).") {
		t.Errorf("expected cooldown finality warning notice in confirmation modal")
	}

	// 4. Check action URL
	if !strings.Contains(html, "action=\"/customer/subscription/checkout\"") {
		t.Errorf("expected form action to be /customer/subscription/checkout")
	}

	// 5. Check confirm button
	if !strings.Contains(html, "تأكيد الخصم المباشر من المحفظة") {
		t.Errorf("expected submit button 'تأكيد الخصم المباشر من المحفظة'")
	}
}

func TestTenantSubscriptionModals_24HoursCooldownLabel(t *testing.T) {
	data := pages.TenantSubscriptionPageData{
		WalletBalance: money.MustParse("2000.00"),
		CooldownHours: 24,
	}

	var buf bytes.Buffer
	err := pages.TenantSubscriptionModals(data, "vendor").Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("failed to render TenantSubscriptionModals: %v", err)
	}

	html := buf.String()
	expectedNotice := "تنبيه نهائي: هذا التغيير نهائي ولا يمكن تغيير الباقة مرة أخرى طوال فترة التهدئة (24 ساعة)."
	if !strings.Contains(html, expectedNotice) {
		t.Errorf("expected 24 hours cooldown notice in modal, got: %s", html)
	}
	if !strings.Contains(html, "action=\"/vendor/subscription/checkout\"") {
		t.Errorf("expected action to be /vendor/subscription/checkout")
	}
}
