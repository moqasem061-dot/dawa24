package integration_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
)

// SC-004: the same sync delivered again (a retried batch, an ERP retry) must
// not multiply variants or links. 100 identical runs: the first creates, the
// other 99 are unchanged by hash and never call the matcher again.
func TestERPSyncFlow_Repeat100(t *testing.T) {
	f := newFlowFixture(t, matchflow.StateAccepted)
	body := erpBatchBody(erpItem("R100", "21.50", 3), erpItem("R101", "14.00", 8))
	beforeVariants := f.variantCount()
	beforeLinks := f.linkCount()

	accepted1, batch1 := f.acceptAndProcess(body)
	if batch1.CreatedCount != 2 {
		t.Fatalf("first run created = %d, want 2", batch1.CreatedCount)
	}

	start := time.Now()
	for i := 0; i < 99; i++ {
		_, batch := f.acceptAndProcess(body)
		if batch.State != "completed" {
			t.Fatalf("run %d state = %q", i+2, batch.State)
		}
		if batch.CreatedCount != 0 || batch.UpdatedCount != 0 {
			t.Fatalf("run %d created %d updated %d, want 0/0 (unchanged by hash)", i+2, batch.CreatedCount, batch.UpdatedCount)
		}
		if batch.UnchangedCount != 2 {
			t.Fatalf("run %d unchanged = %d, want 2", i+2, batch.UnchangedCount)
		}
	}
	t.Logf("99 repeat runs took %s", time.Since(start).Round(time.Millisecond))

	if f.variantCount() != beforeVariants+2 {
		t.Errorf("variants = %d, want %d (seed + 2, no duplicates)", f.variantCount(), beforeVariants+2)
	}
	if f.linkCount() != beforeLinks+2 {
		t.Errorf("links = %d, want %d (no duplicate links)", f.linkCount(), beforeLinks+2)
	}
	if f.matcher.requests != 1 {
		t.Errorf("matcher calls = %d, want 1 (runs 2-100 are unchanged, tiers only)", f.matcher.requests)
	}
	if got := accepted1.SyncID; got == "" {
		t.Error("empty syncId on the first run")
	}
}

// Two batches of the same integration are processed in acceptance order while
// the earlier one is queued or processing (research D-09): the later batch's
// worker snoozes instead of racing ahead.
func TestERPSyncFlow_TwoBatchesInOrder(t *testing.T) {
	f := newFlowFixture(t, matchflow.StateAccepted)
	bodyA := erpBatchBody(erpItem("ORDA", "11.00", 1))
	bodyB := erpBatchBody(erpItem("ORDB", "12.00", 1))

	acceptedA, batchA := f.acceptOnly(bodyA, "")
	acceptedB, batchB := f.acceptOnly(bodyB, "")
	if batchA.ID >= batchB.ID {
		t.Fatalf("fixture order wrong: %d >= %d", batchA.ID, batchB.ID)
	}

	err := f.svc.ProcessChunk(database.WithTenant(f.ctx, f.org), batchB.ID, 0)
	if err == nil {
		t.Fatal("batch 2 processed while batch 1 was queued")
	}
	if err != erp.ErrWaitForEarlierBatch {
		t.Fatalf("err = %v, want ErrWaitForEarlierBatch", err)
	}

	if err := f.svc.ProcessChunk(database.WithTenant(f.ctx, f.org), batchA.ID, 0); err != nil {
		t.Fatalf("batch 1: %v", err)
	}
	if err := f.svc.ProcessChunk(database.WithTenant(f.ctx, f.org), batchB.ID, 0); err != nil {
		t.Fatalf("batch 2 after batch 1: %v", err)
	}
	for name, batch := range map[string]erp.SyncBatch{"A": f.batchBySyncID(acceptedA.SyncID), "B": f.batchBySyncID(acceptedB.SyncID)} {
		if batch.State != "completed" {
			t.Errorf("batch %s state = %q", name, batch.State)
		}
	}
}

// One accepted batch larger than one chunk splits into ceil(n/2500) chunks and
// finishes only after every chunk is done.
func TestERPSyncFlow_SameBatchTwoChunks(t *testing.T) {
	f := newFlowFixture(t, matchflow.StateAccepted)
	items := make([]map[string]any, 0, 2501)
	for i := 0; i < 2501; i++ {
		items = append(items, erpItem(fmt.Sprintf("TWO%05d", i), fmt.Sprintf("%d.25", 10+i%50), 1))
	}
	accepted, batch := f.acceptOnly(erpBatchBody(items...), "")
	if batch.ChunkCount != 2 {
		t.Fatalf("chunk_count = %d, want 2", batch.ChunkCount)
	}
	if len(f.enqueuer.jobs) != 2 {
		t.Fatalf("enqueued jobs = %d, want 2", len(f.enqueuer.jobs))
	}
	if got := f.enqueuer.jobs[0]; got.BatchID != batch.ID || got.OrganizationID != f.org || got.ChunkNo != 0 {
		t.Errorf("first job = %+v", got)
	}
	if got := f.enqueuer.jobs[1]; got.ChunkNo != 1 {
		t.Errorf("second job chunk = %d, want 1", got.ChunkNo)
	}

	if err := f.svc.ProcessChunk(database.WithTenant(f.ctx, f.org), batch.ID, 0); err != nil {
		t.Fatal(err)
	}
	mid := f.batchBySyncID(accepted.SyncID)
	if mid.State != "processing" {
		t.Errorf("state after chunk 0 = %q, want processing", mid.State)
	}
	if mid.ChunksDone != 1 {
		t.Errorf("chunks_done after chunk 0 = %d, want 1", mid.ChunksDone)
	}
	if err := f.svc.ProcessChunk(database.WithTenant(f.ctx, f.org), batch.ID, 1); err != nil {
		t.Fatal(err)
	}
	done := f.batchBySyncID(accepted.SyncID)
	if done.State != "completed" || done.ChunksDone != 2 {
		t.Errorf("final state = %q chunks_done = %d, want completed/2", done.State, done.ChunksDone)
	}
	if done.ItemCount != 2501 {
		t.Errorf("item_count = %d, want 2501", done.ItemCount)
	}
}
