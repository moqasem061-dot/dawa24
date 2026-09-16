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

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui"
)

func TestAdminDeletionRequestsUnifiedRoutes(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger)

	r := chi.NewRouter()
	handler.RegisterAdminRoutes(r)

	adminActor := &authctx.Actor{
		UserID:      1,
		IsStaff:     true,
		Role:        "super_admin",
		Permissions: []string{"*"},
	}

	t.Run("Legacy path 301 redirects to unified page with tab=organizations", func(t *testing.T) {
		ctx := authctx.WithActor(context.Background(), *adminActor)
		req, _ := http.NewRequestWithContext(ctx, "GET", "/admin/organizations/deletion-requests?status=pending", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusMovedPermanently {
			t.Fatalf("expected 301, got %d", rr.Code)
		}
		loc := rr.Header().Get("Location")
		if !strings.Contains(loc, "/admin/deletion-requests") || !strings.Contains(loc, "tab=organizations") {
			t.Errorf("unexpected redirect Location: %q", loc)
		}
	})

	t.Run("Unified page default tab returns 200 for staff", func(t *testing.T) {
		ctx := authctx.WithActor(context.Background(), *adminActor)
		req, _ := http.NewRequestWithContext(ctx, "GET", "/admin/deletion-requests", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
	})

	t.Run("Unified page users tab returns 200 for staff", func(t *testing.T) {
		ctx := authctx.WithActor(context.Background(), *adminActor)
		req, _ := http.NewRequestWithContext(ctx, "GET", "/admin/deletion-requests?tab=users", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
	})

	t.Run("Approve org deletion without auth redirects to login", func(t *testing.T) {
		req, _ := http.NewRequest("POST", "/admin/deletion-requests/organizations/1/approve", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d", rr.Code)
		}
		if !strings.Contains(rr.Header().Get("Location"), "/auth/login") {
			t.Errorf("expected redirect to login, got %s", rr.Header().Get("Location"))
		}
	})

	t.Run("Approve user deletion without auth redirects to login", func(t *testing.T) {
		req, _ := http.NewRequest("POST", "/admin/deletion-requests/users/1/approve", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d", rr.Code)
		}
		if !strings.Contains(rr.Header().Get("Location"), "/auth/login") {
			t.Errorf("expected redirect to login, got %s", rr.Header().Get("Location"))
		}
	})
}
