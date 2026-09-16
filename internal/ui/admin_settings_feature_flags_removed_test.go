package ui_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui"
)

func TestAdminSettings_FeatureFlagsSectionRemoved(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger)

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			actor := authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			}
			ctx := authctx.WithActor(req.Context(), actor)
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})
	r.Get("/admin/settings", handler.AdminSettingsPage)

	t.Run("GET /admin/settings contains no Feature Flags section or keys", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/settings", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		body, _ := io.ReadAll(rec.Body)
		bodyStr := string(body)

		// Assert removed section texts are 100% gone
		assert.NotContains(t, bodyStr, "Feature Flags")
		assert.NotContains(t, bodyStr, "مفاتيح تشغيل الميزات والأنظمة")
		assert.NotContains(t, bodyStr, "التحكم الفوري في إظهار أو إخفاء أقسام المنصة")
		assert.NotContains(t, bodyStr, "jobs.enabled")
		assert.NotContains(t, bodyStr, "jobs.seeker_accounts")
		assert.NotContains(t, bodyStr, "reviews.enabled")
		assert.NotContains(t, bodyStr, "offers.enabled")
		assert.NotContains(t, bodyStr, "finder.enabled")
		assert.NotContains(t, bodyStr, "services.enabled")
		assert.NotContains(t, bodyStr, "compare.enabled")
		assert.NotContains(t, bodyStr, "/admin/settings/features/toggle")

		// Assert general & financial settings remain present and functional
		assert.Contains(t, bodyStr, "الإعدادات المالية والمنظومة")
		assert.Contains(t, bodyStr, "حفظ إعدادات المنصة")
	})
}
