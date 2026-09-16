package ui_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/importrun"
	"github.com/muhiya/dawa24-store/internal/ui"
)

type mockCompareFileService struct {
	files map[int64]*compare.CompareFile
}

func newMockCompareFileService() *mockCompareFileService {
	return &mockCompareFileService{
		files: make(map[int64]*compare.CompareFile),
	}
}

func (m *mockCompareFileService) addFile(f *compare.CompareFile) {
	m.files[f.ID] = f
}

func setupResumableTestHandler() (*ui.UIHandler, *mockImportRunRepo, *mockCompareFileService) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	runRepo := newMockImportRunRepo()
	compMock := newMockCompareFileService()

	handler := ui.NewUIHandler(
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, log,
	)
	handler.SetImportRunRepo(runRepo)

	return handler, runRepo, compMock
}

func TestTempWarehouseRun_CreateAndResolve(t *testing.T) {
	handler, _, _ := setupResumableTestHandler()
	ctx := context.Background()

	actor := authctx.Actor{
		UserID:         41,
		OrganizationID: 10,
		Role:           "moderator",
	}

	fileIDs := []int64{101, 102}
	suppNames := map[int64]string{
		101: "Supplier Alpha",
		102: "Supplier Beta",
	}
	baseURL := "/admin/user/temparte-warehouses"

	run, err := handler.CreateTempWarehouseRun(ctx, actor, fileIDs, suppNames, baseURL)
	if err != nil {
		t.Fatalf("unexpected error creating run: %v", err)
	}
	if run == nil || run.ID <= 0 {
		t.Fatalf("expected valid run with ID > 0, got: %v", run)
	}
	if run.Kind != importrun.KindTempWarehouse {
		t.Errorf("expected kind %s, got: %s", importrun.KindTempWarehouse, run.Kind)
	}
	if run.Phase != ui.PhaseMapping {
		t.Errorf("expected initial phase %s, got: %s", ui.PhaseMapping, run.Phase)
	}

	// Resolve by numeric ID
	resolved, err := handler.ResolveTempWarehouseRun(ctx, strconv.FormatInt(run.ID, 10))
	if err != nil || resolved == nil {
		t.Fatalf("failed to resolve run by numeric ID: %v", err)
	}
	if resolved.ID != run.ID {
		t.Errorf("resolved ID %d != run ID %d", resolved.ID, run.ID)
	}

	// Resolve active run for user
	activeRun, payload := handler.GetActiveTempWarehouseRun(ctx, actor.UserID)
	if activeRun == nil || payload == nil {
		t.Fatalf("expected active run and payload for user %d, got nil", actor.UserID)
	}
	if activeRun.ID != run.ID {
		t.Errorf("active run ID %d != %d", activeRun.ID, run.ID)
	}
	if len(payload.FileIDs) != 2 {
		t.Errorf("expected 2 file IDs in payload, got: %d", len(payload.FileIDs))
	}
}

func TestTempWarehouseRun_DispatcherPhases(t *testing.T) {
	handler, runRepo, _ := setupResumableTestHandler()

	run := &importrun.Run{
		UserID: 41,
		Kind:   importrun.KindTempWarehouse,
		State:  importrun.StateProcessing,
		Phase:  ui.PhaseMapping,
	}
	_ = runRepo.CreateRun(context.Background(), run)

	tests := []struct {
		phase       string
		expectedURL string
	}{
		{ui.PhaseMapping, "/admin/user/temparte-warehouses/runs/1/mapping"},
		{ui.PhaseReview, "/admin/user/temparte-warehouses/runs/1/review"},
		{ui.PhaseStaging, "/admin/user/temparte-warehouses/runs/1/progress"},
		{ui.PhaseCommitting, "/admin/user/temparte-warehouses/runs/1/progress"},
	}

	for _, tc := range tests {
		run.Phase = tc.phase
		r := httptest.NewRequest(http.MethodGet, "/admin/user/temparte-warehouses/runs/1", nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("runID", "1")
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))

		w := httptest.NewRecorder()
		handler.AdminTempWarehouseRunDispatcher(w, r)

		resp := w.Result()
		loc := resp.Header.Get("Location")
		if loc != tc.expectedURL {
			t.Errorf("for phase %s, expected redirect to %s, got: %s", tc.phase, tc.expectedURL, loc)
		}
	}
}

func TestTempWarehouseRun_AutosaveAndNavigation(t *testing.T) {
	handler, _, _ := setupResumableTestHandler()
	ctx := context.Background()

	actor := authctx.Actor{UserID: 41, Role: "moderator"}
	run, _ := handler.CreateTempWarehouseRun(ctx, actor, []int64{201, 202}, map[int64]string{201: "Supp A", 202: "Supp B"}, "/admin/user/temparte-warehouses")

	// 1. Test autosave debounced POST via AJAX
	form := url.Values{}
	form.Set("file_id", "201")
	form.Set("col_name", "1")
	form.Set("col_code", "0")
	form.Set("col_price", "2")
	form.Set("col_discount", "3")
	form.Set("is_autosave", "1")

	req := httptest.NewRequest(http.MethodPost, "/admin/user/temparte-warehouses/runs/1/mapping", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("runID", strconv.FormatInt(run.ID, 10))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	handler.AdminTempWarehouseRunMappingSubmit(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for autosave, got: %d", w.Code)
	}

	var jsonResp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&jsonResp); err != nil {
		t.Fatalf("failed to decode JSON autosave response: %v", err)
	}
	if jsonResp["success"] != true {
		t.Errorf("expected success true in autosave, got: %v", jsonResp)
	}

	// Verify payload was updated in memory/repo
	updatedRun, _ := handler.ResolveTempWarehouseRun(ctx, strconv.FormatInt(run.ID, 10))
	p, _ := ui.DecodeTempWarehousePayload(updatedRun.Payload)
	cfg, ok := p.Mappings[201]
	if !ok || cfg.NameCol == nil || *cfg.NameCol != 1 {
		t.Errorf("expected Mappings[201] to have NameCol=1, got: %+v", cfg)
	}

	// 2. Test action=next moves to next file (202)
	formNext := url.Values{}
	formNext.Set("file_id", "201")
	formNext.Set("col_name", "1")
	formNext.Set("action", "next")

	reqNext := httptest.NewRequest(http.MethodPost, "/admin/user/temparte-warehouses/runs/1/mapping", strings.NewReader(formNext.Encode()))
	reqNext.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqNext = reqNext.WithContext(context.WithValue(reqNext.Context(), chi.RouteCtxKey, rctx))

	wNext := httptest.NewRecorder()
	handler.AdminTempWarehouseRunMappingSubmit(wNext, reqNext)

	if loc := wNext.Header().Get("Location"); !strings.Contains(loc, "file_id=202") {
		t.Errorf("expected redirect to next file (file_id=202), got: %s", loc)
	}

	// 3. Test action=review moves to review page
	formReview := url.Values{}
	formReview.Set("file_id", "202")
	formReview.Set("col_name", "1")
	formReview.Set("action", "review")

	reqReview := httptest.NewRequest(http.MethodPost, "/admin/user/temparte-warehouses/runs/1/mapping", strings.NewReader(formReview.Encode()))
	reqReview.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqReview = reqReview.WithContext(context.WithValue(reqReview.Context(), chi.RouteCtxKey, rctx))

	wReview := httptest.NewRecorder()
	handler.AdminTempWarehouseRunMappingSubmit(wReview, reqReview)

	if loc := wReview.Header().Get("Location"); !strings.Contains(loc, "/review") {
		t.Errorf("expected redirect to /review, got: %s", loc)
	}
}

func TestTempWarehouseRun_CommitAndCancel(t *testing.T) {
	handler, _, _ := setupResumableTestHandler()
	ctx := context.Background()

	actor := authctx.Actor{UserID: 41, Role: "moderator"}
	run, _ := handler.CreateTempWarehouseRun(ctx, actor, []int64{301}, map[int64]string{301: "Supp C"}, "/admin/my/temparte-warehouses")

	// Test Commit
	reqCommit := httptest.NewRequest(http.MethodPost, "/admin/my/temparte-warehouses/runs/1/commit", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("runID", strconv.FormatInt(run.ID, 10))
	reqCommit = reqCommit.WithContext(context.WithValue(reqCommit.Context(), chi.RouteCtxKey, rctx))

	wCommit := httptest.NewRecorder()
	handler.AdminTempWarehouseRunCommitSubmit(wCommit, reqCommit)

	if loc := wCommit.Header().Get("Location"); !strings.Contains(loc, "/progress") {
		t.Errorf("expected redirect to /progress after commit, got: %s", loc)
	}

	// Verify committed state
	commRun, _ := handler.ResolveTempWarehouseRun(ctx, strconv.FormatInt(run.ID, 10))
	if commRun.State != importrun.StateCommitted {
		t.Errorf("expected state %s, got: %s", importrun.StateCommitted, commRun.State)
	}
	if commRun.Phase != ui.PhaseDone {
		t.Errorf("expected phase %s, got: %s", ui.PhaseDone, commRun.Phase)
	}

	// Test Cancel on new run
	run2, _ := handler.CreateTempWarehouseRun(ctx, actor, []int64{302}, map[int64]string{302: "Supp D"}, "/admin/my/temparte-warehouses")
	reqCancel := httptest.NewRequest(http.MethodPost, "/admin/my/temparte-warehouses/runs/2/cancel", nil)
	rctx2 := chi.NewRouteContext()
	rctx2.URLParams.Add("runID", strconv.FormatInt(run2.ID, 10))
	reqCancel = reqCancel.WithContext(context.WithValue(reqCancel.Context(), chi.RouteCtxKey, rctx2))

	wCancel := httptest.NewRecorder()
	handler.AdminTempWarehouseRunCancelSubmit(wCancel, reqCancel)

	cancelledRun, _ := handler.ResolveTempWarehouseRun(ctx, strconv.FormatInt(run2.ID, 10))
	if cancelledRun.State != importrun.StateCancelled {
		t.Errorf("expected state cancelled, got: %s", cancelledRun.State)
	}
}

func TestTempWarehouseRun_ProgressPollingJSON(t *testing.T) {
	handler, runRepo, _ := setupResumableTestHandler()

	run := &importrun.Run{
		UserID:  41,
		Kind:    importrun.KindTempWarehouse,
		State:   importrun.StateProcessing,
		Phase:   ui.PhaseMapping,
		Percent: 45,
	}
	_ = runRepo.CreateRun(context.Background(), run)

	req := httptest.NewRequest(http.MethodGet, "/admin/user/temparte-warehouses/runs/1/progress?poll=1", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("runID", "1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	handler.AdminTempWarehouseRunProgressPage(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got: %d", w.Code)
	}

	var data map[string]any
	if err := json.NewDecoder(w.Body).Decode(&data); err != nil {
		t.Fatalf("failed to decode JSON progress: %v", err)
	}
	if data["percent"] != float64(45) {
		t.Errorf("expected percent 45, got: %v", data["percent"])
	}
	if data["done"] != false {
		t.Errorf("expected done false, got: %v", data["done"])
	}
}
