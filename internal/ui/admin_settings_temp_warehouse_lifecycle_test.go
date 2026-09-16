package ui_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/ui"
)

type mockLifecycleAdminRepo struct {
	platformadmin.Repository
	settings map[string]*platformadmin.SystemSetting
}

func newMockLifecycleAdminRepo() *mockLifecycleAdminRepo {
	return &mockLifecycleAdminRepo{
		settings: make(map[string]*platformadmin.SystemSetting),
	}
}

func (m *mockLifecycleAdminRepo) GetSetting(_ context.Context, key string) (*platformadmin.SystemSetting, error) {
	s, ok := m.settings[key]
	if !ok {
		return nil, apperr.NotFound("setting")
	}
	return s, nil
}

func (m *mockLifecycleAdminRepo) SetSetting(_ context.Context, s *platformadmin.SystemSetting) error {
	m.settings[s.Key] = s
	return nil
}

func (m *mockLifecycleAdminRepo) ListPolicyVersions(_ context.Context, _ string) ([]*platformadmin.Policy, error) {
	return nil, nil
}

func (m *mockLifecycleAdminRepo) GetActivePolicy(_ context.Context, _ string) (*platformadmin.Policy, error) {
	return nil, nil
}

func TestAdminSettings_TempWarehouseLifecycle(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	adminRepo := newMockLifecycleAdminRepo()
	adminSvc := platformadmin.NewService(adminRepo, logger)

	compareRepo := newMockBulkCompareRepo()
	compareSvc := compare.NewService(compareRepo, logger)

	// Seed one file in mock compare repo
	readyFile := &compare.CompareFile{
		OriginalFilename: "warehouse_test.xlsx",
		StorageKey:       "temp_warehouses/test.xlsx",
		Status:           compare.FileReady,
		IsTempWarehouse:  true,
		CreatedAt:        time.Now().Add(-1000 * time.Hour), // Older than 720 hours
		UpdatedAt:        time.Now().Add(-1000 * time.Hour),
	}
	_ = compareRepo.CreateFile(context.Background(), readyFile)

	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, adminSvc, nil, nil, nil, nil, nil, logger)
	handler.SetCompareService(compareSvc)

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			actor := authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"platform.setting.view", "platform.setting.update"},
			}
			ctx := authctx.WithActor(req.Context(), actor)
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})

	r.Get("/admin/settings", handler.AdminSettingsPage)
	r.Post("/admin/settings/temp-warehouses/lifecycle", handler.AdminTempWarehouseLifecycleSubmit)
	r.Post("/admin/settings/temp-warehouses/run-lifecycle", handler.AdminTempWarehouseRunLifecycle)

	t.Run("GET /admin/settings renders temp warehouse lifecycle controls", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/settings?tab=features", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		body, _ := io.ReadAll(rec.Body)
		bodyStr := string(body)

		assert.Contains(t, bodyStr, "دورة حياة وأرشفة المستودعات المؤقتة وملفات المقارنة")
		assert.Contains(t, bodyStr, "الأرشفة التلقائية للمستودعات وملفات المقارنة")
		assert.Contains(t, bodyStr, "auto_archive_hours")
		assert.Contains(t, bodyStr, "الحذف والتفريغ النهائي لبيانات المستودع")
		assert.Contains(t, bodyStr, "auto_delete_days")
		assert.Contains(t, bodyStr, "/admin/settings/temp-warehouses/lifecycle")
		assert.Contains(t, bodyStr, "/admin/settings/temp-warehouses/run-lifecycle")
		assert.Contains(t, bodyStr, "تشغيل الفحص والتنظيف الآن")
	})

	t.Run("POST /admin/settings/temp-warehouses/lifecycle saves settings", func(t *testing.T) {
		form := url.Values{}
		form.Set("auto_archive_hours", "48")
		form.Set("auto_archive_enabled", "true")
		form.Set("auto_delete_days", "15")
		form.Set("auto_delete_enabled", "true")

		req := httptest.NewRequest(http.MethodPost, "/admin/settings/temp-warehouses/lifecycle", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		require.Equal(t, http.StatusSeeOther, rec.Code)
		loc := rec.Header().Get("Location")
		assert.Contains(t, loc, "/admin/settings")
		assert.Contains(t, loc, "tab=features")
		assert.Contains(t, loc, "notice=success")

		settings, err := adminSvc.GetTempWarehouseLifecycleSettings(context.Background())
		require.NoError(t, err)
		require.NotNil(t, settings)
		assert.Equal(t, 48, settings.AutoArchiveHours)
		assert.True(t, settings.AutoArchiveEnabled)
		assert.Equal(t, 15, settings.AutoDeleteDays)
		assert.True(t, settings.AutoDeleteEnabled)
	})

	t.Run("POST /admin/settings/temp-warehouses/run-lifecycle triggers execution", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/admin/settings/temp-warehouses/run-lifecycle", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		require.Equal(t, http.StatusSeeOther, rec.Code)
		loc := rec.Header().Get("Location")
		assert.Contains(t, loc, "/admin/settings")
		assert.Contains(t, loc, "tab=features")
		assert.Contains(t, loc, "notice=success")

		// The seeded readyFile was created 1000h ago, with 48h limit it must now be archived
		updatedFile, err := compareRepo.GetFileByID(context.Background(), readyFile.ID)
		require.NoError(t, err)
		assert.Equal(t, compare.FileArchived, updatedFile.Status)
	})
}
