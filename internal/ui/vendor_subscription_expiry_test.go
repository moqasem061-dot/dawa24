package ui

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/notifications"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/ui/pages"
)

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
