package integration_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
)

// SC-002 at the worker level: one worker (stricter than production's three)
// accepts 50,000 items in under 5 s and processes the whole batch through 20
// chunks in under 7 minutes with every item in a terminal state.
func TestERPSyncFlow_Scale50000(t *testing.T) {
	f := newFlowFixture(t, matchflow.StateAccepted)
	items := make([]map[string]any, 0, 50000)
	for i := 0; i < 50000; i++ {
		items = append(items, map[string]any{
			"externalProductCode": fmt.Sprintf("%d", 900000+i),
			"arName":              "صنف رقم " + fmt.Sprint(900000+i),
			"publicPrice":         "10.50",
			"stock":               5,
			"isQuota":             false,
			"quotaQuantity":       0,
		})
	}
	body := erpBatchBody(items...)

	acceptStart := time.Now()
	accepted, batch := f.acceptOnly(body, "")
	acceptTook := time.Since(acceptStart)
	t.Logf("accept 50000 items took %s", acceptTook)
	if acceptTook >= 5*time.Second {
		t.Fatalf("accept took %s, SC-002 allows under 5 s", acceptTook)
	}

	if accepted.ItemCount != 50000 {
		t.Fatalf("item_count = %d, want 50000", accepted.ItemCount)
	}
	if batch.ChunkCount != 20 {
		t.Fatalf("chunk_count = %d, want 20", batch.ChunkCount)
	}
	if len(f.enqueuer.jobs) != 20 {
		t.Fatalf("enqueued chunk jobs = %d, want 20", len(f.enqueuer.jobs))
	}

	processStart := time.Now()
	for chunk := 0; chunk < batch.ChunkCount; chunk++ {
		if err := f.svc.ProcessChunk(database.WithTenant(f.ctx, f.org), batch.ID, chunk); err != nil {
			t.Fatalf("ProcessChunk %d: %v", chunk, err)
		}
	}
	processTook := time.Since(processStart)
	t.Logf("process 20 chunks took %s", processTook)
	if processTook >= 7*time.Minute {
		t.Fatalf("processing took %s, SC-002 allows under 7 minutes", processTook)
	}

	final := f.batchBySyncID(accepted.SyncID)
	if final.State != "completed" {
		t.Errorf("final state = %q, want completed", final.State)
	}
	if final.ChunksDone != 20 {
		t.Errorf("chunks_done = %d, want 20", final.ChunksDone)
	}
	total := final.CreatedCount + final.NeedsReviewCount + final.FailedCount
	if total != 50000 {
		t.Errorf("created %d + needs_review %d + failed %d = %d, want 50000",
			final.CreatedCount, final.NeedsReviewCount, final.FailedCount, total)
	}
	items2, err := f.repo.ListSyncItems(f.ctx, f.org, batch.ID, 0, "pending", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(items2) != 0 {
		t.Errorf("pending items remain: %d", len(items2))
	}
	items3, err := f.repo.ListSyncItems(f.ctx, f.org, batch.ID, 0, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(items3) == 0 {
		t.Error("no items at all were written")
	}
}
