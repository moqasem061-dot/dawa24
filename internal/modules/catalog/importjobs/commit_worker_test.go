package importjobs

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	"github.com/riverqueue/river"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
	"github.com/moqasem061-dot/dawa24/internal/modules/catalog/matching"
	"github.com/moqasem061-dot/dawa24/internal/platform/importrun"
	"github.com/moqasem061-dot/dawa24/internal/platform/queue"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

type mockCommitMatchingSvc struct {
	revalFn func(ctx context.Context, s matching.Scope, ref matching.RowRef, d matchflow.RowDecision) (matchflow.RowDecision, error)
}

func (m *mockCommitMatchingSvc) Prepare(context.Context, matching.PrepareRequest) (matching.PrepareResult, error) {
	return matching.PrepareResult{}, nil
}
func (m *mockCommitMatchingSvc) GetRun(context.Context, int64) (*matching.MatchRun, error) {
	return nil, nil
}
func (m *mockCommitMatchingSvc) ListRunRows(context.Context, matching.Scope, int64, int) ([]matching.DiagnosticRowData, error) {
	return nil, nil
}
func (m *mockCommitMatchingSvc) Diagnose(context.Context, matching.Scope, int64, productmatch.Row, matchflow.RowDecision) (matching.DiagnosticRow, error) {
	return matching.DiagnosticRow{}, nil
}
func (m *mockCommitMatchingSvc) MarkReviewable(context.Context, int64) error { return nil }
func (m *mockCommitMatchingSvc) Rematch(context.Context, matching.Scope, productmatch.Row, int) (matchflow.RowDecision, error) {
	return matchflow.RowDecision{}, nil
}
func (m *mockCommitMatchingSvc) Search(context.Context, matching.Scope, string, productmatch.Row, int) ([]matchflow.CandidateView, error) {
	return nil, nil
}
func (m *mockCommitMatchingSvc) Review(context.Context, matching.Scope, matching.ReviewAction) (matchflow.RowDecision, error) {
	return matchflow.RowDecision{}, nil
}
func (m *mockCommitMatchingSvc) PreviewBulk(context.Context, matching.Scope, matching.ReviewAction, []matching.RowRef) (matching.BulkPreview, error) {
	return matching.BulkPreview{}, nil
}
func (m *mockCommitMatchingSvc) StartEnhancement(context.Context, int64, []matching.EnhanceRow) (matching.EnhanceHandle, error) {
	return matching.EnhanceHandle{}, nil
}
func (m *mockCommitMatchingSvc) StopEnhancement(context.Context, int64) error { return nil }
func (m *mockCommitMatchingSvc) Revalidate(ctx context.Context, s matching.Scope, ref matching.RowRef, d matchflow.RowDecision) (matchflow.RowDecision, error) {
	if m.revalFn != nil {
		return m.revalFn(ctx, s, ref, d)
	}
	return d, nil
}
func (m *mockCommitMatchingSvc) Remember(context.Context, matching.Scope, matching.KnowledgeWrite) error {
	return nil
}
func (m *mockCommitMatchingSvc) ListKnowledge(context.Context, matching.Scope, matching.KnowledgeFilter) (matching.Page[matching.Knowledge], error) {
	return matching.Page[matching.Knowledge]{}, nil
}
func (m *mockCommitMatchingSvc) RevokeKnowledge(context.Context, matching.Scope, int64) error {
	return nil
}
func (m *mockCommitMatchingSvc) PromoteKnowledge(context.Context, matching.Scope, int64) error {
	return nil
}

type mockCatalogRepoForCommit struct {
	catalog.Repository
	upserted []*catalog.SavingProduct
}

func (m *mockCatalogRepoForCommit) BatchUpsertSavingProducts(ctx context.Context, orgID int64, userID *int64, items []*catalog.SavingProduct) (int, int, error) {
	m.upserted = append(m.upserted, items...)
	return len(items), 0, nil
}

type testImportRunRepo struct {
	runs map[int64]*importrun.Run
	rows map[int64][]importrun.Row
}

func newTestImportRunRepo() *testImportRunRepo {
	return &testImportRunRepo{
		runs: make(map[int64]*importrun.Run),
		rows: make(map[int64][]importrun.Row),
	}
}

func (m *testImportRunRepo) CreateRun(ctx context.Context, run *importrun.Run) error { return nil }
func (m *testImportRunRepo) GetRunByPublicID(ctx context.Context, publicID string, orgID int64) (*importrun.Run, error) {
	return nil, nil
}
func (m *testImportRunRepo) GetRunByPublicIDSystem(ctx context.Context, publicID string) (*importrun.Run, error) {
	return nil, nil
}
func (m *testImportRunRepo) GetRunByID(ctx context.Context, id int64) (*importrun.Run, error) {
	return m.runs[id], nil
}
func (m *testImportRunRepo) UpdateProgress(ctx context.Context, id int64, phase string, percent int, processed int) error {
	if r, ok := m.runs[id]; ok {
		r.Phase = phase
		r.Percent = percent
		r.ProcessedRows = processed
	}
	return nil
}
func (m *testImportRunRepo) TransitionState(ctx context.Context, id int64, newState string) error {
	if r, ok := m.runs[id]; ok {
		r.State = newState
	}
	return nil
}
func (m *testImportRunRepo) FailRun(ctx context.Context, id int64, errMsg string) error {
	if r, ok := m.runs[id]; ok {
		r.State = importrun.StateFailed
		r.ErrorMessage = errMsg
	}
	return nil
}
func (m *testImportRunRepo) SetResult(ctx context.Context, id int64, result json.RawMessage) error {
	if r, ok := m.runs[id]; ok {
		r.Result = result
	}
	return nil
}
func (m *testImportRunRepo) SetRiverJobID(ctx context.Context, id int64, jobID int64) error {
	return nil
}
func (m *testImportRunRepo) ListRunsByOrg(ctx context.Context, orgID int64, kind string, limit, offset int) ([]*importrun.Run, int, error) {
	return nil, 0, nil
}
func (m *testImportRunRepo) InsertRows(ctx context.Context, runID int64, rows []importrun.Row) error {
	return nil
}
func (m *testImportRunRepo) ListRows(ctx context.Context, runID int64, onlyIncluded bool, limit, offset int) ([]importrun.Row, int, error) {
	rows := m.rows[runID]
	var res []importrun.Row
	for _, r := range rows {
		if !onlyIncluded || r.Included {
			res = append(res, r)
		}
	}
	return res, len(res), nil
}
func (m *testImportRunRepo) UpdateRow(ctx context.Context, rowID int64, data json.RawMessage, included *bool, matchedProductID *int64) error {
	return nil
}
func (m *testImportRunRepo) GetActiveRunForUser(ctx context.Context, userID int64, kind string) (*importrun.Run, error) {
	return nil, nil
}
func (m *testImportRunRepo) UpdatePayload(ctx context.Context, id int64, payload json.RawMessage) error {
	return nil
}
func (m *testImportRunRepo) RecoverStaleRuns(ctx context.Context) (int, error) {
	return 0, nil
}

func TestCommitWorker_RevalidatesEachRowAndRefusesNonAccepted(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	prod1 := int64(101)
	prod2 := int64(102)

	mockMatching := &mockCommitMatchingSvc{
		revalFn: func(ctx context.Context, s matching.Scope, ref matching.RowRef, d matchflow.RowDecision) (matchflow.RowDecision, error) {
			if *d.ProductID == prod1 {
				// prod1 is accepted
				return matchflow.RowDecision{
					State:     matchflow.StateAccepted,
					ProductID: &prod1,
					Revision:  d.Revision,
					Granularity: matchflow.GranularityPair{
						Achieved: matchflow.GranularityProduct,
					},
				}, nil
			}
			// prod2 has become ambiguous / needs_review
			return matchflow.RowDecision{
				State:     matchflow.StateNeedsReview,
				ProductID: &prod2,
				Revision:  d.Revision,
				Granularity: matchflow.GranularityPair{
					Achieved: matchflow.GranularityProduct,
				},
			}, nil
		},
	}

	catRepo := &mockCatalogRepoForCommit{}
	catSvc := catalog.NewService(catRepo, logger)
	catSvc.SetMatchingService(mockMatching)

	runRepo := newTestImportRunRepo()
	runID := int64(55)
	runRepo.runs[runID] = &importrun.Run{
		ID:             runID,
		Kind:           importrun.KindSavingProducts,
		State:          importrun.StateReady,
		OrganizationID: 10,
		UserID:         1,
	}

	data1, _ := json.Marshal(SavingRowData{
		Index:       0,
		NameProduct: "Panadol",
		SKU:         "PAN-1",
		Quantity:    10,
		PriceMinor:  1500,
		ProductID:   &prod1,
	})
	data2, _ := json.Marshal(SavingRowData{
		Index:       1,
		NameProduct: "Brufen",
		SKU:         "BRU-1",
		Quantity:    5,
		PriceMinor:  2000,
		ProductID:   &prod2,
	})

	runRepo.rows[runID] = []importrun.Row{
		{ID: 1, RunID: runID, RowNumber: 1, Included: true, Data: data1, MatchedProductID: &prod1},
		{ID: 2, RunID: runID, RowNumber: 2, Included: true, Data: data2, MatchedProductID: &prod2},
	}

	worker := NewCommitWorker(nil, runRepo, catSvc, logger)
	job := &river.Job[queue.ImportCommitArgs]{
		Args: queue.ImportCommitArgs{RunID: runID},
	}

	if err := worker.Work(ctx, job); err != nil {
		t.Fatalf("worker.Work failed: %v", err)
	}

	// Only prod1 must be committed! prod2 was rejected during revalidation!
	if len(catRepo.upserted) != 1 {
		t.Fatalf("expected 1 committed item, got %d", len(catRepo.upserted))
	}
	if *catRepo.upserted[0].ProductID != prod1 {
		t.Errorf("expected product ID %d, got %d", prod1, *catRepo.upserted[0].ProductID)
	}
	if runRepo.runs[runID].State != importrun.StateCommitted {
		t.Errorf("expected run state committed, got %s", runRepo.runs[runID].State)
	}
}
