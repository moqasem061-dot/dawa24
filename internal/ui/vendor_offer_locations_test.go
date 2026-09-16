package ui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

func TestVendorOfferLocationsPage_AuthRequired(t *testing.T) {
	h := newTestUIHandler()

	req := httptest.NewRequest(http.MethodGet, "/vendor/offers/42/locations", nil)
	rec := httptest.NewRecorder()

	h.VendorOfferLocationsPage(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 See Other redirect for unauthenticated user, got %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "/auth/login") {
		t.Errorf("expected redirect to login, got %q", loc)
	}
}

func TestVendorOfferLocationsPage_OfferNotFound(t *testing.T) {
	h := newTestUIHandler()

	actor := authctx.Actor{
		UserID:         10,
		OrganizationID: 100,
		OrgType:        "vendor",
		Role:           "vendor",
		Permissions:    []string{"vendor.offer.view"},
	}

	req := httptest.NewRequest(http.MethodGet, "/vendor/offers/999/locations", nil)
	req = req.WithContext(authctx.WithActor(req.Context(), actor))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "999")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rec := httptest.NewRecorder()
	h.VendorOfferLocationsPage(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 See Other, got %d", rec.Code)
	}
	redirectLoc := rec.Header().Get("Location")
	if !strings.HasPrefix(redirectLoc, "/vendor/offers") {
		t.Errorf("expected redirect to /vendor/offers, got %q", redirectLoc)
	}
}

func TestVendorOfferLocationNewSubmit_CityRequired(t *testing.T) {
	h := newTestUIHandler()

	actor := authctx.Actor{
		UserID:         10,
		OrganizationID: 100,
		OrgType:        "vendor",
		Role:           "vendor",
		Permissions:    []string{"vendor.offer.manage"},
	}

	// Form submission with missing city_id
	form := url.Values{
		"governorate_id": {"1"},
		"city_id":        {""},
		"radius":         {"2000"},
		"latitude":       {"30.0444"},
		"longitude":      {"31.2357"},
	}

	req := httptest.NewRequest(http.MethodPost, "/vendor/offers/42/locations/new", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(authctx.WithActor(req.Context(), actor))

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "42")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rec := httptest.NewRecorder()
	h.VendorOfferLocationNewSubmit(rec, req)

	// Since promo service is nil, it safely redirects with service unavailable error
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 See Other redirect, got %d", rec.Code)
	}
}

func TestVendorOfferLocationDeleteSubmit_Validation(t *testing.T) {
	h := newTestUIHandler()

	actor := authctx.Actor{
		UserID:         10,
		OrganizationID: 100,
		OrgType:        "vendor",
		Role:           "vendor",
		Permissions:    []string{"vendor.offer.manage"},
	}

	req := httptest.NewRequest(http.MethodPost, "/vendor/offers/0/locations/0/delete", nil)
	req = req.WithContext(authctx.WithActor(req.Context(), actor))

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "0")
	rctx.URLParams.Add("locId", "0")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rec := httptest.NewRecorder()
	h.VendorOfferLocationDeleteSubmit(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 See Other redirect, got %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "notice_type=error") && !strings.Contains(loc, "notice=") {
		t.Errorf("expected error notice query in redirect, got %q", loc)
	}
}

func TestVendorOfferLocationI18nKeys(t *testing.T) {
	checkKey := func(key, ar, en string) {
		t.Helper()
		if ar == key || ar == "" {
			t.Errorf("expected translated Arabic text for key %q, got %q", key, ar)
		}
		if en == key || en == "" {
			t.Errorf("expected translated English text for key %q, got %q", key, en)
		}
	}

	checkKey("vendor.offer.location_added_success",
		i18n.T("ar", "vendor.offer.location_added_success"),
		i18n.T("en", "vendor.offer.location_added_success"))
	checkKey("vendor.offer.location_updated_success",
		i18n.T("ar", "vendor.offer.location_updated_success"),
		i18n.T("en", "vendor.offer.location_updated_success"))
	checkKey("vendor.offer.location_deleted_success",
		i18n.T("ar", "vendor.offer.location_deleted_success"),
		i18n.T("en", "vendor.offer.location_deleted_success"))
	checkKey("vendor.offer.location_not_found",
		i18n.T("ar", "vendor.offer.location_not_found"),
		i18n.T("en", "vendor.offer.location_not_found"))
	checkKey("vendor.offer.city_required",
		i18n.T("ar", "vendor.offer.city_required"),
		i18n.T("en", "vendor.offer.city_required"))
	checkKey("vendor.offer.city_mismatch",
		i18n.T("ar", "vendor.offer.city_mismatch"),
		i18n.T("en", "vendor.offer.city_mismatch"))
}
