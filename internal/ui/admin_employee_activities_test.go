package ui

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

type mockAuditActivitiesRepo struct {
	platformadmin.Repository
	lastFilter platformadmin.AuditLogFilter
	entries    []*platformadmin.AuditEntry
	totalCount int
}

func (m *mockAuditActivitiesRepo) ListAuditLogWithFilter(_ context.Context, filter platformadmin.AuditLogFilter) ([]*platformadmin.AuditEntry, int, error) {
	m.lastFilter = filter
	return m.entries, m.totalCount, nil
}

func (m *mockAuditActivitiesRepo) ListPublicSettings(_ context.Context) ([]*platformadmin.SystemSetting, error) {
	return nil, nil
}

func newTestAuditEntries() []*platformadmin.AuditEntry {
	orgID := int64(101)
	actorID := int64(10)
	return []*platformadmin.AuditEntry{
		{
			ID:               1,
			ActorUserID:      &actorID,
			ActorName:        "أحمد علي",
			ActorEmail:       "ahmed@dawa24.com",
			OrganizationID:   &orgID,
			OrganizationName: "مستودع الأمل",
			Action:           "catalog.product.create",
			ActionLabelAr:    "إضافة منتج للكتالوج",
			EntityType:       "product",
			EntityTypeAr:     "منتج",
			EntityID:         "501",
			IPAddress:        "192.168.1.50",
			CreatedAt:        time.Date(2026, 3, 5, 14, 30, 0, 0, time.UTC),
		},
		{
			ID:               2,
			ActorUserID:      &actorID,
			ActorName:        "أحمد علي",
			ActorEmail:       "ahmed@dawa24.com",
			OrganizationID:   nil,
			OrganizationName: "",
			Action:           "reference.plan.create",
			ActionLabelAr:    "إنشاء باقة اشتراك",
			EntityType:       "plan",
			EntityTypeAr:     "باقة اشتراك",
			EntityID:         "3",
			IPAddress:        "10.0.0.1",
			CreatedAt:        time.Date(2026, 3, 6, 9, 15, 0, 0, time.UTC),
		},
	}
}

func setupActivitiesTestHandler(repo *mockAuditActivitiesRepo) (*UIHandler, *chi.Mux) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	adminSvc := platformadmin.NewService(repo, logger)

	h := NewUIHandler(
		nil, nil, nil, nil, nil, nil, nil, nil, adminSvc, nil, nil, nil, nil, nil, logger,
	)

	r := chi.NewRouter()
	h.RegisterAdminRoutes(r)
	return h, r
}

func superAdminActor() authctx.Actor {
	return authctx.Actor{
		UserID:      1,
		IsStaff:     true,
		Role:        "super_admin",
		Permissions: []string{"*"},
	}
}

func TestAdminEmployeeActivities_AuthGuard(t *testing.T) {
	repo := &mockAuditActivitiesRepo{}
	_, r := setupActivitiesTestHandler(repo)

	t.Run("Anonymous GET /admin/employee-activities redirects to login", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/admin/employee-activities", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusSeeOther, rec.Code)
		assert.Contains(t, rec.Header().Get("Location"), "/auth/login")
	})

	t.Run("Anonymous GET /admin/employee-activities/export redirects to login", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/admin/employee-activities/export", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusSeeOther, rec.Code)
		assert.Contains(t, rec.Header().Get("Location"), "/auth/login")
	})

	t.Run("Super admin GET /admin/employee-activities returns 200", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/admin/employee-activities", nil)
		req = req.WithContext(authctx.WithActor(req.Context(), superAdminActor()))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	})
}

func TestAdminEmployeeActivities_FilterAndPagination(t *testing.T) {
	entries := newTestAuditEntries()
	repo := &mockAuditActivitiesRepo{
		entries:    entries,
		totalCount: 45,
	}
	_, r := setupActivitiesTestHandler(repo)

	req := httptest.NewRequest("GET", "/admin/employee-activities?page=2&limit=10&action=catalog.product.create&date_from=2026-03-01&date_to=2026-03-15&q=Paracetamol&org_id=101&user_id=10", nil)
	req = req.WithContext(authctx.WithActor(req.Context(), superAdminActor()))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	// Verify filter parameters passed down to repository
	assert.Equal(t, 10, repo.lastFilter.Limit)
	assert.Equal(t, 10, repo.lastFilter.Offset) // (page 2 - 1) * 10
	assert.Equal(t, "catalog.product.create", repo.lastFilter.Action)
	assert.Equal(t, "Paracetamol", repo.lastFilter.Search)
	require.NotNil(t, repo.lastFilter.OrganizationID)
	assert.Equal(t, int64(101), *repo.lastFilter.OrganizationID)
	require.NotNil(t, repo.lastFilter.ActorUserID)
	assert.Equal(t, int64(10), *repo.lastFilter.ActorUserID)
	require.NotNil(t, repo.lastFilter.DateFrom)
	assert.Equal(t, 2026, repo.lastFilter.DateFrom.Year())
	assert.Equal(t, time.March, repo.lastFilter.DateFrom.Month())
	assert.Equal(t, 1, repo.lastFilter.DateFrom.Day())
	require.NotNil(t, repo.lastFilter.DateTo)
	assert.Equal(t, 15, repo.lastFilter.DateTo.Day())

	// Verify rendered HTML
	body := rec.Body.String()
	assert.Contains(t, body, "catalog.product.create")
	assert.Contains(t, body, "إضافة منتج للكتالوج")
	assert.Contains(t, body, "192.168.1.50")
	assert.Contains(t, body, "أحمد علي")
	assert.Contains(t, body, "مستودع الأمل")
	// Verify date inputs preserve values
	assert.Contains(t, body, `value="2026-03-01"`)
	assert.Contains(t, body, `value="2026-03-15"`)
	// Verify export button preserves filter query parameters
	assert.Contains(t, body, "/admin/employee-activities/export?")
	assert.Contains(t, body, "action=catalog.product.create")
	assert.Contains(t, body, "date_from=2026-03-01")
	assert.Contains(t, body, "date_to=2026-03-15")
}

func TestAdminEmployeeActivities_ExportExcel(t *testing.T) {
	entries := newTestAuditEntries()
	repo := &mockAuditActivitiesRepo{
		entries:    entries,
		totalCount: len(entries),
	}
	_, r := setupActivitiesTestHandler(repo)

	req := httptest.NewRequest("GET", "/admin/employee-activities/export?date_from=2026-03-01&date_to=2026-03-15", nil)
	req = req.WithContext(authctx.WithActor(req.Context(), superAdminActor()))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", rec.Header().Get("Content-Type"))
	disp := rec.Header().Get("Content-Disposition")
	assert.True(t, strings.HasPrefix(disp, `attachment; filename="employee_activities_`), "Disposition header must have valid filename prefix")
	assert.True(t, strings.HasSuffix(disp, `.xlsx"`), "Disposition header must have .xlsx suffix")

	// Verify valid Excel file content
	f, err := excelize.OpenReader(bytes.NewReader(rec.Body.Bytes()))
	require.NoError(t, err, "Response body must be a valid openable Excel spreadsheet")
	defer f.Close()

	sheet := "سجل العمليات"
	sheets := f.GetSheetList()
	assert.Contains(t, sheets, sheet, "Excel file must contain 'سجل العمليات' worksheet")

	// Verify headers in row 1
	expectedHeaders := []string{
		"# المعرف (ID)",
		"تاريخ العملية",
		"توقيت العملية",
		"الموظف المنفذ",
		"البريد الإلكتروني",
		"المنشأة التابع لها",
		"كود المنشأة",
		"نوع الإجراء",
		"كود الإجراء",
		"القسم / العنصر",
		"معرف الصنف",
		"عنوان IP",
	}
	for colIdx, expectedHeader := range expectedHeaders {
		cell, err := excelize.CoordinatesToCellName(colIdx+1, 1)
		require.NoError(t, err)
		val, err := f.GetCellValue(sheet, cell)
		require.NoError(t, err)
		assert.Equal(t, expectedHeader, val, fmt.Sprintf("Header in column %d must match", colIdx+1))
	}

	// Verify row 2 data
	idCell, _ := f.GetCellValue(sheet, "A2")
	assert.Equal(t, "1", idCell)
	actorCell, _ := f.GetCellValue(sheet, "D2")
	assert.Equal(t, "أحمد علي", actorCell)
	orgCell, _ := f.GetCellValue(sheet, "F2")
	assert.Equal(t, "مستودع الأمل", orgCell)
	actionLabelCell, _ := f.GetCellValue(sheet, "H2")
	assert.Equal(t, "إضافة منتج للكتالوج", actionLabelCell)
	actionCodeCell, _ := f.GetCellValue(sheet, "I2")
	assert.Equal(t, "catalog.product.create", actionCodeCell)
	ipCell, _ := f.GetCellValue(sheet, "L2")
	assert.Equal(t, "192.168.1.50", ipCell)

	// Verify row 3 data (entry with nil OrgID)
	idCell3, _ := f.GetCellValue(sheet, "A3")
	assert.Equal(t, "2", idCell3)
	orgCell3, _ := f.GetCellValue(sheet, "F3")
	assert.Equal(t, "", orgCell3)
	actionLabelCell3, _ := f.GetCellValue(sheet, "H3")
	assert.Equal(t, "إنشاء باقة اشتراك", actionLabelCell3)
	actionCodeCell3, _ := f.GetCellValue(sheet, "I3")
	assert.Equal(t, "reference.plan.create", actionCodeCell3)
	ipCell3, _ := f.GetCellValue(sheet, "L3")
	assert.Equal(t, "10.0.0.1", ipCell3)
}

func TestAuditActionTranslations(t *testing.T) {
	requiredKeys := []struct {
		key      string
		expected string
	}{
		// Organizations
		{"audit.action.org.approve", "اعتماد منشأة"},
		{"audit.action.org.reject", "رفض منشأة"},
		{"audit.action.org.suspend", "إيقاف منشأة"},
		{"audit.action.org.reactivate", "إعادة تفعيل منشأة"},
		{"audit.action.org.registered", "تسجيل منشأة"},
		{"audit.action.org.change_request.approve", "اعتماد تعديل بيانات المنشأة"},
		{"audit.action.org.deletion.approve", "الموافقة على حذف المنشأة"},

		// Users
		{"audit.action.user.create_staff", "إضافة موظف إدارة"},
		{"audit.action.user.suspend", "إيقاف حساب مستخدم"},
		{"audit.action.user.reactivate", "إعادة تفعيل حساب مستخدم"},
		{"audit.action.user.reset_mfa", "إعادة ضبط المصادقة الثنائية (MFA)"},
		{"audit.action.user.password_set", "تعيين كلمة المرور"},
		{"audit.action.user.deletion.approve", "الموافقة على حذف حساب"},

		// Catalogue
		{"audit.action.catalog.product.create", "إضافة منتج للكتالوج"},
		{"audit.action.catalog.variant.create", "إضافة عرض توريد صنف"},
		{"audit.action.catalog.bulk.activate_all", "تفعيل شامل لكافة الأصناف"},

		// Commerce & Billing
		{"audit.action.commerce.order.status_change", "تحديث حالة الطلب"},
		{"audit.action.commerce.refund", "استرداد مالي لطلب"},
		{"audit.action.commerce.deposit.approve", "اعتماد إيداع محفظة"},
		{"audit.action.commerce.withdrawal.approve", "اعتماد سحب رصيد"},

		// Promo
		{"audit.action.promo.offer.approve", "اعتماد عرض ترويجي"},
		{"audit.action.promo.sponsorship.approve", "اعتماد طلب رعاية"},
		{"audit.action.promo.ad.approve", "اعتماد إعلان ترويجي"},

		// Reference
		{"audit.action.reference.city.create", "إضافة مدينة / مركز"},
		{"audit.action.reference.governorate.create", "إضافة محافظة"},
		{"audit.action.reference.institutional_work.create", "إضافة تصنيف عمل مؤسسي"},
		{"audit.action.reference.plan.create", "إنشاء باقة اشتراك"},
	}

	for _, tc := range requiredKeys {
		t.Run(tc.key, func(t *testing.T) {
			got, ok := i18n.Lookup(i18n.AR, tc.key)
			assert.True(t, ok, "key %s must exist in i18n", tc.key)
			assert.Equal(t, tc.expected, got)
		})
	}
}
