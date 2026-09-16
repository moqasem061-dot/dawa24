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

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui"
)

func TestAdminTranslationsRoutes(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger)

	r := chi.NewRouter()
	handler.RegisterAdminRoutes(r)

	// 1. Anonymous access redirects to login
	req, _ := http.NewRequest(http.MethodGet, "/admin/translations", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusSeeOther {
		t.Errorf("GET /admin/translations anonymous = %d; want %d", w.Code, http.StatusSeeOther)
	}

	// 2. Super admin access returns 200
	adminCtx := authctx.WithActor(context.Background(), authctx.Actor{
		UserID:      1,
		IsStaff:     true,
		Role:        "super_admin",
		Permissions: []string{"*"},
	})
	req, _ = http.NewRequestWithContext(adminCtx, http.MethodGet, "/admin/translations", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /admin/translations super admin = %d; want 200", w.Code)
	}

	// 3. Super admin search query returns 200
	req, _ = http.NewRequestWithContext(adminCtx, http.MethodGet, "/admin/translations?q=save&ns=common", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /admin/translations?q=save super admin = %d; want 200", w.Code)
	}

	// 4. Update translation submit
	form := url.Values{
		"key":     {"common.save"},
		"text_ar": {"حفظ"},
		"text_en": {"Save"},
	}
	req, _ = http.NewRequestWithContext(adminCtx, http.MethodPost, "/admin/translations", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusSeeOther && w.Code != http.StatusOK {
		t.Errorf("POST /admin/translations super admin = %d; want redirect/200", w.Code)
	}

	// 5. Reset translation submit
	resetForm := url.Values{
		"key": {"common.save"},
	}
	req, _ = http.NewRequestWithContext(adminCtx, http.MethodPost, "/admin/translations/reset", strings.NewReader(resetForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusSeeOther && w.Code != http.StatusOK {
		t.Errorf("POST /admin/translations/reset super admin = %d; want redirect/200", w.Code)
	}

	// 6. Sync default translations submit
	req, _ = http.NewRequestWithContext(adminCtx, http.MethodPost, "/admin/translations/sync", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusSeeOther && w.Code != http.StatusOK {
		t.Errorf("POST /admin/translations/sync super admin = %d; want redirect/200", w.Code)
	}
}
