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

func TestCustomerPhase7Routes(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger)

	r := chi.NewRouter()
	handler.RegisterCustomerRoutes(r)
	// The catalogue, the cart and the buyer's own orders moved to the shared
	// buying surface when suppliers gained the purchasing section; a pharmacy
	// reaches them through the same registrar a supplier does.
	handler.RegisterBuyingRoutes(r)
	handler.RegisterPublicRoutes(r)

	// /customer/cpanel is a 301 to /customer/dashboard: it was a link hub to
	// three pages already in the sidebar (PLAN_V7 Task 3.2).
	tests := []struct {
		name       string
		path       string
		method     string
		actor      *authctx.Actor
		wantStatus int
	}{
		{
			name:   "Customer GET /customer/saving-products returns 200",
			path:   "/customer/saving-products",
			method: "GET",
			actor: &authctx.Actor{
				UserID:         1,
				OrganizationID: 5,
				OrgType:        "customer",
				Permissions:    []string{"pharmacy.*"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Misspelled /customer/saveing-products 301 redirects",
			path:   "/customer/saveing-products",
			method: "GET",
			actor: &authctx.Actor{
				UserID:         1,
				OrganizationID: 5,
				OrgType:        "customer",
				Permissions:    []string{"pharmacy.*"},
			},
			wantStatus: http.StatusMovedPermanently,
		},
		{
			name:   "Customer GET /orders/offers 301 redirects to /orders",
			path:   "/orders/offers",
			method: "GET",
			actor: &authctx.Actor{
				UserID:         1,
				OrganizationID: 5,
				OrgType:        "customer",
				Permissions:    []string{"pharmacy.*"},
			},
			wantStatus: http.StatusMovedPermanently,
		},
		{
			name:   "Customer GET /customer/add-order returns 200",
			path:   "/customer/add-order",
			method: "GET",
			actor: &authctx.Actor{
				UserID:         1,
				OrganizationID: 5,
				OrgType:        "customer",
				Permissions:    []string{"pharmacy.*"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Customer GET /customer/products/main/10 redirects to /catalog/10",
			path:   "/customer/products/main/10",
			method: "GET",
			actor: &authctx.Actor{
				UserID:         1,
				OrganizationID: 5,
				OrgType:        "customer",
				Permissions:    []string{"pharmacy.*"},
			},
			wantStatus: http.StatusMovedPermanently,
		},
		{
			name:       "Anonymous GET /tracking returns 200",
			path:       "/tracking",
			method:     "GET",
			actor:      nil,
			wantStatus: http.StatusOK,
		},
	}

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

func TestCustomerSavingProductsPageImportRoute(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger)

	r := chi.NewRouter()
	handler.RegisterCustomerRoutes(r)

	actor := authctx.Actor{
		UserID:         1,
		OrganizationID: 5,
		OrgType:        "customer",
		Permissions:    []string{"pharmacy.*"},
	}
	ctx := authctx.WithActor(context.Background(), actor)

	req, _ := http.NewRequestWithContext(ctx, "GET", "/customer/saving-products", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rr.Code)
	}

	body := rr.Body.String()

	// Verify old modal is not rendered
	if strings.Contains(body, "openPharmacyImportModal()") {
		t.Errorf("expected no openPharmacyImportModal() in HTML body")
	}
	if strings.Contains(body, "pharmacy-saving-import-modal") {
		t.Errorf("expected no pharmacy-saving-import-modal in HTML body")
	}

	// Verify link to dedicated import page
	if !strings.Contains(body, `href="/customer/saving-products/import"`) {
		t.Errorf("expected link href=\"/customer/saving-products/import\" in HTML body")
	}

	// Verify the dedicated import page returns 200 OK
	reqImport, _ := http.NewRequestWithContext(ctx, "GET", "/customer/saving-products/import", nil)
	rrImport := httptest.NewRecorder()
	r.ServeHTTP(rrImport, reqImport)

	if rrImport.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /customer/saving-products/import, got %d", rrImport.Code)
	}
}

func TestCustomerSavingBulkDeleteRoute(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger)

	r := chi.NewRouter()
	handler.RegisterCustomerRoutes(r)

	actor := authctx.Actor{
		UserID:         1,
		OrganizationID: 5,
		OrgType:        "customer",
		Permissions:    []string{"pharmacy.*"},
	}
	ctx := authctx.WithActor(context.Background(), actor)

	form := url.Values{}
	form.Add("selected_ids", "101")
	form.Add("selected_ids", "102")

	req, _ := http.NewRequestWithContext(ctx, "POST", "/customer/saving-products/bulk-delete", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	// Since catSvc is nil in mock, it will redirect back with error notice
	if rr.Code != http.StatusSeeOther && rr.Code != http.StatusFound {
		t.Fatalf("expected redirect (302/303), got %d", rr.Code)
	}
	loc := rr.Header().Get("Location")
	if !strings.HasPrefix(loc, "/customer/saving-products") {
		t.Errorf("expected redirect to /customer/saving-products, got %s", loc)
	}
}
