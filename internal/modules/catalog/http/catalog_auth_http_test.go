package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
)

func TestProtectedRoutesRejectAnonymousCallers(t *testing.T) {
	router := newTestRouter(t)
	for _, route := range protectedRoutes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			req := httptest.NewRequest(route.method, route.path, strings.NewReader("{}"))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("got %d, want 401 — this endpoint is reachable without a session", rec.Code)
			}
		})
	}
}

func TestProtectedRoutesRejectGarbageSessionToken(t *testing.T) {
	router := newTestRouter(t)
	for _, route := range protectedRoutes {
		req := httptest.NewRequest(route.method, route.path, strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "dawa24_session", Value: "forged-token-that-was-never-issued"})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s with a forged token got %d, want 401", route.method, route.path, rec.Code)
		}
	}
}

func TestUnauthorizedResponseUsesTheErrorEnvelope(t *testing.T) {
	router := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/catalog/products", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	var body httpx.ErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not the JSON error envelope: %v (body: %s)", err, rec.Body.String())
	}
	if body.Error.Code == "" {
		t.Error("error envelope has no code")
	}
	if body.Error.RequestID == "" {
		t.Error("error envelope has no request_id")
	}
}

func TestHandlerRejectsUnknownJSONFields(t *testing.T) {
	router := newTestRouter(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/catalog/products",
		strings.NewReader(`{"name":{"ar":"test"},"price":100,"unknown_field":"y"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "dawa24_session", Value: "valid-token"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("got %d, want 422 for an unknown JSON field", rec.Code)
	}
}

func TestHandlerRejectsMalformedBody(t *testing.T) {
	router := newTestRouter(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/catalog/products",
		strings.NewReader(`{"name": `))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "dawa24_session", Value: "valid-token"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("got %d, want 422 for a malformed body", rec.Code)
	}
}

func (r stubRepo) CountProductsByOrg(_ context.Context, _ int64, _ string) (int, error) {
	r.fail("CountProductsByOrg")
	return 0, nil
}

// Category -> brand relationship stubs (PLAN_V7 Phase 4). Behaviour is covered
// by the catalog repository tests; these only satisfy the interface.
func (s stubRepo) ListBrandsByCategory(context.Context, int64) ([]*catalog.Brand, error) {
	return nil, nil
}

func (s stubRepo) BrandInCategory(context.Context, int64, int64) (bool, error) {
	return true, nil
}

func (s stubRepo) SetBrandCategories(context.Context, int64, []int64) error {
	return nil
}
