package http

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/platform/features"
	"github.com/moqasem061-dot/dawa24/internal/platform/httpx"
)

type healthRepo struct {
	erp.Repository
	rows  map[string]erp.KeyAuthRow
	infos map[int64]erp.HealthInfo
}

func (r *healthRepo) FindKeyByPrefixAsSystem(_ context.Context, prefix string) (erp.KeyAuthRow, bool, error) {
	row, ok := r.rows[prefix]
	return row, ok, nil
}
func (r *healthRepo) OrgStatusType(context.Context, int64) (string, string, error) {
	return "approved", "vendor", nil
}
func (r *healthRepo) TouchLastUsed(context.Context, int64, int64, string) error { return nil }
func (r *healthRepo) HealthData(_ context.Context, p erp.Principal) (erp.HealthInfo, error) {
	info, ok := r.infos[p.OrganizationID]
	if !ok || p.KeyID != p.OrganizationID*10 {
		return erp.HealthInfo{}, erp.ErrNotFound
	}
	return info, nil
}
func TestHandlerHealth(t *testing.T) {
	pepper := []byte("health-test-pepper")
	full, prefix, hash, err := erp.GenerateKey("test", pepper)
	if err != nil {
		t.Fatal(err)
	}
	repo := &healthRepo{rows: map[string]erp.KeyAuthRow{prefix: {APIKey: erp.APIKey{ID: 20, OrganizationID: 2, IntegrationID: 3, KeyPrefix: prefix, Status: "active", Scopes: []string{erp.ScopeOrdersRead}, WarehouseID: 22, BranchID: 23, RateLimitPerMinute: 600, MaxBatchItems: 50000}, KeyHash: hash, IntegrationStatus: "active", Provider: "testprovider"}}, infos: map[int64]erp.HealthInfo{1: {OrganizationName: map[string]string{"ar": "ألف", "en": "A"}, WarehouseName: "A warehouse", IntegrationStatus: "active"}, 2: {OrganizationName: map[string]string{"ar": "باء", "en": "B"}, WarehouseName: "B warehouse", IntegrationStatus: "active"}}}
	var logs bytes.Buffer
	svc := erp.NewService(repo, erp.Ports{}, erp.Config{Pepper: pepper}, slog.New(slog.NewTextHandler(&logs, nil)))
	handler := NewHandler(svc, erp.Config{}, httpx.NewLimiter(nil, "test:"), slog.New(slog.NewTextHandler(&logs, nil))).Routes()
	req := httptest.NewRequest("GET", "/health?organization_id=1", nil)
	req.Header.Set("Authorization", "Bearer "+full)
	req.Header.Set("X-Organization-Id", "1")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	var env struct {
		Data struct {
			APIVersion, ServerTime string
			Organization           struct {
				ID   int64
				Name map[string]string
				Type string
			}
			Integration struct{ Provider, Status string }
			Key         struct {
				Prefix    string
				Scopes    []string
				Warehouse struct {
					ID   int64
					Name string
				}
				Branch             struct{ ID int64 }
				RateLimitPerMinute int
				ExpiresAt          *string
			}
		}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || env.Data.Organization.ID != 2 || !reflect.DeepEqual(env.Data.Organization.Name, map[string]string{"ar": "باء", "en": "B"}) || env.Data.Organization.Type != "vendor" || env.Data.Integration.Provider != "testprovider" || env.Data.Integration.Status != "active" || env.Data.Key.Prefix != prefix || !reflect.DeepEqual(env.Data.Key.Scopes, []string{erp.ScopeOrdersRead}) || env.Data.Key.Warehouse.ID != 22 || env.Data.Key.Warehouse.Name != "B warehouse" || env.Data.Key.Branch.ID != 23 || env.Data.Key.RateLimitPerMinute != 600 || env.Data.Key.ExpiresAt != nil || env.Data.APIVersion == "" || env.Data.ServerTime == "" {
		t.Fatalf("health %d: %s", w.Code, w.Body.String())
	}
	spec := httptest.NewRecorder()
	handler.ServeHTTP(spec, httptest.NewRequest("GET", "/openapi.yaml", nil))
	if spec.Code != 200 {
		t.Errorf("spec status=%d", spec.Code)
	}
	unknown := httptest.NewRecorder()
	badRoute := httptest.NewRequest("GET", "/unknown", nil)
	badRoute.Header.Set("Authorization", "Bearer "+full)
	handler.ServeHTTP(unknown, badRoute)
	if unknown.Code != 404 || !strings.Contains(unknown.Body.String(), "ERP_NOT_FOUND") {
		t.Errorf("unknown: %d %s", unknown.Code, unknown.Body.String())
	}
	// The secret is base64url and may contain '_', so it is taken from the
	// parser rather than a naive underscore split: the assertions below must
	// cover the whole secret, not the fragment before its first underscore.
	_, secret, ok := erp.ParseKey(full)
	if !ok {
		t.Fatal("generated key does not parse")
	}
	mutated := []byte(secret)
	if mutated[0] == 'A' {
		mutated[0] = 'B'
	} else {
		mutated[0] = 'A'
	}
	bad := full[:len(full)-len(secret)] + string(mutated)
	badReq := httptest.NewRequest("GET", "/health", nil)
	badReq.Header.Set("Authorization", "Bearer "+bad)
	badW := httptest.NewRecorder()
	handler.ServeHTTP(badW, badReq)
	if badW.Code != 401 || strings.Contains(badW.Body.String(), bad) || strings.Contains(badW.Body.String(), secret) || strings.Contains(logs.String(), bad) || strings.Contains(logs.String(), secret) {
		t.Error("bad key response or logs leaked credentials")
	}
}

func TestHandlerAnswersERPDisabledWhileSwitchedOff(t *testing.T) {
	handler := NewHandler(nil, erp.Config{}, httpx.NewLimiter(nil, "test:"), slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))).Routes()

	for _, tc := range []struct{ method, path string }{
		{"GET", "/health"},
		{"POST", "/products/sync"},
		{"GET", "/openapi.yaml"},
		{"GET", "/no-such-route"},
	} {
		restore := features.SetGlobal(features.NewStatic(map[string]bool{features.KeyERPEnabled: false}))
		// A syntactically valid-looking key must not matter: the gate runs
		// before authentication, so nothing about the key is ever checked.
		req := httptest.NewRequest(tc.method, tc.path, nil)
		req.Header.Set("Authorization", "Bearer dk_test_anything")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		restore()

		var env responseEnvelope
		if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
			t.Fatalf("%s %s: %v: %s", tc.method, tc.path, err, w.Body.String())
		}
		if w.Code != 503 || env.Status != "error" || env.Error.Code != "ERP_DISABLED" || env.Error.Message["ar"] == "" || env.Error.Message["en"] == "" {
			t.Errorf("%s %s: status=%d body=%s, want 503 ERP_DISABLED with bilingual message", tc.method, tc.path, w.Code, w.Body.String())
		}
	}

	// Switched back on, the same request reaches key authentication again.
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	var env responseEnvelope
	_ = json.Unmarshal(w.Body.Bytes(), &env)
	if w.Code != 401 || env.Error.Code != "ERP_AUTH_MISSING" {
		t.Errorf("switched on: status=%d body=%s, want 401 ERP_AUTH_MISSING", w.Code, w.Body.String())
	}
}
