package ingest

import (
	"context"
	"log/slog"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
)

type aiTrackingStore struct {
	session    *Session
	fileBytes  []byte
	stagedRows []*RowOutcome
	phases     []Phase
}

func (s *aiTrackingStore) Sweep(_ context.Context) error { return nil }

func (s *aiTrackingStore) Create(_ context.Context, sess *Session, file []byte) error {
	s.session = sess
	s.fileBytes = file
	return nil
}
func (s *aiTrackingStore) Get(_ context.Context, _ string) (*Session, error) {
	return s.session, nil
}
func (s *aiTrackingStore) File(_ context.Context, _ int64) ([]byte, error) {
	return s.fileBytes, nil
}
func (s *aiTrackingStore) SaveDraft(_ context.Context, sess *Session) error {
	s.session = sess
	s.phases = append(s.phases, sess.Phase)
	return nil
}
func (s *aiTrackingStore) Begin(_ context.Context, _ int64) error       { return nil }
func (s *aiTrackingStore) BeginCommit(_ context.Context, _ int64) error { return nil }
func (s *aiTrackingStore) Progress(_ context.Context, _ int64, _ int, _ string) error {
	return nil
}
func (s *aiTrackingStore) FinishStaging(_ context.Context, sess *Session) error {
	s.session = sess
	s.phases = append(s.phases, sess.Phase)
	return nil
}
func (s *aiTrackingStore) RecoverStaleRuns(_ context.Context) (int, error) { return 0, nil }
func (s *aiTrackingStore) Finish(_ context.Context, sess *Session) error {
	s.session = sess
	s.phases = append(s.phases, sess.Phase)
	return nil
}
func (s *aiTrackingStore) Fail(_ context.Context, _ int64, _ string) error { return nil }
func (s *aiTrackingStore) Cancel(_ context.Context, _ int64) error         { return nil }
func (s *aiTrackingStore) List(_ context.Context, _ int64, _ int) ([]*Session, error) {
	return nil, nil
}
func (s *aiTrackingStore) AppendRows(_ context.Context, _, _ int64, rows []RowOutcome) error {
	for i := range rows {
		r := rows[i]
		s.stagedRows = append(s.stagedRows, &r)
	}
	return nil
}
func (s *aiTrackingStore) ClearRows(_ context.Context, _ int64) error {
	s.stagedRows = nil
	return nil
}
func (s *aiTrackingStore) Rows(_ context.Context, _ int64, _ RowFilter) ([]*RowOutcome, int, error) {
	return s.stagedRows, len(s.stagedRows), nil
}
func (s *aiTrackingStore) RowCounts(_ context.Context, _ int64) (map[string]int, error) {
	return map[string]int{}, nil
}
func (s *aiTrackingStore) ApplyAIMatches(_ context.Context, _ int64, _ []AIMatch) error {
	return nil
}
func (s *aiTrackingStore) UpdateRow(_ context.Context, _, _ int64, _ string, _ string, _, _, _, _ *float64, _ *int, _ *bool) error {
	return nil
}
func (s *aiTrackingStore) SetBatchQuantity(_ context.Context, _ int64, _ int) error { return nil }
func (s *aiTrackingStore) AssignRowMatch(_ context.Context, _, _, _ int64, _, _ string) error {
	return nil
}
func (s *aiTrackingStore) ToggleRowExclude(_ context.Context, _, _ int64) (bool, error) {
	return false, nil
}
func (s *aiTrackingStore) ConfirmRowMatches(_ context.Context, _ int64, _ []int64) (int, error) {
	return 0, nil
}
func (s *aiTrackingStore) ClearRowMatches(_ context.Context, _ int64, _ []int64) (int, error) {
	return 0, nil
}
func (s *aiTrackingStore) SetRowsExcluded(_ context.Context, _ int64, _ []int64, _ bool) (int, error) {
	return 0, nil
}
func (s *aiTrackingStore) PendingRowIDs(_ context.Context, _ int64, _ int) ([]int64, int, error) {
	return nil, 0, nil
}
func (s *aiTrackingStore) RowIDsForFilter(_ context.Context, _ int64, _ RowFilter, _ int) ([]int64, error) {
	return nil, nil
}
func (s *aiTrackingStore) StagedRowsForCommit(_ context.Context, _ int64) ([]*RowOutcome, error) {
	return s.stagedRows, nil
}
func (s *aiTrackingStore) MentionedRows(_ context.Context, _ int64) ([]RowMention, error) {
	return nil, nil
}
func (s *aiTrackingStore) UpdateCommittedRows(_ context.Context, _ int64, _ []RowOutcome) error {
	return nil
}

// TestVendorImport_AI_Off verifies that when the AI enhancer is nil:
// 1. 0 enhancer calls are made
// 2. ai_state="off" on every staged row
// 3. The run transitions directly to reviewable (PhaseReview) without an AI waiting stage
// 4. Commit succeeds
func TestVendorImport_AI_Off(t *testing.T) {
	ctx := context.Background()

	// Master catalogue products
	products := []MatchProduct{
		{
			ID:          1001,
			NameAR:      "بنادول اكسترا 500 مجم",
			NameEN:      "Panadol Extra 500mg",
			SKU:         "PAN-EXT-500",
			PublicPrice: "30.00",
		},
		{
			ID:          1002,
			NameAR:      "أموكسيسيلين 500 مجم",
			NameEN:      "Amoxicillin 500mg",
			SKU:         "AMX-500",
			PublicPrice: "45.00",
		},
	}

	catMock := &mockCommitCatalogPort{
		products:      products,
		keys:          []VariantKey{},
		nextVariantID: 5000,
	}
	invMock := &mockCommitInventoryPort{
		warehouses: []Warehouse{{ID: 1}},
	}

	csvData := []byte("name,price,quantity\nPanadol Extra 500mg,30,10\nAmoxicillin 500mg,45,20\n")

	session := &Session{
		ID:             1,
		PublicID:       "imp-ai-off-test",
		OrganizationID: 10,
		Filename:       "vendor.csv",
		Phase:          PhaseSettings,
		Settings: Settings{
			WarehouseID:   1,
			Mode:          ModeUpsert,
			StockMode:     StockReplace,
			UseAI:         false, // AI explicitly off
			MinMatchScore: 0.60,
		},
	}

	store := &aiTrackingStore{
		session:   session,
		fileBytes: csvData,
	}

	svc := NewService(nil, slog.Default())
	svc.SetImportStore(store)
	svc.SetCatalogPort(catMock)
	svc.SetInventoryPort(invMock)
	// AI enhancer is explicitly nil
	svc.SetEnhancer(nil)

	if svc.AIAvailable() {
		t.Fatalf("expected AIAvailable() to be false when enhancer is nil")
	}

	// 1. Stage the import
	err := svc.StageImport(ctx, session)
	if err != nil {
		t.Fatalf("StageImport failed: %v", err)
	}

	// 2. Assert run state is reviewable without any AI waiting stage
	if session.Phase != PhaseReview {
		t.Fatalf("expected session phase %s, got %s", PhaseReview, session.Phase)
	}

	for _, p := range store.phases {
		if p == "ai_waiting" || p == "enhancing" {
			t.Fatalf("unexpected intermediate AI phase observed: %s", p)
		}
	}

	// 3. Assert every staged row has ai_state="off"
	if len(store.stagedRows) != 2 {
		t.Fatalf("expected 2 staged rows, got %d", len(store.stagedRows))
	}

	for i, row := range store.stagedRows {
		if row.AIState != string(matchflow.AIStateOff) {
			t.Errorf("row %d: expected ai_state=%q, got %q", i+1, matchflow.AIStateOff, row.AIState)
		}
	}

	// 4. Confirm and Commit
	committedSess, err := svc.CommitImport(ctx, session.PublicID)
	if err != nil {
		t.Fatalf("CommitImport failed: %v", err)
	}

	if committedSess.Phase != PhaseCompleted {
		t.Errorf("expected session phase %s after commit, got %s", PhaseCompleted, committedSess.Phase)
	}
	if committedSess.InsertedRows != 2 {
		t.Errorf("expected 2 inserted rows, got %d", committedSess.InsertedRows)
	}
	if len(catMock.writtenVariants) != 2 {
		t.Errorf("expected 2 variants written to catalog, got %d", len(catMock.writtenVariants))
	}
	if len(invMock.writtenRows) != 2 {
		t.Errorf("expected 2 inventory rows written, got %d", len(invMock.writtenRows))
	}
}
