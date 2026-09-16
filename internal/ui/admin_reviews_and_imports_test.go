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

func TestAdminReviewsAndImportsRoutes(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger)

	r := chi.NewRouter()
	handler.RegisterAdminRoutes(r)

	t.Run("Anonymous GET /admin/reviews redirects to login", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/reviews", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusSeeOther, rec.Code)
		assert.Contains(t, rec.Header().Get("Location"), "/auth/login")
	})

	t.Run("Super admin GET /admin/reviews renders reviews hub", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/reviews", nil)
		actor := authctx.Actor{
			UserID:      1,
			IsStaff:     true,
			Role:        "super_admin",
			Permissions: []string{"*"},
		}
		ctx := authctx.WithActor(req.Context(), actor)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req.WithContext(ctx))

		require.Equal(t, http.StatusOK, rec.Code)
		body, _ := io.ReadAll(rec.Body)
		bodyStr := string(body)
		assert.Contains(t, bodyStr, "تقييمات ومراجعات الموردين")
		assert.Contains(t, bodyStr, "كل التقييمات")
		assert.Contains(t, bodyStr, "لا توجد تقييمات مطابقة")
	})

	t.Run("Anonymous GET /admin/organizations/import redirects to login", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/organizations/import", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusSeeOther, rec.Code)
		assert.Contains(t, rec.Header().Get("Location"), "/auth/login")
	})

	t.Run("Super admin GET /admin/organizations/import renders hub with tabs", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/organizations/import", nil)
		actor := authctx.Actor{
			UserID:      1,
			IsStaff:     true,
			Role:        "super_admin",
			Permissions: []string{"*"},
		}
		ctx := authctx.WithActor(req.Context(), actor)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req.WithContext(ctx))

		require.Equal(t, http.StatusOK, rec.Code)
		body, _ := io.ReadAll(rec.Body)
		bodyStr := string(body)
		assert.Contains(t, bodyStr, "استيراد المنتجات لمنظمات")
		assert.Contains(t, bodyStr, "الصيدليات (العملاء)")
		assert.Contains(t, bodyStr, "الموردون والشركات")
	})

	t.Run("Super admin GET /admin/organizations/import?tab=vendor renders vendor tab", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/organizations/import?tab=vendor", nil)
		actor := authctx.Actor{
			UserID:      1,
			IsStaff:     true,
			Role:        "super_admin",
			Permissions: []string{"*"},
		}
		ctx := authctx.WithActor(req.Context(), actor)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req.WithContext(ctx))

		require.Equal(t, http.StatusOK, rec.Code)
		body, _ := io.ReadAll(rec.Body)
		bodyStr := string(body)
		assert.Contains(t, bodyStr, "الموردون والشركات")
	})
}
