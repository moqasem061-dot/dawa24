package ui

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/modules/notifications"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

type mockIdentityRepoForTableTest struct {
	identity.Repository
	adminUser int64
}

func (m *mockIdentityRepoForTableTest) ListStaffUserIDs(_ context.Context) ([]int64, error) {
	return []int64{m.adminUser}, nil
}

func (m *mockIdentityRepoForTableTest) GetUserByID(_ context.Context, id int64) (*identity.User, error) {
	return &identity.User{
		ID:    id,
		Name:  i18n.Text{i18n.AR: "مستخدم تجريبي"},
		Email: "user@example.com",
	}, nil
}

type mockOrgRepoForTableTest struct {
	org.Repository
	ownerID int64
	orgName string
	staffID int64
}

func (m *mockOrgRepoForTableTest) GetOrganizationByID(_ context.Context, id int64) (*org.Organization, error) {
	return &org.Organization{
		ID:        id,
		TradeName: i18n.Text{i18n.AR: m.orgName},
		LegalName: m.orgName,
		OwnerID:   m.ownerID,
		Type:      org.TypeCustomer,
	}, nil
}

func (m *mockOrgRepoForTableTest) ListMembersByOrg(_ context.Context, orgID int64) ([]*org.Member, error) {
	return []*org.Member{
		{
			ID:             1,
			OrganizationID: orgID,
			UserID:         m.ownerID,
			RoleKey:        "owner",
			IsActive:       true,
		},
		{
			ID:             2,
			OrganizationID: orgID,
			UserID:         m.staffID,
			RoleKey:        "staff",
			IsActive:       true,
		},
	}, nil
}

type eventTestCase struct {
	name            string
	trigger         func(h *UIHandler, ctx context.Context, adminID, ownerID, staffID, testOrgID, supplierOrgID int64)
	expectedMinLogs int
	targetUserID    int64
	expectedPerm    string
	titleSnippet    string
}

func TestEveryEventDispatchesNotificationLog(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	nRepo := &mockNotifRepo{logs: make([]*notifications.NotificationLog, 0)}
	notifSvc := notifications.NewService(nRepo, logger)

	adminUserID := int64(10)
	orgOwnerID := int64(20)
	orgStaffID := int64(21)
	testOrgID := int64(100)
	supplierOrgID := int64(200)

	idRepo := &mockIdentityRepoForTableTest{adminUser: adminUserID}
	idSvc := identity.NewService(idRepo, nil, logger)

	orgRepo := &mockOrgRepoForTableTest{
		ownerID: orgOwnerID,
		orgName: "صيدلية النور المعتمدة",
		staffID: orgStaffID,
	}
	orgSvc := org.NewService(orgRepo, logger)

	handler := &UIHandler{
		notifSvc: notifSvc,
		idSvc:    idSvc,
		orgSvc:   orgSvc,
		log:      logger,
	}

	allCases := append(commerceTestCases, orgLifecycleTestCases...)
	for _, tc := range allCases {
		t.Run(tc.name, func(t *testing.T) {
			nRepo.logs = nil
			tc.trigger(handler, ctx, adminUserID, orgOwnerID, orgStaffID, testOrgID, supplierOrgID)
			require.GreaterOrEqual(t, len(nRepo.logs), tc.expectedMinLogs, "Event %s failed to generate notifications.logs", tc.name)

			logEntry := nRepo.logs[0]
			assert.NotEmpty(t, logEntry.Title, "Notification title must not be empty")
			assert.NotEmpty(t, logEntry.Body, "Notification body must not be empty")
			assert.Equal(t, notifications.ChannelInApp, logEntry.Channel, "Channel should be in_app")

			if tc.titleSnippet != "" {
				assert.Contains(t, logEntry.Title, tc.titleSnippet, "Notification title did not match expected Arabic template")
			}
			if tc.targetUserID > 0 {
				assert.Equal(t, tc.targetUserID, logEntry.UserID, "Notification user ID mismatch")
			}
			if tc.expectedPerm != "" {
				assert.Equal(t, tc.expectedPerm, logEntry.RequiredPermission, "RequiredPermission mismatch")
			}
		})
	}
}

var commerceTestCases = []eventTestCase{
	{
		name: "Quota exhausted -> buying org",
		trigger: func(h *UIHandler, ctx context.Context, _, _, _, testOrgID, _ int64) {
			h.notifyQuotaExhausted(ctx, testOrgID, "فرع المعادي", "شركة الأدوية", "كونكور 5 مجم")
		},
		expectedMinLogs: 1,
		expectedPerm:    "pharmacy.order.create",
		titleSnippet:    "استنفاد الحصة التوريدية",
	},
	{
		name: "Quota released -> buying org",
		trigger: func(h *UIHandler, ctx context.Context, _, _, _, testOrgID, _ int64) {
			h.notifyQuotaReleased(ctx, testOrgID, "فرع المعادي", "شركة الأدوية", "كونكور 5 مجم")
		},
		expectedMinLogs: 1,
		expectedPerm:    "pharmacy.order.create",
		titleSnippet:    "إتاحة وتجديد الحصة",
	},
	{
		name: "Order cancelled by buyer -> supplier org",
		trigger: func(h *UIHandler, ctx context.Context, _, _, _, _, supplierOrgID int64) {
			h.notifyOrderCancelledByBuyer(ctx, supplierOrgID, "ORD-888", "صيدلية النور", "تأخر التوريد")
		},
		expectedMinLogs: 1,
		expectedPerm:    "vendor.order.view",
		titleSnippet:    "إلغاء الطلب من قبل المشتري",
	},
	{
		name: "Smart order run finished -> user",
		trigger: func(h *UIHandler, ctx context.Context, _, ownerID, _, testOrgID, _ int64) {
			h.notifySmartOrderRunFinished(ctx, ownerID, testOrgID, 101)
		},
		expectedMinLogs: 1,
		targetUserID:    20,
		titleSnippet:    "الطلب الذكي",
	},
	{
		name: "Smart order run failed -> user",
		trigger: func(h *UIHandler, ctx context.Context, _, ownerID, _, testOrgID, _ int64) {
			h.notifySmartOrderRunFailed(ctx, ownerID, testOrgID, 102, "خطأ بالاتصال")
		},
		expectedMinLogs: 1,
		targetUserID:    20,
		titleSnippet:    "تعذر إكمال جولة الطلب الذكي",
	},
	{
		name: "Import run finished -> user",
		trigger: func(h *UIHandler, ctx context.Context, _, ownerID, _, testOrgID, _ int64) {
			h.notifyImportRunFinished(ctx, ownerID, testOrgID, 201, 150)
		},
		expectedMinLogs: 1,
		targetUserID:    20,
		titleSnippet:    "اكتمل استيراد المنتجات",
	},
	{
		name: "Import run failed -> user",
		trigger: func(h *UIHandler, ctx context.Context, _, ownerID, _, testOrgID, _ int64) {
			h.notifyImportRunFailed(ctx, ownerID, testOrgID, 202, "تنسيق الأعمدة خاطئ")
		},
		expectedMinLogs: 1,
		targetUserID:    20,
		titleSnippet:    "فشل استيراد المنتجات",
	},
	{
		name: "Order placed -> customer & vendor",
		trigger: func(h *UIHandler, ctx context.Context, _, _, staffID, testOrgID, supplierOrgID int64) {
			total, _ := money.Parse("1500.00")
			sub, _ := money.Parse("1500.00")
			prodID := int64(1)
			order := &commerce.Order{
				ID:             55,
				OrderNumber:    "ORD-0055",
				CustomerID:     staffID,
				OrganizationID: &testOrgID,
				TotalAmount:    total,
				Shipments: []*commerce.OrderShipment{
					{
						OrganizationID: supplierOrgID,
						Subtotal:       sub,
						Lines:          []*commerce.OrderLine{{ProductID: &prodID, Quantity: 2}},
					},
				},
			}
			h.notifyOrderPlaced(ctx, order, "صيدلية النور")
		},
		expectedMinLogs: 2,
		titleSnippet:    "استلام طلبك",
	},
	{
		name: "Order status changed -> customer",
		trigger: func(h *UIHandler, ctx context.Context, _, _, staffID, testOrgID, _ int64) {
			order := &commerce.Order{
				ID:             55,
				OrderNumber:    "ORD-0055",
				CustomerID:     staffID,
				OrganizationID: &testOrgID,
			}
			h.notifyOrderStatusChanged(ctx, order, 1, commerce.StatusProcessing, "مورد الأدوية", "")
		},
		expectedMinLogs: 1,
		titleSnippet:    "تحديث حالة الطلب",
	},
}
