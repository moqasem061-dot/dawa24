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
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
	"github.com/muhiya/dawa24-store/internal/ui"
)

func TestSmartOrderResultsRoutes(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger)

	r := chi.NewRouter()
	handler.RegisterSmartOrderRoutes(r)

	// 1. Anonymous access redirects to login
	req, _ := http.NewRequest(http.MethodGet, "/customer/smart-order/new", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusSeeOther {
		t.Errorf("GET /customer/smart-order/new anonymous = %d; want %d", w.Code, http.StatusSeeOther)
	}

	// The wizard is gated on the Smart Ordering capability now that suppliers
	// reach the same pages; a member without it is redirected, not served.
	customerActor := authctx.Actor{
		UserID:         10,
		OrganizationID: 2,
		OrgType:        "customer",
		OrgStatus:      "approved",
		Scope:          rbac.ScopePharmacy,
		Role:           "customer",
	}
	customerActor.Grants([]string{
		rbac.BuySmartOrderView.Pharmacy,
		rbac.BuySmartOrderRun.Pharmacy,
	})
	customerCtx := authctx.WithActor(context.Background(), customerActor)

	// 2. Catalog search returns 200 with JSON
	req, _ = http.NewRequestWithContext(customerCtx, http.MethodGet, "/customer/smart-order/SO-TEST-001/catalog-search?q=panadol", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /customer/smart-order/SO-TEST-001/catalog-search = %d; want 200", w.Code)
	}

	// 3. Line match post (503 when service is unconfigured in test stub, or redirect when configured)
	form := url.Values{
		"product_id": {"150"},
	}
	req, _ = http.NewRequestWithContext(customerCtx, http.MethodPost, "/customer/smart-order/SO-TEST-001/lines/5/match?match=unmatched&limit=25", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusSeeOther && w.Code != http.StatusServiceUnavailable && w.Code != http.StatusNotFound {
		t.Errorf("POST /customer/smart-order/.../match = %d; want 503 or redirect", w.Code)
	}
}
