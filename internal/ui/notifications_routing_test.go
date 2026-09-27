package ui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"

	"github.com/moqasem061-dot/dawa24/internal/modules/notifications"
	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
	"github.com/moqasem061-dot/dawa24/internal/platform/rbac"
)

func TestResolveNotificationDestination(t *testing.T) {
	handler := &UIHandler{}
	ctx := context.Background()

	vendorActor := authctx.Actor{
		UserID:         10,
		OrganizationID: 100,
		Role:           "vendor",
		Scope:          rbac.ScopeVendor,
		OrgType:        "vendor",
		Permissions:    []string{"vendor.order.view", "vendor.wallet.view", "vendor.delivery.view", "vendor.document.view", "vendor.purchase_request.view"},
	}

	customerActor := authctx.Actor{
		UserID:         20,
		OrganizationID: 200,
		Role:           "customer",
		Scope:          rbac.ScopePharmacy,
		OrgType:        "pharmacy",
		Permissions:    []string{"pharmacy.order.view", "pharmacy.wallet.view"},
	}

	courierActor := authctx.Actor{
		UserID:         30,
		OrganizationID: 100,
		Role:           "courier",
		Scope:          rbac.ScopeVendor,
		OrgType:        "vendor",
		Permissions:    []string{"vendor.delivery.view"},
	}

	adminActor := authctx.Actor{
		UserID:      1,
		IsStaff:     true,
		Role:        "admin",
		Permissions: []string{"*"},
	}

	tests := []struct {
		name     string
		actor    authctx.Actor
		notif    *notifications.NotificationLog
		expected string
	}{
		{
			name:     "Vendor supply order notification",
			actor:    vendorActor,
			notif:    &notifications.NotificationLog{Title: "طلب توريد جديد #ORD-9999", Body: "تم استلام طلب توريد جديد"},
			expected: "/vendor/orders",
		},
		{
			name:     "Customer order confirmation notification",
			actor:    customerActor,
			notif:    &notifications.NotificationLog{Title: "تم استلام طلبك #ORD-5555", Body: "طلبك قيد التجهيز"},
			expected: "/orders",
		},
		{
			name:     "Courier delivery parcel notification",
			actor:    courierActor,
			notif:    &notifications.NotificationLog{Title: "طرد جديد بانتظار التوصيل", Body: "تم تعيينك لتوصيل طرد جديد"},
			expected: "/vendor/delivery",
		},
		{
			name:     "Customer shipment delivered notification with number",
			actor:    customerActor,
			notif:    &notifications.NotificationLog{Title: "تم تسليم شحنتك بنجاح (10542-1)", Body: "قام مندوب الشحن بتسليم طلبك #10542 من المورد الأهرام بنجاح."},
			expected: "/orders/10542",
		},
		{
			name:     "Vendor shipment delivered notification with number",
			actor:    vendorActor,
			notif:    &notifications.NotificationLog{Title: "تم تسليم الشحنة وتحصيل القيمة (10542-1)", Body: "تم تسليم الشحنة #10542-1 للصيدلية بنجاح وتأكيد كود الاستلام."},
			expected: "/vendor/orders/10542",
		},
		{
			name:     "Customer generic delivery notification must never route to vendor delivery",
			actor:    customerActor,
			notif:    &notifications.NotificationLog{Title: "طرد جديد قيد التوصيل", Body: "المندوب في الطريق للتسليم"},
			expected: "/orders",
		},
		{
			name:     "Vendor wallet deposit notification",
			actor:    vendorActor,
			notif:    &notifications.NotificationLog{Title: "تم شحن محفظتك", Body: "تم إيداع مبلغ 5000 ج.م في محفظتك"},
			expected: "/vendor/wallet",
		},
		{
			name:     "Customer wallet notification",
			actor:    customerActor,
			notif:    &notifications.NotificationLog{Title: "إيداع في المحفظة", Body: "تم إضافة رصيد لحسابك"},
			expected: "/customer/wallet",
		},
		{
			name:     "Platform admin approval notification",
			actor:    adminActor,
			notif:    &notifications.NotificationLog{Title: "تسجيل منشأة جديدة", Body: "منشأة صيدلية جديدة بانتظار الاعتماد"},
			expected: "/admin/approvals?tab=organizations",
		},
		{
			name:     "Job seeker notification for admin",
			actor:    adminActor,
			notif:    &notifications.NotificationLog{Title: "تسجيل باحث عن عمل جديد", Body: "تم تسجيل دكتور صيدلي جديد"},
			expected: "/admin/approvals?tab=job_seekers",
		},
		{
			name:     "Smart Order notification for pharmacy",
			actor:    customerActor,
			notif:    &notifications.NotificationLog{Title: "اكتمال تشغيل الطلب الذكي", Body: "تم تحليل النواقص ومطابقتها"},
			expected: "/customer/smart-order",
		},
		{
			name:     "Vendor ingest catalog notification",
			actor:    vendorActor,
			notif:    &notifications.NotificationLog{Title: "اكتمال استيراد ملف الأسعار", Body: "تم استيراد الأصناف في الكتالوج"},
			expected: "/vendor/ingest",
		},
		{
			name:     "Sponsorship notification for vendor",
			actor:    vendorActor,
			notif:    &notifications.NotificationLog{Title: "تحديث باقة رعاية", Body: "تمت الموافقة على باقة الرعاية"},
			expected: "/vendor/offers-packages/sponsorships",
		},
		{
			name:     "Admin special offer awaiting review notification",
			actor:    adminActor,
			notif:    &notifications.NotificationLog{Title: "عرض خاص جديد بانتظار المراجعة", Body: "أضاف المورد Dawa24 Seller العرض الخاص 'خصم 20% على أدوية ومستلزمات الموسم' وهو بانتظار المراجعة والاعتماد.", RequiredPermission: "promo.offer.view"},
			expected: "/admin/offers?status=pending",
		},
		{
			name:     "Admin general special offer notification",
			actor:    adminActor,
			notif:    &notifications.NotificationLog{Title: "عرض خاص", Body: "تم تحديث بيانات العرض الخاص", RequiredPermission: "promo.offer.view"},
			expected: "/admin/offers",
		},
		{
			name:     "Admin withdrawal request notification",
			actor:    adminActor,
			notif:    &notifications.NotificationLog{Title: "طلب سحب رصيد جديد بحاجة لموافقة", Body: "قدمت منشأة (Dawa24 Seller) طلب سحب بقيمة 10000 ج.م", RequiredPermission: "billing.payment.view"},
			expected: "/admin/finance/withdrawals",
		},
		{
			name:     "Admin deposit request notification",
			actor:    adminActor,
			notif:    &notifications.NotificationLog{Title: "طلب شحن رصيد جديد", Body: "قدمت صيدلية طلب شحن رصيد محفظة", RequiredPermission: "billing.payment.view"},
			expected: "/admin/finance/deposits",
		},
		{
			name:     "Document notification for admin",
			actor:    adminActor,
			notif:    &notifications.NotificationLog{Title: "مستند ترخيص جديد", Body: "تم رفع رخصة صيدلية جديدة"},
			expected: "/admin/approvals?tab=documents",
		},
		{
			name:     "Document notification for vendor",
			actor:    vendorActor,
			notif:    &notifications.NotificationLog{Title: "طلب مستند إضافي", Body: "يرجى رفع رخصة المخزن"},
			expected: "/vendor/documents",
		},
		{
			name:     "Account deletion notification for admin",
			actor:    adminActor,
			notif:    &notifications.NotificationLog{Title: "طلب حذف حساب مستخدم", Body: "المستخدم يطلب حذف حسابه"},
			expected: "/admin/deletion-requests?tab=users",
		},
		{
			name:     "Organization deletion notification for admin",
			actor:    adminActor,
			notif:    &notifications.NotificationLog{Title: "طلب حذف منشأة", Body: "المنشأة تطلب حذف حسابها"},
			expected: "/admin/deletion-requests?tab=organizations",
		},
		{
			name:     "Security password change notification",
			actor:    customerActor,
			notif:    &notifications.NotificationLog{Title: "تنبيه أمان: تم تغيير كلمة المرور", Body: "تم تحديث كلمة المرور لحسابك"},
			expected: "/settings?tab=security",
		},
		{
			name:     "Explicit URL embedded in notification body",
			actor:    vendorActor,
			notif:    &notifications.NotificationLog{Title: "إشعار مخصص", Body: "يمكنك متابعة التفاصيل عبر الرابط https://dawa24.com/vendor/branches"},
			expected: "/vendor/branches",
		},
		{
			name:     "Nil notification fallback",
			actor:    customerActor,
			notif:    nil,
			expected: "/customer/dashboard",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dest := handler.ResolveNotificationDestination(ctx, tt.actor, tt.notif)
			assert.Equal(t, tt.expected, dest)
		})
	}
}

func TestNotificationOpenRedirect(t *testing.T) {
	mockRepo := &mockNotifRepo{
		logs: []*notifications.NotificationLog{
			{
				ID:        42,
				UserID:    10,
				Title:     "طلب توريد جديد #ORD-1234",
				Body:      "تم استلام طلب توريد جديد",
				IsRead:    false,
				CreatedAt: time.Now().UTC(),
			},
		},
	}

	notifSvc := notifications.NewService(mockRepo, nil)
	handler := &UIHandler{notifSvc: notifSvc}

	vendorActor := authctx.Actor{
		UserID:         10,
		OrganizationID: 100,
		Role:           "vendor",
		Scope:          rbac.ScopeVendor,
		OrgType:        "vendor",
		Permissions:    []string{"vendor.order.view"},
	}

	r := chi.NewRouter()
	r.Get("/notifications/{id}/open", handler.NotificationOpenRedirect)

	// 1. Standard GET request with actor in context
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/notifications/42/open", nil)
	req = req.WithContext(authctx.WithActor(req.Context(), vendorActor))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/vendor/orders", rec.Header().Get("Location"))
	assert.Equal(t, "notificationRead", rec.Header().Get("HX-Trigger"))

	// Verify the notification was marked as read
	assert.True(t, mockRepo.logs[0].IsRead)

	// 2. HTMX GET request
	htmxReq := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/notifications/42/open", nil)
	htmxReq.Header.Set("HX-Request", "true")
	htmxReq = htmxReq.WithContext(authctx.WithActor(htmxReq.Context(), vendorActor))
	htmxRec := httptest.NewRecorder()

	r.ServeHTTP(htmxRec, htmxReq)

	assert.Equal(t, http.StatusOK, htmxRec.Code)
	assert.Equal(t, "/vendor/orders", htmxRec.Header().Get("HX-Redirect"))
}
