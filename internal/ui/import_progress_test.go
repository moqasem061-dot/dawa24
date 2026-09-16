package ui_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/importrun"
	"github.com/muhiya/dawa24-store/internal/ui"
)

// mockImportRunRepo implements importrun.Repository for testing.
type mockImportRunRepo struct {
	runs map[string]*importrun.Run
	rows map[int64][]importrun.Row
}

func newMockImportRunRepo() *mockImportRunRepo {
	return &mockImportRunRepo{
		runs: make(map[string]*importrun.Run),
		rows: make(map[int64][]importrun.Row),
	}
}

func (m *mockImportRunRepo) CreateRun(ctx context.Context, run *importrun.Run) error {
	run.ID = int64(len(m.runs) + 1)
	if run.PublicID == "" {
		run.PublicID = "mock-uuid-1234"
	}
	m.runs[run.PublicID] = run
	return nil
}

func (m *mockImportRunRepo) GetRunByPublicID(ctx context.Context, publicID string, orgID int64) (*importrun.Run, error) {
	if r, ok := m.runs[publicID]; ok && r.OrganizationID == orgID {
		return r, nil
	}
	return nil, nil
}

func (m *mockImportRunRepo) GetRunByPublicIDSystem(ctx context.Context, publicID string) (*importrun.Run, error) {
	if r, ok := m.runs[publicID]; ok {
		return r, nil
	}
	return nil, nil
}

func (m *mockImportRunRepo) GetRunByID(ctx context.Context, id int64) (*importrun.Run, error) {
	for _, r := range m.runs {
		if r.ID == id {
			return r, nil
		}
	}
	return nil, nil
}

func (m *mockImportRunRepo) UpdateProgress(ctx context.Context, id int64, phase string, percent int, processed int) error {
	for _, r := range m.runs {
		if r.ID == id {
			r.Phase = phase
			r.Percent = percent
			r.ProcessedRows = processed
			return nil
		}
	}
	return nil
}

func (m *mockImportRunRepo) TransitionState(ctx context.Context, id int64, newState string) error {
	for _, r := range m.runs {
		if r.ID == id {
			r.State = newState
			return nil
		}
	}
	return nil
}

func (m *mockImportRunRepo) FailRun(ctx context.Context, id int64, errMsg string) error {
	for _, r := range m.runs {
		if r.ID == id {
			r.State = importrun.StateFailed
			r.ErrorMessage = errMsg
			return nil
		}
	}
	return nil
}

func (m *mockImportRunRepo) SetResult(ctx context.Context, id int64, result json.RawMessage) error {
	for _, r := range m.runs {
		if r.ID == id {
			r.Result = result
			return nil
		}
	}
	return nil
}

func (m *mockImportRunRepo) SetRiverJobID(ctx context.Context, id int64, jobID int64) error {
	return nil
}

func (m *mockImportRunRepo) ListRunsByOrg(ctx context.Context, orgID int64, kind string, limit, offset int) ([]*importrun.Run, int, error) {
	var list []*importrun.Run
	for _, r := range m.runs {
		if r.OrganizationID == orgID {
			list = append(list, r)
		}
	}
	return list, len(list), nil
}

func (m *mockImportRunRepo) InsertRows(ctx context.Context, runID int64, rows []importrun.Row) error {
	m.rows[runID] = append(m.rows[runID], rows...)
	return nil
}

func (m *mockImportRunRepo) ListRows(ctx context.Context, runID int64, onlyIncluded bool, limit, offset int) ([]importrun.Row, int, error) {
	rows := m.rows[runID]
	var res []importrun.Row
	for _, r := range rows {
		if !onlyIncluded || r.Included {
			res = append(res, r)
		}
	}
	return res, len(res), nil
}

func (m *mockImportRunRepo) UpdateRow(ctx context.Context, rowID int64, data json.RawMessage, included *bool, matchedProductID *int64) error {
	return nil
}

func (m *mockImportRunRepo) RecoverStaleRuns(ctx context.Context) (int, error) {
	return 0, nil
}

func (m *mockImportRunRepo) UpdatePayload(ctx context.Context, id int64, payload json.RawMessage) error {
	for _, r := range m.runs {
		if r.ID == id {
			r.Payload = payload
			return nil
		}
	}
	return nil
}

func (m *mockImportRunRepo) GetActiveRunForUser(ctx context.Context, userID int64, kind string) (*importrun.Run, error) {
	for _, r := range m.runs {
		if r.UserID == userID && r.Kind == kind && !r.IsDone() {
			return r, nil
		}
	}
	return nil, nil
}

func TestImportProgressJSON_Unauthorized(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger)

	r := chi.NewRouter()
	r.Get("/imports/{id}/progress", handler.ImportProgressJSON)

	req := httptest.NewRequest(http.MethodGet, "/imports/some-id/progress", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized, got %d", rec.Code)
	}
}

func TestImportProgressJSON_FromDatabase(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger)

	mockRepo := newMockImportRunRepo()
	handler.SetImportRunRepo(mockRepo)

	// Preload a run in ready state.
	run := &importrun.Run{
		ID:             1,
		PublicID:       "run-abc-123",
		OrganizationID: 42,
		UserID:         10,
		Kind:           importrun.KindSavingProducts,
		State:          importrun.StateReady,
		Phase:          "Processing complete",
		Percent:        100,
		TotalRows:      20,
		ProcessedRows:  20,
	}
	_ = mockRepo.CreateRun(context.Background(), run)

	rowItem := map[string]any{
		"index":        1,
		"name_product": "Panadol Extra",
		"price_minor":  2500,
	}
	rowData, _ := json.Marshal(rowItem)
	_ = mockRepo.InsertRows(context.Background(), run.ID, []importrun.Row{
		{RunID: run.ID, RowNumber: 1, Data: rowData, Included: true},
	})

	actor := authctx.Actor{
		UserID:         10,
		OrganizationID: 42,
		OrgType:        "vendor",
		OrgStatus:      "approved",
	}

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(authctx.WithActor(req.Context(), actor)))
		})
	})
	r.Get("/imports/{id}/progress", handler.ImportProgressJSON)

	req := httptest.NewRequest(http.MethodGet, "/imports/run-abc-123/progress", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}

	if resp["state"] != importrun.StateReady {
		t.Errorf("expected state %s, got %v", importrun.StateReady, resp["state"])
	}
	if resp["done"] != false { // StateReady is not terminal; StateCommitted/Failed/Cancelled is Done
		t.Errorf("expected done false, got %v", resp["done"])
	}
	if resp["percent"] != float64(100) {
		t.Errorf("expected percent 100, got %v", resp["percent"])
	}

	// Verify items are attached for ready saving products.
	items, ok := resp["items"].([]any)
	if !ok || len(items) != 1 {
		t.Errorf("expected 1 attached item, got %v", resp["items"])
	}
}

func TestImportProgressJSON_FromInMemoryFallback(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger)

	// In-memory session in globalSavingImportSessionStore
	sess := ui.GlobalSavingImportSessionStore().NewSessionWithID("inmem-session-999", 55, 12, "sample.xlsx", 10)

	actor := authctx.Actor{
		UserID:         12,
		OrganizationID: 55,
		OrgType:        "customer",
		OrgStatus:      "approved",
	}

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(authctx.WithActor(req.Context(), actor)))
		})
	})
	r.Get("/imports/{id}/progress", handler.ImportProgressJSON)

	req := httptest.NewRequest(http.MethodGet, "/imports/"+sess.ID+"/progress", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}

	if resp["id"] != "inmem-session-999" {
		t.Errorf("expected session id inmem-session-999, got %v", resp["id"])
	}
}
