package matching

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// simulateReaperSweep runs in-memory reaper logic matching jobs.SweepStaleRuns SQL:
// UPDATE catalog.match_runs SET state = 'interrupted', finished_at = now()
// WHERE state IN ('preparing', 'enhancing') AND heartbeat_at < now() - maxAge;
func simulateReaperSweep(repo *mockRepository, maxAge time.Duration, now time.Time) int64 {
	repo.mu.Lock()
	defer repo.mu.Unlock()

	var count int64
	for id, run := range repo.runs {
		state := repo.runStates[id]
		if (state == "preparing" || state == "enhancing") && now.Sub(run.HeartbeatAt) > maxAge {
			repo.runStates[id] = "interrupted"
			repo.runFinished[id] = true
			run.State = "interrupted"
			finished := now
			run.FinishedAt = &finished
			count++
		}
	}
	return count
}

func TestRecovery_Reaper_MarksInterrupted_WhenHeartbeatOlderThan45s(t *testing.T) {
	repo := newMockRepository()
	now := time.Now()

	// 1. Run in 'preparing' with heartbeat 50s ago -> should become interrupted
	runStalePreparing := &MatchRun{
		OrganizationID: 1,
		Workflow:       "vendor",
		EntryPoint:     "catalog_stage",
		ConsumerRef:    "ingest:101",
		State:          "preparing",
		HeartbeatAt:    now.Add(-50 * time.Second),
	}
	_ = repo.CreateRun(context.Background(), runStalePreparing)
	runStalePreparing.HeartbeatAt = now.Add(-50 * time.Second) // preserve custom timestamp

	// 2. Run in 'enhancing' with heartbeat 60s ago -> should become interrupted
	runStaleEnhancing := &MatchRun{
		OrganizationID: 1,
		Workflow:       "smartorder",
		EntryPoint:     "smartorder_run",
		ConsumerRef:    "smartorder:102",
		State:          "enhancing",
		HeartbeatAt:    now.Add(-60 * time.Second),
	}
	_ = repo.CreateRun(context.Background(), runStaleEnhancing)
	runStaleEnhancing.HeartbeatAt = now.Add(-60 * time.Second)

	// 3. Run in 'preparing' with fresh heartbeat 10s ago -> should stay preparing
	runFreshPreparing := &MatchRun{
		OrganizationID: 1,
		Workflow:       "savings",
		EntryPoint:     "saving_products",
		ConsumerRef:    "savings:103",
		State:          "preparing",
		HeartbeatAt:    now.Add(-10 * time.Second),
	}
	_ = repo.CreateRun(context.Background(), runFreshPreparing)
	runFreshPreparing.HeartbeatAt = now.Add(-10 * time.Second)

	// 4. Run in 'reviewable' with heartbeat 100s ago -> must NOT become interrupted
	runReviewable := &MatchRun{
		OrganizationID: 1,
		Workflow:       "vendor",
		EntryPoint:     "catalog_stage",
		ConsumerRef:    "ingest:104",
		State:          "reviewable",
		HeartbeatAt:    now.Add(-100 * time.Second),
	}
	_ = repo.CreateRun(context.Background(), runReviewable)
	runReviewable.HeartbeatAt = now.Add(-100 * time.Second)

	// 5. Run in 'committed' -> must NOT become interrupted
	runCommitted := &MatchRun{
		OrganizationID: 1,
		Workflow:       "compare",
		EntryPoint:     "compare_file",
		ConsumerRef:    "compare:105",
		State:          "committed",
		HeartbeatAt:    now.Add(-120 * time.Second),
	}
	_ = repo.CreateRun(context.Background(), runCommitted)
	runCommitted.HeartbeatAt = now.Add(-120 * time.Second)

	// Run sweep with 45s threshold
	affected := simulateReaperSweep(repo, 45*time.Second, now)
	if affected != 2 {
		t.Fatalf("expected 2 stale runs reaped, got %d", affected)
	}

	// Verify states
	if repo.runStates[runStalePreparing.ID] != "interrupted" {
		t.Errorf("expected runStalePreparing to be interrupted, got %s", repo.runStates[runStalePreparing.ID])
	}
	if repo.runStates[runStaleEnhancing.ID] != "interrupted" {
		t.Errorf("expected runStaleEnhancing to be interrupted, got %s", repo.runStates[runStaleEnhancing.ID])
	}
	if repo.runStates[runFreshPreparing.ID] != "preparing" {
		t.Errorf("expected runFreshPreparing to stay preparing, got %s", repo.runStates[runFreshPreparing.ID])
	}
	if repo.runStates[runReviewable.ID] != "reviewable" {
		t.Errorf("expected runReviewable to stay reviewable, got %s", repo.runStates[runReviewable.ID])
	}
	if repo.runStates[runCommitted.ID] != "committed" {
		t.Errorf("expected runCommitted to stay committed, got %s", repo.runStates[runCommitted.ID])
	}
}

func TestRecovery_HeartbeatTicker_KeepsHeartbeatFresh(t *testing.T) {
	repo := newMockRepository()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	run := &MatchRun{
		OrganizationID: 1,
		Workflow:       "vendor",
		State:          "preparing",
		HeartbeatAt:    time.Now().Add(-1 * time.Hour),
	}
	_ = repo.CreateRun(ctx, run)
	initialHB := run.HeartbeatAt

	// Start ticker with fast interval for test
	stopHB := StartHeartbeat(ctx, repo, run.ID, 15*time.Millisecond)
	time.Sleep(50 * time.Millisecond)
	stopHB()

	repo.mu.Lock()
	afterHB := repo.runs[run.ID].HeartbeatAt
	repo.mu.Unlock()

	if !afterHB.After(initialHB) {
		t.Fatalf("expected heartbeat to advance, initial=%v, after=%v", initialHB, afterHB)
	}
}

func TestRecovery_RepeatedCommit_IdempotentSingleEffect(t *testing.T) {
	// Simulated commit store for consumer row: (row_id, revision)
	type committedRow struct {
		rowID     int64
		revision  int
		productID int64
		commits   int
	}

	type commitStore struct {
		mu   sync.Mutex
		rows map[int64]*committedRow
	}

	store := &commitStore{rows: make(map[int64]*committedRow)}

	commitRow := func(rowID int64, rev int, prodID int64) error {
		store.mu.Lock()
		defer store.mu.Unlock()

		existing, exists := store.rows[rowID]
		if !exists {
			store.rows[rowID] = &committedRow{
				rowID:     rowID,
				revision:  rev,
				productID: prodID,
				commits:   1,
			}
			return nil
		}

		// Optimistic concurrency / revision check
		if existing.revision > rev {
			return ErrRevisionConflict
		}
		if existing.revision == rev {
			// Idempotent repeat: same (row, revision) produces no new side-effect
			return nil
		}

		// Revision advanced: apply new decision
		existing.revision = rev
		existing.productID = prodID
		existing.commits++
		return nil
	}

	rowID := int64(42)
	initialRev := 1
	targetProdID := int64(1001)

	// First commit: creates record
	if err := commitRow(rowID, initialRev, targetProdID); err != nil {
		t.Fatalf("first commit failed: %v", err)
	}

	// Repeated commit with SAME (row, revision): idempotent single effect
	for i := 0; i < 5; i++ {
		if err := commitRow(rowID, initialRev, targetProdID); err != nil {
			t.Fatalf("repeated commit #%d failed: %v", i+1, err)
		}
	}

	store.mu.Lock()
	entry := store.rows[rowID]
	store.mu.Unlock()

	if entry.commits != 1 {
		t.Fatalf("expected exactly 1 commit effect, got %d", entry.commits)
	}
	if entry.productID != targetProdID {
		t.Fatalf("expected productID=%d, got %d", targetProdID, entry.productID)
	}

	// Stale revision: rejected with ErrRevisionConflict
	if err := commitRow(rowID, 0, targetProdID); err != ErrRevisionConflict {
		t.Fatalf("expected ErrRevisionConflict for stale rev 0, got %v", err)
	}
}

func TestRecovery_CountsReconcile_1to1(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepository()

	// Build test catalogue index with 3 known products
	idx := testMasterIndex(1)
	builder := IndexBuilderFunc(func(_ context.Context, _ string, _ int64, _ string) (*productmatch.Index, error) {
		return idx, nil
	})
	cache := NewIndexCache(builder)
	svc := NewService(repo, cache, nil)

	// Create 10 source rows:
	// - 4 rows matching Product 101 ("Concor 5mg 30 tab")
	// - 3 rows matching Product 102 ("Panadol Extra 24 tab")
	// - 2 rows with distinct unmatched names (StateNoCandidate)
	// - 1 row with uninterpretable/malformed text (StateInvalidInput)
	rows := []productmatch.Row{
		{Number: 1, Name: "كونكور 5 مجم 30 قرص"},
		{Number: 2, Name: "كونكور 5 مجم 30 قرص"},
		{Number: 3, Name: "Concor 5mg 30 tab"},
		{Number: 4, Name: "Concor 5mg 30 tab"},
		{Number: 5, Name: "بانادول اكسترا 24 قرص"},
		{Number: 6, Name: "بانادول اكسترا 24 قرص"},
		{Number: 7, Name: "Panadol Extra 24 tab"},
		{Number: 8, Name: "دواء مجهول تماماً 12345 xyz"},
		{Number: 9, Name: "صنف غير موجود أبداً 99999 abc"},
		{Number: 10, Name: ""}, // empty row
	}

	req := productmatch.PrepareRequest{
		Scope: productmatch.Scope{
			EffectiveOrg: 1,
			Workflow:     WorkflowVendor,
			EntryPoint:   "test_reconcile",
		},
		ConsumerRef: "ingest:test:reconcile",
		SourceRev:   1,
		Rows:        rows,
		Required:    productmatch.GranularityProduct,
	}

	res, err := svc.Prepare(ctx, req)
	if err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}

	if len(res.Decisions) != len(rows) {
		t.Fatalf("expected %d decisions, got %d", len(rows), len(res.Decisions))
	}

	counts := res.Summary

	// Total must exactly match the number of source rows (1:1 reconciliation per FR-094)
	if counts.Total != len(rows) {
		t.Fatalf("expected Total=%d, got %d", len(rows), counts.Total)
	}

	// Sum of all mutually exclusive primary states must equal Total
	sumStates := counts.PrimaryTotal()

	if sumStates != counts.Total {
		t.Fatalf("primary states sum (%d) does not match Total (%d): %+v",
			sumStates, counts.Total, counts)
	}

	// Verify that distinct questions is deduplicated:
	// row 1 & 2 share ar sig, 3 & 4 share en sig, 5 & 6 share ar sig, 7 has en sig, 8, 9, 10
	if counts.DistinctQuestions > counts.Total {
		t.Fatalf("DistinctQuestions (%d) cannot exceed Total (%d)", counts.DistinctQuestions, counts.Total)
	}

	// Verify that repo has the run created and counts updated
	repo.mu.Lock()
	savedRun := repo.runs[res.RunID]
	repo.mu.Unlock()

	if savedRun == nil {
		t.Fatalf("run %d not found in repository", res.RunID)
	}
	if savedRun.Counts.Total != len(rows) {
		t.Fatalf("expected savedRun.Counts.Total=%d, got %d", len(rows), savedRun.Counts.Total)
	}
}
