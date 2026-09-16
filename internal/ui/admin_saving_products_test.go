package ui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

func TestAdminSavingProductSearchJSON_Unauthorized(t *testing.T) {
	h := newTestUIHandler()
	req := httptest.NewRequest(http.MethodGet, "/admin/saving-products/search-products?q=panadol", nil)
	rec := httptest.NewRecorder()

	h.AdminSavingProductSearchJSON(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for unauthenticated request, got %d", rec.Code)
	}
}

func TestAdminSavingProductSearchJSON_ShortQuery(t *testing.T) {
	h := newTestUIHandler()
	actor := authctx.Actor{
		UserID:      1,
		Role:        "super_admin",
		Permissions: []string{"catalog.saving_product.view"},
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/saving-products/search-products?q=a", nil)
	req = req.WithContext(authctx.WithActor(req.Context(), actor))
	rec := httptest.NewRecorder()

	h.AdminSavingProductSearchJSON(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	var res []any
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}
	if len(res) != 0 {
		t.Errorf("expected empty array for short query, got %d items", len(res))
	}
}

func TestAdminSavingProductLinkSubmit_Unauthorized(t *testing.T) {
	h := newTestUIHandler()
	req := httptest.NewRequest(http.MethodPost, "/admin/saving-products/10/link", nil)
	rec := httptest.NewRecorder()

	h.AdminSavingProductLinkSubmit(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 See Other redirect, got %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "/auth/login") {
		t.Errorf("expected redirect to login, got %q", loc)
	}
}

func TestAdminSavingProductLinkSubmit_InvalidID(t *testing.T) {
	h := newTestUIHandler()
	actor := authctx.Actor{
		UserID:      1,
		Role:        "super_admin",
		Permissions: []string{"catalog.saving_product.update"},
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/saving-products/abc/link", nil)
	req = req.WithContext(authctx.WithActor(req.Context(), actor))
	rec := httptest.NewRecorder()

	h.AdminSavingProductLinkSubmit(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 See Other, got %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "/admin/saving-products") || !strings.Contains(loc, "notice_type=error") {
		t.Errorf("expected error redirect to /admin/saving-products, got %q", loc)
	}
}

func TestRedirectAdminSavingProducts_ContextPreservation(t *testing.T) {
	h := newTestUIHandler()

	form := url.Values{
		"page":       {"2"},
		"q":          {"xyz"},
		"filter":     {"unlinked"},
		"org_id":     {"15"},
		"user_id":    {"30"},
		"product_id": {"101"},
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/saving-products/10/link", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	h.redirectAdminSavingProducts(rec, req, "success", "تم ربط الصنف بنجاح")

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 See Other, got %d", rec.Code)
	}

	loc := rec.Header().Get("Location")
	u, err := url.Parse(loc)
	if err != nil {
		t.Fatalf("invalid redirect location: %v", err)
	}

	if u.Path != "/admin/saving-products" {
		t.Errorf("expected path /admin/saving-products, got %q", u.Path)
	}

	q := u.Query()
	if q.Get("page") != "2" {
		t.Errorf("expected page=2 preserved, got %q", q.Get("page"))
	}
	if q.Get("q") != "xyz" {
		t.Errorf("expected q=xyz preserved, got %q", q.Get("q"))
	}
	if q.Get("filter") != "unlinked" {
		t.Errorf("expected filter=unlinked preserved, got %q", q.Get("filter"))
	}
	if q.Get("org_id") != "15" {
		t.Errorf("expected org_id=15 preserved, got %q", q.Get("org_id"))
	}
	if q.Get("user_id") != "30" {
		t.Errorf("expected user_id=30 preserved, got %q", q.Get("user_id"))
	}
	if q.Get("notice_type") != "success" {
		t.Errorf("expected notice_type=success, got %q", q.Get("notice_type"))
	}
}

func TestCalculateSavingMatchScore(t *testing.T) {
	p := &catalog.Product{
		ID:             10,
		SKU:            "PAN-EXTRA-24",
		Name:           i18n.Text{"ar": "بانادول إكسترا 24 قرص", "en": "Panadol Extra 24 Tablets"},
		ScientificName: "Paracetamol + Caffeine",
	}

	// 1. Exact match Arabic
	scoreExactAR := calculateSavingMatchScore("بانادول اكسترا 24 قرص", p)
	if scoreExactAR != 1.0 {
		t.Errorf("expected exact AR score 1.0, got %f", scoreExactAR)
	}

	// 2. Exact match SKU
	scoreExactSKU := calculateSavingMatchScore("pan extra 24", p)
	if scoreExactSKU < 0.90 {
		t.Errorf("expected high SKU score >= 0.90, got %f", scoreExactSKU)
	}

	// 3. Prefix match
	scorePrefix := calculateSavingMatchScore("بانادول", p)
	if scorePrefix < 0.85 {
		t.Errorf("expected prefix match score >= 0.85, got %f", scorePrefix)
	}

	// 4. Completely unrelated medicine
	scoreUnrelated := calculateSavingMatchScore("كونجستال شراب 120 مل", p)
	if scoreUnrelated > 0.40 {
		t.Errorf("expected low score for unrelated medicine <= 0.40, got %f", scoreUnrelated)
	}
}

func TestAdminSavingProductsPage_Smoke(t *testing.T) {
	h := newTestUIHandler()
	actor := authctx.Actor{
		UserID:      1,
		Role:        "super_admin",
		Permissions: []string{"catalog.saving_product.view"},
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/saving-products?page=2&q=panadol&filter=unlinked", nil)
	req = req.WithContext(authctx.WithActor(req.Context(), actor))
	rctx := chi.NewRouteContext()
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rec := httptest.NewRecorder()
	h.AdminSavingProductsPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK rendering AdminSavingProductsPage, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "admin-saving-link-modal") {
		t.Errorf("expected page HTML to contain admin-saving-link-modal component")
	}
}
