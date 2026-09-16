package ui_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

func TestAdminProductSampleDownloads(t *testing.T) {
	actor := authctx.Actor{
		UserID:      1,
		Role:        "super_admin",
		IsStaff:     true,
		OrgStatus:   "approved",
		Permissions: []string{"*"},
	}
	router := newTestRouter(&actor)

	t.Run("GET /admin/products/sample.csv", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/admin/products/sample.csv", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("want 200, got %d", rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "text/csv; charset=utf-8" {
			t.Fatalf("want text/csv, got %q", ct)
		}
		if len(rec.Body.Bytes()) < 100 {
			t.Fatalf("CSV content too small: %d bytes", len(rec.Body.Bytes()))
		}
	})

	t.Run("GET /admin/products/sample.xlsx", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/admin/products/sample.xlsx", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("want 200, got %d", rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" {
			t.Fatalf("want spreadsheetml.sheet, got %q", ct)
		}
		if len(rec.Body.Bytes()) < 100 {
			t.Fatalf("XLSX content too small: %d bytes", len(rec.Body.Bytes()))
		}
	})
}

func TestVendorIngestAndRolesRoutes(t *testing.T) {
	actor := &authctx.Actor{
		UserID:         1,
		OrganizationID: 1,
		OrgType:        "vendor",
		OrgStatus:      "approved",
		Role:           "vendor",
		IsOwner:        true,
		Permissions:    []string{"*"},
	}
	router := newTestRouter(actor)

	t.Run("GET /vendor/roles", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/vendor/roles", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("want 200, got %d", rec.Code)
		}
	})

	t.Run("GET /vendor/ingest/sample.csv", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/vendor/ingest/sample.csv", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("want 200, got %d", rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "text/csv; charset=utf-8" {
			t.Fatalf("want text/csv, got %q", ct)
		}
		if len(rec.Body.Bytes()) < 50 {
			t.Fatalf("CSV template too small")
		}
	})

	t.Run("GET /vendor/ingest/sample.xlsx", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/vendor/ingest/sample.xlsx", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("want 200, got %d", rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" {
			t.Fatalf("want excel content type, got %q", ct)
		}
		if len(rec.Body.Bytes()) < 100 {
			t.Fatalf("Excel template too small")
		}
	})

	t.Run("GET /vendor/ingest/inventory.csv", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/vendor/ingest/inventory.csv", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("want 200, got %d", rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "text/csv; charset=utf-8" {
			t.Fatalf("want text/csv, got %q", ct)
		}
	})
}
