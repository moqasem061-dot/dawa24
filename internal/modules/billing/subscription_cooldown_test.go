package billing

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func setupCooldownTest() (*Service, *mockBillingRepo) {
	repo := newMockBillingRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(repo, logger)

	repo.plans["basic"] = &Plan{
		ID:         1,
		Slug:       "basic",
		Name:       i18n.Text{"ar": "باقة أساسية", "en": "Basic Plan"},
		PriceMonth: money.Zero,
		PriceYear:  money.Zero,
		IsDefault:  true,
		IsActive:   true,
	}
	repo.plans["pro"] = &Plan{
		ID:         2,
		Slug:       "pro",
		Name:       i18n.Text{"ar": "باقة المحترفين", "en": "Pro Plan"},
		PriceMonth: money.MustParse("200.00"),
		PriceYear:  money.MustParse("2000.00"),
		IsDefault:  false,
		IsActive:   true,
	}
	repo.plans["enterprise"] = &Plan{
		ID:         3,
		Slug:       "enterprise",
		Name:       i18n.Text{"ar": "باقة المؤسسات", "en": "Enterprise Plan"},
		PriceMonth: money.MustParse("500.00"),
		PriceYear:  money.MustParse("5000.00"),
		IsDefault:  false,
		IsActive:   true,
	}

	return svc, repo
}

// WO-15 Rule 1: Upgrades from a free/default plan are exempt from cooldown.
func TestSubscribeWithWallet_FreePlanExemptFromCooldown(t *testing.T) {
	ctx := context.Background()
	svc, _ := setupCooldownTest()

	userID := int64(10)
	orgID := int64(100)
	_, _ = svc.Deposit(ctx, userID, "EGP", money.MustParse("1000.00"), "deposit", nil, "Deposit")

	// 1. Assign basic (free) plan
	_, err := svc.SubscribeWithWallet(ctx, userID, &orgID, "basic", "monthly", true)
	if err != nil {
		t.Fatalf("failed to subscribe to basic: %v", err)
	}

	// 2. Immediate upgrade from basic to pro must SUCCEED without cooldown
	sub, err := svc.SubscribeWithWallet(ctx, userID, &orgID, "pro", "monthly", true)
	if err != nil {
		t.Fatalf("upgrade from free plan failed unexpectedly: %v", err)
	}
	if sub.PlanID != 2 {
		t.Errorf("expected upgraded plan ID 2, got %d", sub.PlanID)
	}
}

// WO-15 Rule 2 & 3: Paid plan change within cooldown period is rejected with apperr.Conflict.
func TestSubscribeWithWallet_PaidPlanEnforcesCooldown(t *testing.T) {
	ctx := context.Background()
	svc, _ := setupCooldownTest()

	userID := int64(10)
	orgID := int64(100)
	_, _ = svc.Deposit(ctx, userID, "EGP", money.MustParse("2000.00"), "deposit", nil, "Deposit")

	// 1. Subscribe to pro (paid plan) today
	_, err := svc.SubscribeWithWallet(ctx, userID, &orgID, "pro", "monthly", true)
	if err != nil {
		t.Fatalf("failed to subscribe to pro: %v", err)
	}

	// Verify cooldown status
	info, err := svc.CheckSubscriptionChangeCooldown(ctx, userID, &orgID, "enterprise")
	if err != nil {
		t.Fatalf("CheckSubscriptionChangeCooldown failed: %v", err)
	}
	if !info.IsOnCooldown {
		t.Errorf("expected IsOnCooldown to be true")
	}
	if info.CooldownHours != 24 {
		t.Errorf("expected 24 cooldown hours, got %d", info.CooldownHours)
	}
	if info.CooldownDays != 1 {
		t.Errorf("expected 1 cooldown day, got %d", info.CooldownDays)
	}

	// 2. Attempt to change to enterprise immediately -> MUST be refused with Conflict
	_, err = svc.SubscribeWithWallet(ctx, userID, &orgID, "enterprise", "monthly", true)
	if err == nil {
		t.Fatalf("expected plan change within cooldown to fail, but it succeeded!")
	}

	appErr, ok := apperr.As(err)
	if !ok {
		t.Fatalf("expected apperr.Error, got %T: %v", err, err)
	}
	if appErr.Kind != apperr.KindConflict {
		t.Errorf("expected KindConflict, got %s", appErr.Kind)
	}
	if appErr.Code != "subscription.change_cooldown" {
		t.Errorf("expected code 'subscription.change_cooldown', got %q", appErr.Code)
	}
	if !info.EarliestAllowedAt.After(time.Now().UTC()) {
		t.Errorf("expected earliest allowed date to be in future, got %v", info.EarliestAllowedAt)
	}
}

// WO-15: Plan change is allowed after the 24 hours cooldown period has elapsed.
func TestSubscribeWithWallet_PaidPlanAllowedAfterCooldown(t *testing.T) {
	ctx := context.Background()
	svc, repo := setupCooldownTest()

	userID := int64(10)
	orgID := int64(100)
	_, _ = svc.Deposit(ctx, userID, "EGP", money.MustParse("2000.00"), "deposit", nil, "Deposit")

	// 1. Create a pro subscription that started 25 hours ago (> 24 hours cooldown)
	now := time.Now().UTC()
	sub := &Subscription{
		ID:             1,
		UserID:         userID,
		OrganizationID: &orgID,
		PlanID:         2, // pro
		Status:         SubActive,
		BillingCycle:   "monthly",
		StartsAt:       now.Add(-25 * time.Hour),
		ExpiresAt:      now.Add(29 * 24 * time.Hour),
	}
	repo.subscriptions[sub.ID] = sub

	// Check cooldown status -> must NOT be on cooldown
	info, err := svc.CheckSubscriptionChangeCooldown(ctx, userID, &orgID, "enterprise")
	if err != nil {
		t.Fatalf("CheckSubscriptionChangeCooldown failed: %v", err)
	}
	if info.IsOnCooldown {
		t.Errorf("expected IsOnCooldown to be false after 25 hours")
	}

	// 2. Change to enterprise -> must SUCCEED
	upSub, err := svc.SubscribeWithWallet(ctx, userID, &orgID, "enterprise", "monthly", true)
	if err != nil {
		t.Fatalf("plan change after cooldown failed: %v", err)
	}
	if upSub.PlanID != 3 {
		t.Errorf("expected plan ID 3 (enterprise), got %d", upSub.PlanID)
	}
}

// Plan change within 24 hours (e.g. after 12 hours) is strictly blocked.
func TestSubscribeWithWallet_BlockedWithin24Hours(t *testing.T) {
	ctx := context.Background()
	svc, repo := setupCooldownTest()

	userID := int64(10)
	orgID := int64(100)
	_, _ = svc.Deposit(ctx, userID, "EGP", money.MustParse("2000.00"), "deposit", nil, "Deposit")

	// Started 12 hours ago (within 24 hours)
	now := time.Now().UTC()
	sub := &Subscription{
		ID:             1,
		UserID:         userID,
		OrganizationID: &orgID,
		PlanID:         2, // pro
		Status:         SubActive,
		BillingCycle:   "monthly",
		StartsAt:       now.Add(-12 * time.Hour),
		ExpiresAt:      now.Add(29 * 24 * time.Hour),
	}
	repo.subscriptions[sub.ID] = sub

	info, err := svc.CheckSubscriptionChangeCooldown(ctx, userID, &orgID, "enterprise")
	if err != nil {
		t.Fatalf("CheckSubscriptionChangeCooldown failed: %v", err)
	}
	if !info.IsOnCooldown {
		t.Errorf("expected IsOnCooldown to be true at 12 hours")
	}

	_, err = svc.SubscribeWithWallet(ctx, userID, &orgID, "enterprise", "monthly", true)
	if err == nil {
		t.Fatalf("expected plan change within 24h to fail, but it succeeded")
	}
	appErr, ok := apperr.As(err)
	if !ok || appErr.Code != "subscription.change_cooldown" {
		t.Errorf("expected subscription.change_cooldown, got %v", err)
	}
}

// WO-15: Custom cooldown settings configured in platform_admin.system_settings are respected.
func TestSubscribeWithWallet_ConfigurableCooldown(t *testing.T) {
	ctx := context.Background()
	svc, repo := setupCooldownTest()

	// Configure 10 days cooldown instead of default 25
	repo.settings["subscription_change_cooldown_days"] = "{\"value\": 10, \"days\": 10}"
	repo.settings["subscription_change_min_days"] = "{\"value\": 2, \"days\": 2}"

	userID := int64(10)
	orgID := int64(100)
	_, _ = svc.Deposit(ctx, userID, "EGP", money.MustParse("2000.00"), "deposit", nil, "Deposit")

	now := time.Now().UTC()
	// Started 12 days ago -> past 10 days cooldown
	sub := &Subscription{
		ID:             1,
		UserID:         userID,
		OrganizationID: &orgID,
		PlanID:         2, // pro
		Status:         SubActive,
		BillingCycle:   "monthly",
		StartsAt:       now.Add(-12 * 24 * time.Hour),
		ExpiresAt:      now.Add(18 * 24 * time.Hour),
	}
	repo.subscriptions[sub.ID] = sub

	info, _ := svc.CheckSubscriptionChangeCooldown(ctx, userID, &orgID, "enterprise")
	if info.IsOnCooldown {
		t.Errorf("expected IsOnCooldown to be false after 12 days when cooldown is 10 days")
	}
	if info.CooldownDays != 10 {
		t.Errorf("expected CooldownDays to be 10, got %d", info.CooldownDays)
	}

	upSub, err := svc.SubscribeWithWallet(ctx, userID, &orgID, "enterprise", "monthly", true)
	if err != nil {
		t.Fatalf("plan change failed: %v", err)
	}
	if upSub.PlanID != 3 {
		t.Errorf("expected enterprise plan ID 3, got %d", upSub.PlanID)
	}
}

// Custom subscription_change_cooldown_hours setting is respected.
func TestSubscribeWithWallet_ConfigurableCooldownHours(t *testing.T) {
	ctx := context.Background()
	svc, repo := setupCooldownTest()

	// Configure 48 hours cooldown
	repo.settings["subscription_change_cooldown_hours"] = "{\"value\": 48, \"hours\": 48}"

	userID := int64(10)
	orgID := int64(100)
	_, _ = svc.Deposit(ctx, userID, "EGP", money.MustParse("2000.00"), "deposit", nil, "Deposit")

	now := time.Now().UTC()
	sub := &Subscription{
		ID:             1,
		UserID:         userID,
		OrganizationID: &orgID,
		PlanID:         2, // pro
		Status:         SubActive,
		BillingCycle:   "monthly",
		StartsAt:       now.Add(-30 * time.Hour), // 30 hours ago, less than 48 hours
		ExpiresAt:      now.Add(28 * 24 * time.Hour),
	}
	repo.subscriptions[sub.ID] = sub

	info, _ := svc.CheckSubscriptionChangeCooldown(ctx, userID, &orgID, "enterprise")
	if !info.IsOnCooldown {
		t.Errorf("expected IsOnCooldown to be true after 30h when cooldown is 48h")
	}
	if info.CooldownHours != 48 {
		t.Errorf("expected CooldownHours to be 48, got %d", info.CooldownHours)
	}
}

