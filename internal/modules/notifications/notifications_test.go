package notifications_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/notifications"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

type mockNotificationRepo struct {
	logs      map[int64]*notifications.NotificationLog
	templates map[string]*notifications.Template
	nextID    int64
}

func newMockNotificationRepo() *mockNotificationRepo {
	return &mockNotificationRepo{
		logs:      map[int64]*notifications.NotificationLog{},
		templates: map[string]*notifications.Template{},
		nextID:    1,
	}
}

func (m *mockNotificationRepo) CreateLog(_ context.Context, l *notifications.NotificationLog) error {
	l.ID = m.nextID
	m.nextID++
	if l.CreatedAt.IsZero() {
		l.CreatedAt = time.Now().UTC()
	}
	m.logs[l.ID] = l
	return nil
}

func (m *mockNotificationRepo) GetTemplateBySlug(_ context.Context, slug string) (*notifications.Template, error) {
	t, ok := m.templates[slug]
	if !ok {
		return nil, apperr.NotFound("template")
	}
	return t, nil
}

func (m *mockNotificationRepo) ListUserNotifications(_ context.Context, userID int64, limit, offset int) ([]*notifications.NotificationLog, error) {
	var list []*notifications.NotificationLog
	for _, l := range m.logs {
		if l.UserID == userID {
			list = append(list, l)
		}
	}
	return list, nil
}

func (m *mockNotificationRepo) MarkAsRead(_ context.Context, id int64, userID int64) error {
	l, ok := m.logs[id]
	if !ok || l.UserID != userID {
		return apperr.NotFound("notification")
	}
	l.IsRead = true
	return nil
}

func (m *mockNotificationRepo) GetUnreadCount(_ context.Context, userID int64) (int, error) {
	count := 0
	for _, l := range m.logs {
		if l.UserID == userID && !l.IsRead {
			count++
		}
	}
	return count, nil
}

func TestTemplateInterpolation(t *testing.T) {
	templateText := "مرحباً {name}، تم تأكيد طلبك رقم {order_id}"
	vars := map[string]string{
		"name":     "أحمد",
		"order_id": "ORD-2026-0001",
	}

	res := notifications.InterpolateTemplate(templateText, vars)
	expected := "مرحباً أحمد، تم تأكيد طلبك رقم ORD-2026-0001"
	if res != expected {
		t.Errorf("InterpolateTemplate = %q; want %q", res, expected)
	}
}

func TestSendAndMarkAsRead(t *testing.T) {
	ctx := context.Background()
	repo := newMockNotificationRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := notifications.NewService(repo, logger)

	repo.templates["order_confirmed"] = &notifications.Template{
		Slug:    "order_confirmed",
		Channel: notifications.ChannelSMS,
		Title:   i18n.New("تأكيد الطلب", "Order Confirmed"),
		Body:    i18n.New("تم تأكيد طلبك #{order_number}", "Your order #{order_number} is confirmed"),
	}

	// 1. Send templated notification
	log, err := svc.SendTemplated(ctx, 42, "+201000000000", "order_confirmed", map[string]string{
		"order_number": "ORD-9999",
	})
	if err != nil {
		t.Fatalf("SendTemplated failed: %v", err)
	}

	unread, _ := svc.GetUnreadCount(ctx, 42)
	if unread != 1 {
		t.Errorf("expected 1 unread notification, got %d", unread)
	}

	// 2. Mark as read
	err = svc.MarkRead(ctx, log.ID, 42)
	if err != nil {
		t.Fatalf("MarkRead failed: %v", err)
	}

	unreadAfter, _ := svc.GetUnreadCount(ctx, 42)
	if unreadAfter != 0 {
		t.Errorf("expected 0 unread notifications after mark read, got %d", unreadAfter)
	}
}

func (m *mockNotificationRepo) MarkAllAsRead(_ context.Context, userID int64) (int64, error) {
	var n int64
	for _, l := range m.logs {
		if l.UserID == userID && !l.IsRead {
			l.IsRead = true
			n++
		}
	}
	return n, nil
}

func (m *mockNotificationRepo) ListUnread(_ context.Context, userID int64, limit, offset int) ([]*notifications.NotificationLog, error) {
	var list []*notifications.NotificationLog
	for _, l := range m.logs {
		if l.UserID == userID && !l.IsRead {
			list = append(list, l)
		}
	}
	if offset >= len(list) {
		return nil, nil
	}
	end := offset + limit
	if limit <= 0 || end > len(list) {
		end = len(list)
	}
	return list[offset:end], nil
}

func (m *mockNotificationRepo) HasNotificationWithTitle(_ context.Context, userID int64, title string, since time.Time) (bool, error) {
	for _, l := range m.logs {
		if l.UserID == userID && l.Title == title && (since.IsZero() || !l.CreatedAt.Before(since)) {
			return true, nil
		}
	}
	return false, nil
}

func TestHasNotificationWithTitle(t *testing.T) {
	ctx := context.Background()
	repo := newMockNotificationRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := notifications.NewService(repo, logger)

	// Before sending, should be false
	has, err := svc.HasNotificationWithTitle(ctx, 100, "تنبيه: اقتراب انتهاء باقة الاشتراك", time.Now().Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if has {
		t.Errorf("expected false before notification sent")
	}

	// Send notification
	_, err = svc.Send(ctx, notifications.SendInput{
		UserID:    100,
		Channel:   notifications.ChannelInApp,
		Recipient: "user-100",
		Title:     "تنبيه: اقتراب انتهاء باقة الاشتراك",
		Body:      "سينتهي اشتراكك قريباً",
	})
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Now should be true
	has, err = svc.HasNotificationWithTitle(ctx, 100, "تنبيه: اقتراب انتهاء باقة الاشتراك", time.Now().Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !has {
		t.Errorf("expected true after notification sent")
	}

	// For a different user, should be false
	hasDiffUser, err := svc.HasNotificationWithTitle(ctx, 200, "تنبيه: اقتراب انتهاء باقة الاشتراك", time.Now().Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hasDiffUser {
		t.Errorf("expected false for different user")
	}

	// For a future timestamp, should be false
	hasFuture, err := svc.HasNotificationWithTitle(ctx, 100, "تنبيه: اقتراب انتهاء باقة الاشتراك", time.Now().Add(1*time.Hour))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hasFuture {
		t.Errorf("expected false for future since timestamp")
	}
}


