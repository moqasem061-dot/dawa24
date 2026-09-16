package http_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	platformadminHttp "github.com/muhiya/dawa24-store/internal/modules/platform_admin/http"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

func (happyRepo) ListErrorLogs(ctx context.Context, filter platformadmin.ErrorLogFilter) ([]*platformadmin.ErrorLog, int, error) {
	return nil, 0, nil
}
func (happyRepo) GetErrorLogByID(ctx context.Context, id int64) (*platformadmin.ErrorLog, error) {
	return nil, nil
}
func (happyRepo) UpdateErrorLogStatus(ctx context.Context, id int64, status string) error {
	return nil
}
func (happyRepo) GetErrorDiagnosticsMetrics(ctx context.Context) (int, int, int, int, error) {
	return 0, 0, 0, 0, nil
}

func (happyRepo) ListContentBlocks(ctx context.Context) ([]*platformadmin.ContentBlock, error) {
	return []*platformadmin.ContentBlock{{ID: 1, Key: "about", Title: i18n.Text{"ar": "من نحن"}, Body: i18n.Text{"ar": "نص"}}}, nil
}
func (happyRepo) ListContentBlocksWithTotal(ctx context.Context, limit, offset int) ([]*platformadmin.ContentBlock, int, error) {
	return []*platformadmin.ContentBlock{{ID: 1, Key: "about", Title: i18n.Text{"ar": "من نحن"}, Body: i18n.Text{"ar": "نص"}}}, 1, nil
}

func (happyRepo) GetContentBlockByKey(ctx context.Context, key string) (*platformadmin.ContentBlock, error) {
	return &platformadmin.ContentBlock{ID: 1, Key: key, Title: i18n.Text{"ar": key}, Body: i18n.Text{"ar": "نص"}}, nil
}

func (happyRepo) UpsertContentBlock(ctx context.Context, b *platformadmin.ContentBlock) error {
	b.ID = 1
	return nil
}

func (happyRepo) ToggleContentBlockStatus(ctx context.Context, id int64) error {
	return nil
}

func (happyRepo) DeleteContentBlock(ctx context.Context, id int64) error {
	return nil
}

func (happyRepo) RecordVisitor(ctx context.Context, v *platformadmin.Visitor) error {
	v.ID = 1
	return nil
}

func (happyRepo) VisitorAnalytics(ctx context.Context, limit int) (*platformadmin.VisitorAnalytics, error) {
	return &platformadmin.VisitorAnalytics{Total: 10, Today: 2, ByDevice: map[string]int{"desktop": 8}, ByOS: map[string]int{"android": 6}, ByBrowser: map[string]int{"chrome": 9}}, nil
}
func (happyRepo) VisitorAnalyticsWithTotal(ctx context.Context, limit, offset int) (*platformadmin.VisitorAnalytics, error) {
	return &platformadmin.VisitorAnalytics{Total: 10, Today: 2, ByDevice: map[string]int{"desktop": 8}, ByOS: map[string]int{"android": 6}, ByBrowser: map[string]int{"chrome": 9}}, nil
}

func (happyRepo) ListAuditLog(ctx context.Context, limit, offset int) ([]*platformadmin.AuditEntry, error) {
	return []*platformadmin.AuditEntry{{ID: 1, Action: "org.registered", EntityType: "organization", EntityID: "x"}}, nil
}
func (happyRepo) ListAuditLogByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*platformadmin.AuditEntry, error) {
	return []*platformadmin.AuditEntry{{ID: 1, OrganizationID: &orgID, Action: "org.registered", EntityType: "organization", EntityID: "x"}}, nil
}
func (happyRepo) ListAuditLogByOrgWithTotal(ctx context.Context, orgID int64, limit, offset int) ([]*platformadmin.AuditEntry, int, error) {
	return []*platformadmin.AuditEntry{{ID: 1, OrganizationID: &orgID, Action: "org.registered", EntityType: "organization", EntityID: "x"}}, 1, nil
}
func (happyRepo) ListOrgStaffAuditLogWithTotal(ctx context.Context, orgID int64, limit, offset int) ([]*platformadmin.AuditEntry, int, error) {
	return []*platformadmin.AuditEntry{{ID: 1, OrganizationID: &orgID, Action: "org.registered", EntityType: "organization", EntityID: "x"}}, 1, nil
}
func (happyRepo) ListAuditLogWithFilter(ctx context.Context, filter platformadmin.AuditLogFilter) ([]*platformadmin.AuditEntry, int, error) {
	return []*platformadmin.AuditEntry{{ID: 1, Action: "org.registered", EntityType: "organization", EntityID: "x"}}, 1, nil
}

func (happyRepo) QueueStats(ctx context.Context) (map[string]int, error) {
	return map[string]int{"available": 3, "completed": 10, "retryable": 1}, nil
}

func (happyRepo) ListTranslations(ctx context.Context, filter platformadmin.TranslationFilter) ([]*platformadmin.Translation, int, error) {
	return nil, 0, nil
}
func (happyRepo) GetTranslationByKey(ctx context.Context, key string) (*platformadmin.Translation, error) {
	return nil, nil
}
func (happyRepo) UpsertTranslation(ctx context.Context, t *platformadmin.Translation) error {
	return nil
}
func (happyRepo) DeleteTranslation(ctx context.Context, key string) error {
	return nil
}
func (happyRepo) GetTranslationStats(ctx context.Context) (*platformadmin.TranslationStats, error) {
	return &platformadmin.TranslationStats{}, nil
}
func (happyRepo) LoadAllCustomTranslations(ctx context.Context) (map[string]i18n.Text, error) {
	return map[string]i18n.Text{}, nil
}

const testCookieName = "dawa24_session"

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()

	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	paSvc := platformadmin.NewService(stubRepo{t: t}, log)

	r := chi.NewRouter()
	r.Use(httpx.RequestID)
	r.Use(httpx.Recover(log))
	r.Use(httpx.Locale)

	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(testCookieName)
			if err != nil || cookie.Value == "" || cookie.Value == "forged-token-that-was-never-issued" {
				httpx.Error(w, r, log, apperr.Unauthorized())
				return
			}
			next.ServeHTTP(w, r)
		})
	})
	platformadminHttp.NewHandler(paSvc, log).RegisterRoutes(r)

	return r
}

func newAuthedRouter(repo platformadmin.Repository) http.Handler {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	paSvc := platformadmin.NewService(repo, log)

	r := chi.NewRouter()
	r.Use(httpx.RequestID)
	r.Use(httpx.Recover(log))
	r.Use(httpx.Locale)

	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actor := authctx.Actor{
				UserID:         1,
				OrganizationID: 1,
				// IsStaff, because the write gates on these two modules use
				// RequireAPIPermission — a platform-settings or homepage-
				// highlight write is a staff action and a tenant member holding
				// the key by accident must still be refused.
				IsStaff:     true,
				Role:        "admin",
				Permissions: []string{"admin", "platform.admin"},
			}
			ctx := authctx.WithActor(r.Context(), actor)
			ctx = database.WithTenant(ctx, 1)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	platformadminHttp.NewHandler(paSvc, log).RegisterRoutes(r)
	return r
}

var protectedRoutes = []struct{ method, path string }{
	{http.MethodGet, "/api/v1/platform/settings/public"},
	{http.MethodGet, "/api/v1/platform/settings/theme"},
	{http.MethodPut, "/api/v1/platform/settings/theme"},
	{http.MethodGet, "/api/v1/platform/countries"},
	{http.MethodGet, "/api/v1/platform/countries/1/cities"},
	{http.MethodGet, "/api/v1/platform/currencies"},
	{http.MethodGet, "/api/v1/platform/languages"},
	{http.MethodPost, "/api/v1/platform/contact"},
	{http.MethodGet, "/api/v1/platform/contact"},
	{http.MethodGet, "/api/v1/admin/platform/translations"},
	{http.MethodPut, "/api/v1/admin/platform/translations/key"},
	{http.MethodGet, "/api/v1/admin/platform/audit-log"},
}

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
	req := httptest.NewRequest(http.MethodGet, "/api/v1/platform/countries", nil)
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

func TestPlatformAdminHandler_HappyPaths(t *testing.T) {
	router := newAuthedRouter(happyRepo{})

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{"ListPublicSettings", http.MethodGet, "/api/v1/platform/settings/public", "", http.StatusOK},
		{"GetSetting", http.MethodGet, "/api/v1/platform/settings/theme", "", http.StatusOK},
		{"SetSetting", http.MethodPut, "/api/v1/platform/settings/theme", `{"value":{"mode":"dark"}}`, http.StatusOK},
		{"ListCountries", http.MethodGet, "/api/v1/platform/countries", "", http.StatusOK},
		{"ListCities", http.MethodGet, "/api/v1/platform/countries/1/cities", "", http.StatusOK},
		{"ListCurrencies", http.MethodGet, "/api/v1/platform/currencies", "", http.StatusOK},
		{"ListLanguages", http.MethodGet, "/api/v1/platform/languages", "", http.StatusOK},
		{"SubmitContact", http.MethodPost, "/api/v1/platform/contact", `{"name":"User","email":"u@example.com","message":"Hello"}`, http.StatusCreated},
		{"ListContactMessages", http.MethodGet, "/api/v1/platform/contact?limit=10&offset=0", "", http.StatusOK},
		{"AdminAuditLog", http.MethodGet, "/api/v1/admin/platform/audit-log", "", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bodyReader io.Reader
			if tt.body != "" {
				bodyReader = strings.NewReader(tt.body)
			}
			req := httptest.NewRequest(tt.method, tt.path, bodyReader)
			if tt.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("%s %s got status %d, want %d (body: %s)", tt.method, tt.path, rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

// Trash methods: the trash screens are exercised by their own tests; these
// stubs only satisfy the Repository interface.
func (s stubRepo) ListSoftDeletableTables(context.Context) ([]*platformadmin.TrashModel, error) {
	return nil, nil
}
func (s stubRepo) ListTrashedRows(context.Context, string, string, int, int) ([]*platformadmin.TrashRow, error) {
	return nil, nil
}
func (s stubRepo) ListTrashedRowsWithTotal(context.Context, string, string, string, int, int) ([]*platformadmin.TrashRow, int, error) {
	return nil, 0, nil
}
func (s stubRepo) RestoreTrashedRow(context.Context, string, string, int64, int64) error {
	return nil
}
func (s stubRepo) PurgeTrashedRow(context.Context, string, string, int64, int64) error {
	return nil
}

// Trash methods: the trash screens are exercised by their own tests; these
// stubs only satisfy the Repository interface.
func (h happyRepo) ListSoftDeletableTables(context.Context) ([]*platformadmin.TrashModel, error) {
	return nil, nil
}
func (h happyRepo) ListTrashedRows(context.Context, string, string, int, int) ([]*platformadmin.TrashRow, error) {
	return nil, nil
}
func (h happyRepo) ListTrashedRowsWithTotal(context.Context, string, string, string, int, int) ([]*platformadmin.TrashRow, int, error) {
	return nil, 0, nil
}
func (h happyRepo) RestoreTrashedRow(context.Context, string, string, int64, int64) error {
	return nil
}
func (h happyRepo) PurgeTrashedRow(context.Context, string, string, int64, int64) error {
	return nil
}
