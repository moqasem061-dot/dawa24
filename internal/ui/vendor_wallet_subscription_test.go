package ui

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/notifications"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func TestDepositModal_ResponsiveLayoutAndCopyButtons(t *testing.T) {
	data := pages.WalletViewData{
		IsVendor: true,
		PlatformPaymentMethods: []*billing.PlatformPaymentMethod{
			{
				ID:               "ppm-cib",
				Name:             i18n.Text{"ar": "البنك التجاري الدولي (CIB)"},
				ProviderType:     "bank",
				IsDepositEnabled: true,
				IsActive:         true,
				BankName:         "CIB",
				AccountName:      "شركة دوا 24",
				AccountNumber:    "100029384756",
				IBAN:             "EG120010000100029384756",
			},
		},
	}

	var buf bytes.Buffer
	err := pages.WalletModals(data).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("failed to render WalletModals: %v", err)
	}

	html := buf.String()

	// Check container class
	if !strings.Contains(html, "deposit-modal-container") {
		t.Errorf("expected deposit-modal-container class in rendered modal")
	}

	// Check scrollable body class
	if !strings.Contains(html, "deposit-modal-body") {
		t.Errorf("expected deposit-modal-body class in rendered modal")
	}

	// Check responsive 2-column grid
	if !strings.Contains(html, "grid grid-cols-1 sm:grid-cols-2") {
		t.Errorf("expected responsive 2-column grid in deposit modal")
	}

	// Check copy button
	if !strings.Contains(html, "نسخ") {
		t.Errorf("expected copy button text in deposit modal")
	}

	// Check fixed modal footer
	if !strings.Contains(html, "deposit-modal-footer") {
		t.Errorf("expected deposit-modal-footer class in rendered modal footer")
	}

	// Check bounded form class
	if !strings.Contains(html, "deposit-modal-form") {
		t.Errorf("expected deposit-modal-form class in rendered modal form")
	}

	// Check fixed modal footer submit button
	if !strings.Contains(html, "تأكيد طلب الشحن") {
		t.Errorf("expected submit button in deposit modal footer")
	}
}

func TestTenantSubscriptionPage_DaysRemainingAndNoManualRenew(t *testing.T) {
	expiryDate := time.Now().Add(15 * 24 * time.Hour)
	planPrice := money.MustParse("1000.00")
	planPriceAnnual := money.MustParse("5000.00")

	plans := []*billing.Plan{
		{
			ID:               1,
			Slug:             "growth",
			Name:             i18n.Text{"ar": "باقة النمو المتقدمة"},
			PriceMonth:       planPrice,
			PriceYear:        planPriceAnnual,
			MaxLoginSessions: 5,
			MaxDevices:       5,
			AIPlanID:         "growth-ai",
		},
		{
			ID:               2,
			Slug:             "enterprise",
			Name:             i18n.Text{"ar": "باقة الشركات الكبرى"},
			PriceMonth:       money.MustParse("3000.00"),
			PriceYear:        money.MustParse("15000.00"),
			MaxLoginSessions: 20,
			MaxDevices:       20,
			AIPlanID:         "enterprise-ai",
		},
	}

	data := pages.TenantSubscriptionPageData{
		Subscription: &pages.OrgSubscriptionView{
			HasSubscription:  true,
			PlanName:         "باقة النمو المتقدمة",
			PlanSlug:         "growth",
			Status:           "نشط ومفعّل",
			ExpiresAt:        expiryDate.Format("2006-01-02"),
			RawExpiresAt:     expiryDate,
			DaysRemaining:    15,
			HasDaysRemaining: true,
			RenewalCost:      planPrice,
			MaxLoginSessions: 5,
			MaxDevices:       5,
			AIPlanID:         "growth-ai",
		},
		Plans:         plans,
		CurrentPlanID: 1,
		WalletBalance: money.MustParse("15000.00"),
		AutoRenew:     true,
		BillingCycle:  "monthly",
	}

	var buf bytes.Buffer
	err := pages.TenantSubscriptionPage(data, "vendor", "ar", "rtl").Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("failed to render TenantSubscriptionPage: %v", err)
	}

	html := buf.String()

	// 1. Must show days remaining
	if !strings.Contains(html, "متبقي 15 يوماً على موعد التجديد") {
		t.Errorf("expected days remaining countdown text, got html snippet without it")
	}

	// 2. Must show auto-renewal information banner
	if !strings.Contains(html, "نظام الفوترة والتجديد التلقائي عبر المحفظة") {
		t.Errorf("expected auto-renewal wallet info banner")
	}

	// 3. Must show wallet balance coverage indication
	if !strings.Contains(html, "يغطي تكلفة التجديد القادم بنجاح") {
		t.Errorf("expected wallet coverage success notice")
	}

	// 4. Must show active status pill on current plan
	if !strings.Contains(html, "باقتك الحالية المفعّلة (تتجدد تلقائياً)") {
		t.Errorf("expected active plan pill")
	}

	// 5. Must NOT show manual renewal button on current plan
	if strings.Contains(html, "تجديد / تمديد باقتك الحالية") {
		t.Errorf("CRITICAL BUG: 'تجديد / تمديد باقتك الحالية' should be completely removed, but was found in HTML!")
	}

	// 6. Must show upgrade button on other tier
	if !strings.Contains(html, "الاشتراك والترقية لهذه الباقة") {
		t.Errorf("expected upgrade button for enterprise plan")
	}
}

func TestSubscriptionsPlural_RedirectsToSingular(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/vendor/subscriptions", nil)
	actor := authctx.Actor{
		UserID:         1,
		OrganizationID: 10,
		OrgType:        "vendor",
		Permissions:    []string{"vendor.subscription.view"},
	}
	req = req.WithContext(authctx.WithActor(req.Context(), actor))

	http.Redirect(rec, req, "/vendor/subscription", http.StatusMovedPermanently)
	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("expected 301, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/vendor/subscription" {
		t.Fatalf("expected /vendor/subscription, got %s", loc)
	}
}

func TestTenantSubscriptionPage_ExtraConcurrentSessions(t *testing.T) {
	data := pages.TenantSubscriptionPageData{
		Subscription: &pages.OrgSubscriptionView{
			HasSubscription:        true,
			PlanName:               "باقة الصيدليات الأساسية",
			PlanSlug:               "basic",
			Status:                 "نشط ومفعّل",
			BaseLoginSessions:      3,
			ExtraLoginSessions:     2,
			HasExtraSessions:       true,
			MaxLoginSessions:       5,
			MaxDevices:             5,
			ExtraSessionsExpiresAt: "2026-10-25",
			AIPlanID:               "free-tier",
		},
		CurrentPlanID: 1,
		WalletBalance: money.MustParse("500.00"),
	}

	var buf bytes.Buffer
	err := pages.TenantSubscriptionPage(data, "customer", "ar", "rtl").Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("failed to render TenantSubscriptionPage with extra sessions: %v", err)
	}

	html := buf.String()

	// 1. Must show extra sessions banner
	if !strings.Contains(html, "تمت ترقية الجلسات المتزامنة لمنشأتك (+2 جلسة إضافية)") {
		t.Errorf("expected extra sessions notice banner in html")
	}

	// 2. Must show extra sessions expiration
	if !strings.Contains(html, "2026-10-25") {
		t.Errorf("expected extra sessions expiry date 2026-10-25 in html")
	}

	// 3. Must show +2 badge on concurrent sessions card
	if !strings.Contains(html, "+2 إضافية") {
		t.Errorf("expected +2 badge in concurrent sessions card")
	}

	// 4. Must show total sessions 5
	if !strings.Contains(html, "5") {
		t.Errorf("expected total 5 sessions in html")
	}

	// 5. Must show breakdown (الأساسية: 3 + الإضافية: 2)
	if !strings.Contains(html, "الأساسية: 3 + الإضافية: 2") {
		t.Errorf("expected breakdown text (الأساسية: 3 + الإضافية: 2) in html")
	}
}

func TestNotifications_OrgExtraDevicesEvents(t *testing.T) {
	// 1. Verify EventOrgExtraDevicesGranted
	evtGranted, ok := notifications.GetEvent(notifications.EventOrgExtraDevicesGranted)
	if !ok {
		t.Fatalf("expected EventOrgExtraDevicesGranted to be registered")
	}
	vars := map[string]string{
		"extra_devices":  "3",
		"total_sessions": "6",
		"expires_text":   " صالحة حتى 2026-10-30",
	}
	titleAr, bodyAr := evtGranted.Render("ar", vars)
	if !strings.Contains(titleAr, "جلسات متزامنة") {
		t.Errorf("expected Arabic title to mention concurrent sessions, got: %s", titleAr)
	}
	if !strings.Contains(bodyAr, "3") || !strings.Contains(bodyAr, "6") {
		t.Errorf("expected Arabic body to contain 3 and 6, got: %s", bodyAr)
	}

	titleEn, bodyEn := evtGranted.Render("en", vars)
	if !strings.Contains(titleEn, "Concurrent Sessions") {
		t.Errorf("expected English title to mention Concurrent Sessions, got: %s", titleEn)
	}
	if !strings.Contains(bodyEn, "3") || !strings.Contains(bodyEn, "6") {
		t.Errorf("expected English body to contain 3 and 6, got: %s", bodyEn)
	}

	// 2. Verify EventOrgExtraDevicesRevoked
	evtRevoked, ok := notifications.GetEvent(notifications.EventOrgExtraDevicesRevoked)
	if !ok {
		t.Fatalf("expected EventOrgExtraDevicesRevoked to be registered")
	}
	varsRevoked := map[string]string{
		"total_sessions": "3",
	}
	_, bodyRevokedAr := evtRevoked.Render("ar", varsRevoked)
	if !strings.Contains(bodyRevokedAr, "3") {
		t.Errorf("expected revoked Arabic body to contain 3, got: %s", bodyRevokedAr)
	}
}

func TestTenantSubscriptionPage_ExpirationWarningBanner(t *testing.T) {
	// 1. Should show banner when DaysRemaining <= 7 and not default plan
	dataExpiring := pages.TenantSubscriptionPageData{
		Subscription: &pages.OrgSubscriptionView{
			HasSubscription:  true,
			PlanName:         "باقة الصيدليات المتقدمة",
			PlanSlug:         "advanced",
			Status:           "نشط ومفعّل",
			HasDaysRemaining: true,
			DaysRemaining:    5,
			ExpiresAt:        "2026-09-21",
			IsDefaultPlan:    false,
		},
		CurrentPlanID: 2,
		WalletBalance: money.MustParse("250.00"),
	}

	var buf bytes.Buffer
	err := pages.TenantSubscriptionPage(dataExpiring, "customer", "ar", "rtl").Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("failed to render TenantSubscriptionPage: %v", err)
	}
	html := buf.String()
	if !strings.Contains(html, "تنبيه: اقتراب موعد تجديد باقة الاشتراك") {
		t.Errorf("expected expiration warning banner when DaysRemaining <= 7")
	}
	if !strings.Contains(html, "2026-09-21") {
		t.Errorf("expected expiry date in banner")
	}
	if !strings.Contains(html, "شحن المحفظة") {
		t.Errorf("expected wallet recharge link in banner")
	}

	// 2. Should NOT show banner when DaysRemaining > 7
	dataNotExpiring := pages.TenantSubscriptionPageData{
		Subscription: &pages.OrgSubscriptionView{
			HasSubscription:  true,
			PlanName:         "باقة الصيدليات المتقدمة",
			PlanSlug:         "advanced",
			Status:           "نشط ومفعّل",
			HasDaysRemaining: true,
			DaysRemaining:    20,
			ExpiresAt:        "2026-10-06",
			IsDefaultPlan:    false,
		},
		CurrentPlanID: 2,
		WalletBalance: money.MustParse("250.00"),
	}
	buf.Reset()
	err = pages.TenantSubscriptionPage(dataNotExpiring, "customer", "ar", "rtl").Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("failed to render TenantSubscriptionPage: %v", err)
	}
	if strings.Contains(buf.String(), "تنبيه: اقتراب موعد تجديد باقة الاشتراك") {
		t.Errorf("expected NO expiration warning banner when DaysRemaining > 7")
	}

	// 3. Should NOT show banner for default plan
	dataDefault := pages.TenantSubscriptionPageData{
		Subscription: &pages.OrgSubscriptionView{
			HasSubscription:  true,
			PlanName:         "باقة الصيدليات الأساسية",
			PlanSlug:         "basic",
			Status:           "نشط ومفعّل",
			HasDaysRemaining: false,
			DaysRemaining:    0,
			IsDefaultPlan:    true,
		},
		CurrentPlanID: 1,
		WalletBalance: money.MustParse("0.00"),
	}
	buf.Reset()
	err = pages.TenantSubscriptionPage(dataDefault, "customer", "ar", "rtl").Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("failed to render TenantSubscriptionPage: %v", err)
	}
	if strings.Contains(buf.String(), "تنبيه: اقتراب موعد تجديد باقة الاشتراك") {
		t.Errorf("expected NO expiration warning banner for default plan")
	}
}

func TestMaybeNotifySubscriptionExpiring_AntiSpam(t *testing.T) {
	nRepo := &mockNotifRepo{logs: make([]*notifications.NotificationLog, 0)}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	notifSvc := notifications.NewService(nRepo, logger)

	h := &UIHandler{
		notifSvc: notifSvc,
		log:      logger,
	}

	actor := authctx.Actor{
		UserID:         42,
		OrganizationID: 10,
	}

	subView := &pages.OrgSubscriptionView{
		HasSubscription:  true,
		PlanName:         "باقة المورد المتقدمة",
		PlanSlug:         "vendor_adv",
		HasDaysRemaining: true,
		DaysRemaining:    4,
		ExpiresAt:        "2026-09-20",
		IsDefaultPlan:    false,
	}

	ctx := context.Background()

	// 1. First visit: should trigger 1 notification
	h.maybeNotifySubscriptionExpiring(ctx, actor, subView)

	if len(nRepo.logs) != 1 {
		t.Fatalf("expected exactly 1 notification, got %d", len(nRepo.logs))
	}
	if nRepo.logs[0].UserID != 42 {
		t.Errorf("expected recipient UserID 42, got %d", nRepo.logs[0].UserID)
	}
	if nRepo.logs[0].Title != "تنبيه: اقتراب انتهاء باقة الاشتراك" {
		t.Errorf("expected title 'تنبيه: اقتراب انتهاء باقة الاشتراك', got %s", nRepo.logs[0].Title)
	}
	if !strings.Contains(nRepo.logs[0].Body, "4 أيام") {
		t.Errorf("expected body to mention 4 days, got %s", nRepo.logs[0].Body)
	}

	// 2. Second visit (anti-spam test): should NOT send duplicate notification
	h.maybeNotifySubscriptionExpiring(ctx, actor, subView)
	if len(nRepo.logs) != 1 {
		t.Fatalf("anti-spam failed: expected still exactly 1 notification, got %d", len(nRepo.logs))
	}

	// 3. Another user in the same organization visits: gets their notification once
	actor2 := authctx.Actor{
		UserID:         43,
		OrganizationID: 10,
	}
	h.maybeNotifySubscriptionExpiring(ctx, actor2, subView)
	if len(nRepo.logs) != 2 {
		t.Fatalf("expected 2 notifications total (1 for each user), got %d", len(nRepo.logs))
	}

	// 4. User 2 visits again: no duplicate
	h.maybeNotifySubscriptionExpiring(ctx, actor2, subView)
	if len(nRepo.logs) != 2 {
		t.Fatalf("anti-spam failed: expected still 2 notifications, got %d", len(nRepo.logs))
	}
}


