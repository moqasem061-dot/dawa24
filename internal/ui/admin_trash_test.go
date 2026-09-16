package ui_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui"
)

func TestAdminDeletesListsAndTrashRoutes(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger)

	r := chi.NewRouter()
	handler.RegisterAdminRoutes(r)

	tests := []struct {
		name       string
		path       string
		method     string
		actor      *authctx.Actor
		wantStatus int
	}{
		{
			name:       "Anonymous GET /admin/deletes-lists redirects to login",
			path:       "/admin/deletes-lists",
			method:     "GET",
			actor:      nil,
			wantStatus: http.StatusSeeOther,
		},
		{
			name:   "Super admin GET /admin/deletes-lists is a 301 to the trash list",
			path:   "/admin/deletes-lists",
			method: "GET",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusMovedPermanently,
		},
		{
			name:   "Super admin GET /admin/deletes-lists/{model} is a 301 to the trash list",
			path:   "/admin/deletes-lists/products",
			method: "GET",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusMovedPermanently,
		},
		{
			name:   "Super admin GET /admin/trash-list is reachable (needs a real service to render)",
			path:   "/admin/trash-list",
			method: "GET",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusSeeOther,
		},
		{
			name:   "Super admin GET /admin/trash-list/{model} is reachable (needs a real service to render)",
			path:   "/admin/trash-list/products",
			method: "GET",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusSeeOther,
		},
		{
			name:   "Super admin POST /admin/trash-list/products/12/restore redirects",
			path:   "/admin/trash-list/products/12/restore",
			method: "POST",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusSeeOther,
		},
		{
			name:   "Super admin POST /admin/trash-list/products/12/purge redirects",
			path:   "/admin/trash-list/products/12/purge",
			method: "POST",
			actor: &authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusSeeOther,
		},
	}

	// These cases construct the handler with nil services. That proves the route
	// exists and is permission-gated; it cannot prove the page renders data —
	// a page that returns 200 with no services is a page that reads nothing.
	// Rendering is covered by the tests that use a real database.
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.actor != nil {
				ctx = authctx.WithActor(ctx, *tt.actor)
			}

			req, _ := http.NewRequestWithContext(ctx, tt.method, tt.path, nil)
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("path %s expected status %d, got %d", tt.path, tt.wantStatus, rr.Code)
			}
		})
	}
}

type adminTrashRepoMock struct {
	platformadmin.Repository
	models   []*platformadmin.TrashModel
	rows     map[string][]*platformadmin.TrashRow
	restored []int64
	purged   []int64
}

func (m *adminTrashRepoMock) ListSoftDeletableTables(context.Context) ([]*platformadmin.TrashModel, error) {
	return m.models, nil
}

func (m *adminTrashRepoMock) ListTrashedRows(ctx context.Context, schema, table string, limit, offset int) ([]*platformadmin.TrashRow, error) {
	key := schema + "." + table
	return m.rows[key], nil
}

func (m *adminTrashRepoMock) ListTrashedRowsWithTotal(ctx context.Context, schema, table, search string, limit, offset int) ([]*platformadmin.TrashRow, int, error) {
	key := schema + "." + table
	all := m.rows[key]
	var filtered []*platformadmin.TrashRow
	for _, r := range all {
		if search == "" || strings.Contains(r.Label, search) || strings.Contains(r.Code, search) {
			filtered = append(filtered, r)
		}
	}
	return filtered, len(filtered), nil
}

func (m *adminTrashRepoMock) RestoreTrashedRow(_ context.Context, _, _ string, id, _ int64) error {
	m.restored = append(m.restored, id)
	return nil
}

func (m *adminTrashRepoMock) PurgeTrashedRow(_ context.Context, _, _ string, id, _ int64) error {
	m.purged = append(m.purged, id)
	return nil
}

func newTestTrashMock() *adminTrashRepoMock {
	orgID := int64(42)
	delID := int64(101)
	return &adminTrashRepoMock{
		models: []*platformadmin.TrashModel{
			{
				Key:         "catalog.products",
				Schema:      "catalog",
				Table:       "products",
				NameAr:      "المنتجات الأساسية",
				NameEn:      "Master Products",
				TotalCount:  100,
				TrashedRows: 5,
			},
			{
				Key:         "catalog.categories",
				Schema:      "catalog",
				Table:       "categories",
				NameAr:      "الأقسام",
				NameEn:      "Categories",
				TotalCount:  20,
				TrashedRows: 0,
			},
		},
		rows: map[string][]*platformadmin.TrashRow{
			"catalog.products": {
				{
					ID:               10,
					Label:            "بانادول اكسترا 500 ملجم",
					Code:             "MED-500",
					OrganizationID:   &orgID,
					OrganizationName: "مستودع الأمل الحديث",
					DeletedBy:        &delID,
					DeletedByName:    "أحمد المشرف",
					DeletedAt:        "2026-03-01 12:00",
					CanRestore:       true,
				},
				{
					ID:               11,
					Label:            "أوجمنتين 1 جم",
					Code:             "MED-900",
					OrganizationID:   &orgID,
					OrganizationName: "مستودع الشفاء",
					DeletedBy:        &delID,
					DeletedByName:    "سارة المشرفة",
					DeletedAt:        "2026-03-02 15:30",
					CanRestore:       false,
					CannotRestoreWhy: "المنشأة التابعة لها محذوفة حالياً — يجب استرجاع المنشأة أولاً",
				},
			},
		},
	}
}

func staffTrashActor() *authctx.Actor {
	return &authctx.Actor{
		UserID:      1,
		IsStaff:     true,
		Role:        "super_admin",
		Permissions: []string{"*"},
	}
}

func TestAdminTrashListPage_RenderingAndCounts(t *testing.T) {
	mockRepo := newTestTrashMock()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	adminSvc := platformadmin.NewService(mockRepo, logger)
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, adminSvc, nil, nil, nil, nil, nil, logger)

	r := chi.NewRouter()
	r.Get("/admin/trash-list", handler.AdminTrashListPage)

	req := httptest.NewRequest("GET", "/admin/trash-list", nil)
	req = req.WithContext(authctx.WithActor(req.Context(), *staffTrashActor()))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "المنتجات والأدوية بالكتالوج المعتمد") {
		t.Error("expected models list to render Arabic names")
	}
	if !strings.Contains(body, "tab-btn") {
		t.Error("expected filter tabs on trash overview")
	}
	if !strings.Contains(body, `x-model="search"`) {
		t.Error("expected search box on trash models overview")
	}
}

func TestAdminTrashListModelPage_HumanIdentityAndConfirmModal(t *testing.T) {
	mockRepo := newTestTrashMock()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	adminSvc := platformadmin.NewService(mockRepo, logger)
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, adminSvc, nil, nil, nil, nil, nil, logger)

	r := chi.NewRouter()
	r.Get("/admin/trash-list/{model}", handler.AdminTrashListModelPage)

	req := httptest.NewRequest("GET", "/admin/trash-list/catalog.products", nil)
	req = req.WithContext(authctx.WithActor(req.Context(), *staffTrashActor()))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	body := rec.Body.String()

	// 1. Human Identity line: Arabic name, SKU/code, owning org, deleted by, deleted at
	if !strings.Contains(body, "بانادول اكسترا 500 ملجم") {
		t.Error("expected Arabic name 'بانادول اكسترا 500 ملجم'")
	}
	if !strings.Contains(body, "MED-500") {
		t.Error("expected SKU/code 'MED-500'")
	}
	if !strings.Contains(body, "مستودع الأمل الحديث") {
		t.Error("expected owning organization 'مستودع الأمل الحديث'")
	}
	if !strings.Contains(body, "أحمد المشرف") {
		t.Error("expected deleted by name 'أحمد المشرف'")
	}
	if !strings.Contains(body, "2026-03-01 12:00") {
		t.Error("expected deletion timestamp '2026-03-01 12:00'")
	}

	// 2. ConfirmModal for purge with irreversibility warning
	if !strings.Contains(body, `id="purge-modal-10"`) {
		t.Error("expected ConfirmModal dialog with id purge-modal-10")
	}
	if !strings.Contains(body, `data-modal-open="purge-modal-10"`) {
		t.Error("expected purge button with data-modal-open='purge-modal-10'")
	}
	if !strings.Contains(body, "تأكيد الحذف النهائي والقطعي") {
		t.Error("expected confirm modal title 'تأكيد الحذف النهائي والقطعي'")
	}

	// 3. Dependency check: Row 2 has CanRestore = false and CannotRestoreWhy
	if !strings.Contains(body, "المنشأة التابعة لها محذوفة حالياً — يجب استرجاع المنشأة أولاً") {
		t.Error("expected missing dependency reason on row 11")
	}
	if !strings.Contains(body, "استرجاع (معطل)") {
		t.Error("expected disabled restore button on row 11")
	}

	// 4. Per-model search bar
	if !strings.Contains(body, `name="q"`) {
		t.Error("expected search input on model trash page")
	}
}

func TestAdminTrashRestoreAndPurgeSubmissions(t *testing.T) {
	mockRepo := newTestTrashMock()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	adminSvc := platformadmin.NewService(mockRepo, logger)
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, adminSvc, nil, nil, nil, nil, nil, logger)

	r := chi.NewRouter()
	r.Post("/admin/trash-list/{model}/{id}/restore", handler.AdminTrashRestoreSubmit)
	r.Post("/admin/trash-list/{model}/{id}/purge", handler.AdminTrashPurgeSubmit)

	// 1. Restore
	reqRestore := httptest.NewRequest("POST", "/admin/trash-list/catalog.products/10/restore", nil)
	reqRestore = reqRestore.WithContext(authctx.WithActor(reqRestore.Context(), *staffTrashActor()))
	recRestore := httptest.NewRecorder()
	r.ServeHTTP(recRestore, reqRestore)

	if recRestore.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", recRestore.Code)
	}
	if len(mockRepo.restored) != 1 || mockRepo.restored[0] != 10 {
		t.Errorf("expected row 10 to be restored, got %v", mockRepo.restored)
	}

	// 2. Purge
	reqPurge := httptest.NewRequest("POST", "/admin/trash-list/catalog.products/10/purge", nil)
	reqPurge = reqPurge.WithContext(authctx.WithActor(reqPurge.Context(), *staffTrashActor()))
	recPurge := httptest.NewRecorder()
	r.ServeHTTP(recPurge, reqPurge)

	if recPurge.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", recPurge.Code)
	}
	if len(mockRepo.purged) != 1 || mockRepo.purged[0] != 10 {
		t.Errorf("expected row 10 to be purged, got %v", mockRepo.purged)
	}
}
