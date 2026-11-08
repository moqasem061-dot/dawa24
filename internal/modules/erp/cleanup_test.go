package erp

import (
	"testing"
	"time"
)

// The retention sweep is fixed by the Retention* constants and bounded by
// CleanupBatchRows; order_imports and idempotency_records are never purged
// (they are the ledger the ERP integrations reconcile against).
func TestCleanupCutoffsAndBounds(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo, Ports{}, erpCleanupTestConfig(), nil)
	now := time.Date(2027, 3, 15, 12, 0, 0, 0, time.UTC)

	if err := svc.Cleanup(testCtx, now); err != nil {
		t.Fatal(err)
	}
	if repo.fakeSync.purgerCalls != 1 {
		t.Fatalf("purger calls = %d, want 1", repo.fakeSync.purgerCalls)
	}
	cut := repo.fakeSync.purgerCutoffs
	expect := map[string]time.Time{
		"api_calls":    now.Add(-RetentionAPICalls),
		"sync_items":   now.Add(-RetentionSyncItems),
		"sync_batches": now.Add(-RetentionSyncBatches),
		"review":       now.Add(-RetentionReview),
		"match_runs":   now.Add(-RetentionMatchRuns),
	}
	if !cut.APICalls.Equal(expect["api_calls"]) {
		t.Errorf("api_calls cutoff = %v, want %v", cut.APICalls, expect["api_calls"])
	}
	if !cut.SyncItems.Equal(expect["sync_items"]) {
		t.Errorf("sync_items cutoff = %v, want %v", cut.SyncItems, expect["sync_items"])
	}
	if !cut.SyncBatches.Equal(expect["sync_batches"]) {
		t.Errorf("sync_batches cutoff = %v, want %v", cut.SyncBatches, expect["sync_batches"])
	}
	if !cut.Review.Equal(expect["review"]) {
		t.Errorf("review cutoff = %v, want %v", cut.Review, expect["review"])
	}
	if !cut.MatchRuns.Equal(expect["match_runs"]) {
		t.Errorf("match_runs cutoff = %v, want %v", cut.MatchRuns, expect["match_runs"])
	}
	if cut.BatchRows != CleanupBatchRows {
		t.Errorf("BatchRows = %d, want %d", cut.BatchRows, CleanupBatchRows)
	}
	// The constants stand exactly as the plan named them, so a silent change
	// cannot shorten a tenant's ledger or lengthen its traces.
	if RetentionAPICalls != 30*24*time.Hour || RetentionSyncItems != 7*24*time.Hour ||
		RetentionSyncBatches != 90*24*time.Hour || RetentionReview != 180*24*time.Hour ||
		RetentionMatchRuns != 30*24*time.Hour {
		t.Error("retention constants drifted from the plan")
	}
	if CleanupBatchRows != 10000 {
		t.Errorf("CleanupBatchRows = %d", CleanupBatchRows)
	}
}

func TestCleanupRepeatsNightly(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo, Ports{}, erpCleanupTestConfig(), nil)
	if err := svc.Cleanup(testCtx, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := svc.Cleanup(testCtx, time.Now()); err != nil {
		t.Fatal(err)
	}
	if repo.fakeSync.purgerCalls != 2 {
		t.Errorf("purger calls = %d, want 2", repo.fakeSync.purgerCalls)
	}
}

func erpCleanupTestConfig() Config {
	return Config{Pepper: []byte("cleanup-pepper-32-bytes-long!!"), Tag: "test", DefaultRate: 600, MaxBatchItems: 50000, Provider: "orgasoft"}
}
