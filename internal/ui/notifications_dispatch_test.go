package ui

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/muhiya/dawa24-store/internal/modules/notifications"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

type mockNotifRepo struct {
	logs   []*notifications.NotificationLog
	nextID int64
}

func (m *mockNotifRepo) CreateLog(_ context.Context, l *notifications.NotificationLog) error {
	m.nextID++
	l.ID = m.nextID
	if l.CreatedAt.IsZero() {
		l.CreatedAt = time.Now().UTC()
	}
	m.logs = append(m.logs, l)
	return nil
}

func (m *mockNotifRepo) GetTemplateBySlug(_ context.Context, slug string) (*notifications.Template, error) {
	return nil, apperr.NotFound("template")
}

func (m *mockNotifRepo) ListUserNotifications(_ context.Context, userID int64, limit, offset int) ([]*notifications.NotificationLog, error) {
	var list []*notifications.NotificationLog
	for _, l := range m.logs {
		if l.UserID == userID {
			list = append(list, l)
		}
	}
	return list, nil
}

func (m *mockNotifRepo) MarkAsRead(_ context.Context, id int64, userID int64) error {
	for _, l := range m.logs {
		if l.ID == id && l.UserID == userID {
			l.IsRead = true
			return nil
		}
	}
	return apperr.NotFound("notification")
}

func (m *mockNotifRepo) GetUnreadCount(_ context.Context, userID int64) (int, error) {
	c := 0
	for _, l := range m.logs {
		if l.UserID == userID && !l.IsRead {
			c++
		}
	}
	return c, nil
}

func (m *mockNotifRepo) MarkAllAsRead(_ context.Context, userID int64) (int64, error) {
	var c int64
	for _, l := range m.logs {
		if l.UserID == userID && !l.IsRead {
			l.IsRead = true
			c++
		}
	}
	return c, nil
}

func (m *mockNotifRepo) ListUnread(_ context.Context, userID int64, limit, offset int) ([]*notifications.NotificationLog, error) {
	var list []*notifications.NotificationLog
	for _, l := range m.logs {
		if l.UserID == userID && !l.IsRead {
			list = append(list, l)
		}
	}
	return list, nil
}

func (m *mockNotifRepo) HasNotificationWithTitle(_ context.Context, userID int64, title string, since time.Time) (bool, error) {
	for _, l := range m.logs {
		if l.UserID == userID && l.Title == title && (since.IsZero() || !l.CreatedAt.Before(since)) {
			return true, nil
		}
	}
	return false, nil
}


func TestNotificationsDispatch_Comprehensive(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	nRepo := &mockNotifRepo{logs: make([]*notifications.NotificationLog, 0)}
	notifSvc := notifications.NewService(nRepo, logger)

	handler := &UIHandler{
		notifSvc: notifSvc,
		log:      logger,
	}

	// 1. Account Registered Welcome Notification
	orgID := int64(100)
	handler.notifyAccountRegistered(ctx, 501, &orgID)
	assert.NotEmpty(t, nRepo.logs)
	lastLog := nRepo.logs[len(nRepo.logs)-1]
	assert.Equal(t, int64(501), lastLog.UserID)
	assert.Contains(t, lastLog.Title, "دواء24")

	// 2. Org Approved / Rejected Notification
	handler.notifyOrgApproved(ctx, orgID)
	handler.notifyOrgRejected(ctx, orgID, "نقص السجل التجاري")

	// 3. Document Verified / Rejected Notification
	handler.notifyDocumentVerified(ctx, orgID, "السجل التجاري", true, "")
	handler.notifyDocumentVerified(ctx, orgID, "رخصة الصيدلية", false, "صورة غير واضحة")

	// 4. Wallet Deposit Approved / Rejected Notification
	amt, _ := money.Parse("5000.00")
	handler.notifyWalletDeposit(ctx, 501, orgID, amt, "approved")
	handler.notifyWalletDepositRejected(ctx, 501, orgID, amt, "عدم تطابق إيصال التحويل")

	// 5. Special Offer Approved / Rejected Notification
	handler.notifySpecialOfferStatus(ctx, orgID, "عرض خصم الصيف 20%", true, "")
	handler.notifySpecialOfferStatus(ctx, orgID, "عرض غير مطابق", false, "مخالفة الشروط")

	// 6. Sponsorship & Ad Status Notification
	handler.notifySponsorshipStatus(ctx, orgID, "الباقة الذهبية", true, "")
	handler.notifySponsorshipStatus(ctx, orgID, "الباقة الفضية", false, "انتهاء السعة")
	handler.notifyAdStatus(ctx, orgID, "بانر رئيسي", true, "")
	handler.notifyAdStatus(ctx, orgID, "بانر جانبي", false, "المقاسات غير متطابقة")

	// 7. Negotiation Offer & Decision Notification
	negAmt, _ := money.Parse("1200.00")
	handler.notifyNegotiationOffer(ctx, orgID, "صيدلية النور", "ORD-1234", negAmt)
	handler.notifyNegotiationDecision(ctx, 501, orgID, "شركة الأدوية المتحدة", "ORD-1234", true, "")
	handler.notifyNegotiationDecision(ctx, 501, orgID, "شركة الأدوية المتحدة", "ORD-1234", false, "السعر المقترح أقل من التكلفة")

	// 8. Quotes & RFQ Notification
	handler.notifyQuoteRequest(ctx, orgID, "صيدلية الشفاء", "بنادول إكسترا", 50)
	unitPrice, _ := money.Parse("45.50")
	handler.notifyQuoteProvided(ctx, 501, orgID, "شركة الأمل", "بنادول إكسترا", unitPrice)
	handler.notifyQuoteDecision(ctx, orgID, "صيدلية الشفاء", "بنادول إكسترا", true)
	handler.notifyQuoteDecision(ctx, orgID, "صيدلية الشفاء", "بنادول إكسترا", false)

	// Confirm that in-app notification logs were created without errors
	assert.Greater(t, len(nRepo.logs), 5)
}

type mockOrgRepoForNotifs struct {
	org.Repository
	members []*org.Member
}

func (m *mockOrgRepoForNotifs) ListMembersByOrg(_ context.Context, orgID int64) ([]*org.Member, error) {
	var res []*org.Member
	for _, mem := range m.members {
		if mem.OrganizationID == orgID {
			res = append(res, mem)
		}
	}
	return res, nil
}

func TestCourierNotificationExclusionAndIsolation(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	nRepo := &mockNotifRepo{logs: make([]*notifications.NotificationLog, 0)}
	notifSvc := notifications.NewService(nRepo, logger)

	orgID := int64(200)
	courierUser := int64(27)
	managerUser := int64(28)

	orgRepo := &mockOrgRepoForNotifs{
		members: []*org.Member{
			{
				ID:             1,
				OrganizationID: orgID,
				UserID:         courierUser,
				RoleKey:        "org_courier",
				IsActive:       true,
			},
			{
				ID:             2,
				OrganizationID: orgID,
				UserID:         managerUser,
				RoleKey:        "org_manager",
				IsActive:       true,
			},
		},
	}
	orgSvc := org.NewService(orgRepo, logger)

	handler := &UIHandler{
		notifSvc: notifSvc,
		orgSvc:   orgSvc,
		log:      logger,
	}

	// 1. Dispatch supply order notification to the organization (vendor.order.view)
	handler.dispatchOrgNotification(ctx, orgID, "vendor.order.view", "طلب توريد جديد #ORD-9999", "تم استلام طلب توريد جديد بقيمة 50,000 ج.م")

	// Courier must NOT have received any notification!
	for _, l := range nRepo.logs {
		assert.NotEqual(t, courierUser, l.UserID, "Courier must NEVER receive vendor supply order notifications!")
	}
	// Manager DID receive it
	var managerLogs []*notifications.NotificationLog
	for _, l := range nRepo.logs {
		if l.UserID == managerUser {
			managerLogs = append(managerLogs, l)
		}
	}
	assert.NotEmpty(t, managerLogs, "Manager must receive vendor order notification")
	assert.Equal(t, "vendor.order.view", managerLogs[0].RequiredPermission)

	// 2. Dispatch parcel delivery notification to the organization (vendor.delivery.view)
	nRepo.logs = nil
	handler.dispatchOrgNotification(ctx, orgID, "vendor.delivery.view", "طرد جديد بانتظار التوصيل", "تم تفويض طرد جديد للتوصيل")

	var courierLogs []*notifications.NotificationLog
	for _, l := range nRepo.logs {
		if l.UserID == courierUser {
			courierLogs = append(courierLogs, l)
		}
	}
	assert.NotEmpty(t, courierLogs, "Courier MUST receive parcel delivery notification")
	assert.Equal(t, "vendor.delivery.view", courierLogs[0].RequiredPermission)

	// 3. Test filterNotificationsForActor for Courier Actor
	courierActor := authctx.Actor{
		UserID:         courierUser,
		OrganizationID: orgID,
		OrgType:        "vendor",
		Role:           "org_courier",
		Permissions:    []string{"vendor.delivery.view"},
	}

	mixedLogs := []*notifications.NotificationLog{
		{
			ID:                 101,
			UserID:             courierUser,
			Title:              "طلب توريد جديد #2721",
			Body:               "طلب توريد لصيدلية الشفاء بقيمة 20,000 ج.م",
			RequiredPermission: "vendor.order.view",
		},
		{
			ID:                 102,
			UserID:             courierUser,
			Title:              "طلب شحن رصيد معلق",
			Body:               "طلب إيداع بمبلغ 15,000 ج.م في انتظار المراجعة",
			RequiredPermission: "vendor.wallet.view",
		},
		{
			ID:                 103,
			UserID:             courierUser,
			Title:              "طلب توريد قديم بدون تصريح مسجل",
			Body:               "طلب توريد قديم",
			RequiredPermission: "", // Unclassified old row
		},
		{
			ID:                 104,
			UserID:             courierUser,
			Title:              "طرد جديد بانتظار التوصيل",
			Body:               "تم تعيينك لتوصيل طرد صيدلية الدواء",
			RequiredPermission: "vendor.delivery.view",
		},
	}

	filtered := filterNotificationsForActor(courierActor, mixedLogs)
	assert.Len(t, filtered, 1, "Only the parcel delivery notification should pass the courier filter")
	assert.Equal(t, int64(104), filtered[0].ID)
	assert.Equal(t, "vendor.delivery.view", filtered[0].RequiredPermission)
}
