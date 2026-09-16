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

	"github.com/muhiya/dawa24-store/internal/modules/ingest"
	ingestHttp "github.com/muhiya/dawa24-store/internal/modules/ingest/http"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

type stubRepo struct{ t *testing.T }

func (r stubRepo) fail(method string) {
	r.t.Helper()
	r.t.Fatalf("repository.%s was called; the request should have been rejected before reaching the repository", method)
}

func (r stubRepo) CreateFileUpload(context.Context, *ingest.FileUpload) error {
	r.fail("CreateFileUpload")
	return nil
}
func (r stubRepo) GetFileUploadByID(context.Context, int64) (*ingest.FileUpload, error) {
	r.fail("GetFileUploadByID")
	return nil, nil
}
func (r stubRepo) CreateImportSession(context.Context, *ingest.ImportSession) error {
	r.fail("CreateImportSession")
	return nil
}
func (r stubRepo) GetImportSessionByID(context.Context, int64) (*ingest.ImportSession, error) {
	r.fail("GetImportSessionByID")
	return nil, nil
}
func (r stubRepo) ListImportSessions(context.Context, int64, int, int) ([]*ingest.ImportSession, error) {
	r.fail("ListImportSessions")
	return nil, nil
}
func (r stubRepo) UpdateImportSessionProgress(context.Context, int64, int, int, ingest.SessionStatus, string) error {
	r.fail("UpdateImportSessionProgress")
	return nil
}
func (r stubRepo) UpdateColumnMapping(context.Context, int64, map[string]string) error {
	r.fail("UpdateColumnMapping")
	return nil
}
func (r stubRepo) UpdateSessionStatus(context.Context, int64, ingest.SessionStatus) error {
	r.fail("UpdateSessionStatus")
	return nil
}
func (r stubRepo) InsertImportRows(context.Context, []*ingest.ImportRow) error {
	r.fail("InsertImportRows")
	return nil
}
func (r stubRepo) ListImportRows(context.Context, int64, string, int, int) ([]*ingest.ImportRow, error) {
	r.fail("ListImportRows")
	return nil, nil
}
func (r stubRepo) GetImportRowByID(context.Context, int64) (*ingest.ImportRow, error) {
	r.fail("GetImportRowByID")
	return nil, nil
}
func (r stubRepo) UpdateImportRowMatch(context.Context, int64, *int64, float64, string) error {
	r.fail("UpdateImportRowMatch")
	return nil
}
func (r stubRepo) UpdateImportSessionConfig(context.Context, int64, *int64, ingest.ImportMode, bool, bool) error {
	r.fail("UpdateImportSessionConfig")
	return nil
}
func (r stubRepo) UpdateImportSessionStats(context.Context, int64, int, int, int, int, int, ingest.SessionStatus, string) error {
	r.fail("UpdateImportSessionStats")
	return nil
}
func (r stubRepo) UpdateImportRowMatchDetailed(context.Context, int64, *int64, float64, ingest.ConfidenceLevel, string, []ingest.CandidateMatch, bool, string) error {
	r.fail("UpdateImportRowMatchDetailed")
	return nil
}
func (r stubRepo) UpdateImportRowApproval(context.Context, int64, bool) error {
	r.fail("UpdateImportRowApproval")
	return nil
}
func (r stubRepo) UpdateImportRowAction(context.Context, int64, string, string) error {
	r.fail("UpdateImportRowAction")
	return nil
}
func (r stubRepo) BatchUpdateImportRowMatches(context.Context, []ingest.RowMatchUpdate) error {
	r.fail("BatchUpdateImportRowMatches")
	return nil
}
func (r stubRepo) BatchUpdateImportRowActions(context.Context, []ingest.RowActionUpdate) error {
	r.fail("BatchUpdateImportRowActions")
	return nil
}

type happyRepo struct{}

func (happyRepo) CreateFileUpload(ctx context.Context, f *ingest.FileUpload) error {
	f.ID = 1
	return nil
}
func (happyRepo) GetFileUploadByID(ctx context.Context, id int64) (*ingest.FileUpload, error) {
	return &ingest.FileUpload{ID: id, Filename: "catalog.xlsx", StorageKey: "uploads/1.xlsx"}, nil
}
func (happyRepo) CreateImportSession(ctx context.Context, s *ingest.ImportSession) error {
	s.ID = 1
	return nil
}
func (happyRepo) GetImportSessionByID(ctx context.Context, id int64) (*ingest.ImportSession, error) {
	return &ingest.ImportSession{ID: id, OrganizationID: 1, Status: ingest.StatusCompleted, TotalRows: 100}, nil
}
func (happyRepo) ListImportSessions(ctx context.Context, orgID int64, limit, offset int) ([]*ingest.ImportSession, error) {
	return []*ingest.ImportSession{{ID: 1, Status: ingest.StatusPending}}, nil
}
func (happyRepo) UpdateImportSessionProgress(ctx context.Context, id int64, processed, matched int, status ingest.SessionStatus, errMsg string) error {
	return nil
}
func (happyRepo) UpdateImportSessionConfig(ctx context.Context, id int64, warehouseID *int64, mode ingest.ImportMode, aiMatching, savingsMatching bool) error {
	return nil
}
func (happyRepo) UpdateImportSessionStats(ctx context.Context, id int64, total, processed, matched, review, unmatched int, status ingest.SessionStatus, errMsg string) error {
	return nil
}
func (happyRepo) UpdateColumnMapping(ctx context.Context, id int64, mapping map[string]string) error {
	return nil
}
func (happyRepo) UpdateSessionStatus(ctx context.Context, id int64, status ingest.SessionStatus) error {
	return nil
}
func (happyRepo) InsertImportRows(ctx context.Context, rows []*ingest.ImportRow) error {
	return nil
}
func (happyRepo) ListImportRows(ctx context.Context, sessionID int64, filter string, limit, offset int) ([]*ingest.ImportRow, error) {
	return []*ingest.ImportRow{{ID: 1, SessionID: sessionID, RowNumber: 1, NormalizedName: "Panadol"}}, nil
}
func (happyRepo) GetImportRowByID(ctx context.Context, id int64) (*ingest.ImportRow, error) {
	return &ingest.ImportRow{ID: id, SessionID: 1, RowNumber: 1, NormalizedName: "Panadol"}, nil
}
func (happyRepo) UpdateImportRowMatch(ctx context.Context, id int64, productID *int64, score float64, status string) error {
	return nil
}
func (happyRepo) UpdateImportRowMatchDetailed(ctx context.Context, id int64, productID *int64, score float64, confLevel ingest.ConfidenceLevel, reason string, candidates []ingest.CandidateMatch, isApproved bool, status string) error {
	return nil
}
func (happyRepo) UpdateImportRowApproval(ctx context.Context, id int64, isApproved bool) error {
	return nil
}
func (happyRepo) UpdateImportRowAction(ctx context.Context, id int64, action, errorDetails string) error {
	return nil
}
func (happyRepo) BatchUpdateImportRowMatches(ctx context.Context, updates []ingest.RowMatchUpdate) error {
	return nil
}
func (happyRepo) BatchUpdateImportRowActions(ctx context.Context, updates []ingest.RowActionUpdate) error {
	return nil
}

const testCookieName = "dawa24_session"

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()

	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	ingestSvc := ingest.NewService(stubRepo{t: t}, log)

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
	ingestHttp.NewHandler(ingestSvc, log).RegisterRoutes(r)

	return r
}

func newAuthedRouter(repo ingest.Repository) http.Handler {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	ingestSvc := ingest.NewService(repo, log)

	r := chi.NewRouter()
	r.Use(httpx.RequestID)
	r.Use(httpx.Recover(log))
	r.Use(httpx.Locale)

	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actor := authctx.Actor{
				UserID:         1,
				OrganizationID: 1,
				Role:           "admin",
				Permissions:    []string{"admin", "ingest.admin"},
			}
			ctx := authctx.WithActor(r.Context(), actor)
			ctx = database.WithTenant(ctx, 1)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	ingestHttp.NewHandler(ingestSvc, log).RegisterRoutes(r)
	return r
}

var protectedRoutes = []struct{ method, path string }{
	{http.MethodPost, "/api/v1/ingest/uploads"},
	{http.MethodPost, "/api/v1/ingest/sessions"},
	{http.MethodGet, "/api/v1/ingest/sessions"},
	{http.MethodGet, "/api/v1/ingest/sessions/1"},
	{http.MethodGet, "/api/v1/ingest/sessions/1/rows"},
	{http.MethodPost, "/api/v1/ingest/sessions/1/mapping"},
	{http.MethodPost, "/api/v1/ingest/sessions/1/commit"},
	{http.MethodPost, "/api/v1/ingest/sessions/1/cancel"},
	{http.MethodPut, "/api/v1/ingest/sessions/1/rows/1"},
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
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ingest/sessions", nil)
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

func TestIngestHandler_HappyPaths(t *testing.T) {
	router := newAuthedRouter(happyRepo{})

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{"RegisterUpload", http.MethodPost, "/api/v1/ingest/uploads", `{"filename":"catalog.xlsx","storage_key":"uploads/1.xlsx","file_size_bytes":1024,"mime_type":"application/vnd.ms-excel"}`, http.StatusCreated},
		{"StartSession", http.MethodPost, "/api/v1/ingest/sessions", `{"file_upload_id":1,"headers":["name","price","sku"],"min_score":0.7}`, http.StatusCreated},
		{"ListSessions", http.MethodGet, "/api/v1/ingest/sessions?limit=10&offset=0", "", http.StatusOK},
		{"GetSession", http.MethodGet, "/api/v1/ingest/sessions/1", "", http.StatusOK},
		{"ListRows", http.MethodGet, "/api/v1/ingest/sessions/1/rows?limit=10&offset=0", "", http.StatusOK},
		{"UpdateMapping", http.MethodPost, "/api/v1/ingest/sessions/1/mapping", `{"mapping":{"name":"product_name","price":"product_price"}}`, http.StatusOK},
		{"CommitSession", http.MethodPost, "/api/v1/ingest/sessions/1/commit", "", http.StatusOK},
		{"CancelSession", http.MethodPost, "/api/v1/ingest/sessions/1/cancel", "", http.StatusOK},
		{"OverrideRowMatch", http.MethodPut, "/api/v1/ingest/sessions/1/rows/1", `{"product_id":1}`, http.StatusOK},
		{"StreamEvents", http.MethodGet, "/api/v1/ingest/sessions/1/events", "", http.StatusOK},
		{"AdminSessions", http.MethodGet, "/api/v1/admin/ingest/sessions", "", http.StatusOK},
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

// foreignRepo stages sessions for organisation 2 and a row of session 7.
type foreignRepo struct{ happyRepo }

func (foreignRepo) GetImportSessionByID(ctx context.Context, id int64) (*ingest.ImportSession, error) {
	org := int64(2)
	if id == 1 {
		org = 1
	}
	return &ingest.ImportSession{ID: id, OrganizationID: org, Status: ingest.StatusPending}, nil
}

func (foreignRepo) GetImportRowByID(ctx context.Context, id int64) (*ingest.ImportRow, error) {
	return &ingest.ImportRow{ID: id, SessionID: 7}, nil
}

// A supplier may act only on its own organisation's import sessions, and on a
// row only through the session it belongs to.
func TestIngestSessionsAreOwnerScoped(t *testing.T) {
	router := newSupplierRouter(foreignRepo{})
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/v1/ingest/sessions/9", ""},
		{http.MethodGet, "/api/v1/ingest/sessions/9/rows", ""},
		{http.MethodGet, "/api/v1/ingest/sessions/9/events", ""},
		{http.MethodPost, "/api/v1/ingest/sessions/9/mapping", `{"mapping":{"a":"b"}}`},
		{http.MethodPost, "/api/v1/ingest/sessions/9/commit", ""},
		{http.MethodPost, "/api/v1/ingest/sessions/9/cancel", ""},
		{http.MethodPut, "/api/v1/ingest/sessions/9/rows/3", `{"product_id":1}`},
		// Own session, but the row belongs to session 7.
		{http.MethodPut, "/api/v1/ingest/sessions/1/rows/3", `{"product_id":1}`},
	} {
		var body io.Reader
		if tc.body != "" {
			body = strings.NewReader(tc.body)
		}
		req := httptest.NewRequest(tc.method, tc.path, body)
		if tc.body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s %s: status %d, want 404 (body %s)", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
}

func newSupplierRouter(repo ingest.Repository) http.Handler {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	r := chi.NewRouter()
	r.Use(httpx.RequestID)
	r.Use(httpx.Recover(log))
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actor := authctx.Actor{UserID: 5, OrganizationID: 1, Permissions: []string{"vendor.ingest.run", "vendor.ingest.view"}}
			ctx := database.WithTenant(authctx.WithActor(r.Context(), actor), 1)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	ingestHttp.NewHandler(ingest.NewService(repo, log), log).RegisterRoutes(r)
	return r
}
