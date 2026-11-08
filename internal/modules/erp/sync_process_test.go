package erp

import (
	"context"
	"io"
	"log/slog"
	"strconv"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// The chunk-processing unit tests run the real ProcessChunk over the in-memory
// fake: intake stores the rows, process resolves tiers, calls the matcher once,
// writes once, and finishes the chunk. Everything the security rules name
// (tenant from context, no key re-read, retired variants untouched) is proven
// here against the fakes.

type processFixture struct {
	t         *testing.T
	repo      *fakeRepo
	writer    *fakeVariantWriter
	stock     *fakeStockWriter
	matcher   *fakeMatcher
	memory    *fakeMatchMemory
	enqueuer  *fakeEnqueuer
	svc       *Service
	org       int64
	tenantCtx context.Context
}

func newProcessFixture(t *testing.T, matcherState matchflow.State) *processFixture {
	repo := newFakeRepo()
	writer := &fakeVariantWriter{}
	stock := &fakeStockWriter{}
	matcher := &fakeMatcher{state: matcherState, productID: ptrInt64(777)}
	memory := &fakeMatchMemory{}
	enqueuer := &fakeEnqueuer{}
	svc := NewService(repo, Ports{
		VariantWriter: writer,
		StockWriter:   stock,
		Matcher:       matcher,
		MatchMemory:   memory,
		Enqueuer:      enqueuer,
	}, Config{Pepper: []byte("process-pepper-32-bytes-long!!"), Tag: "test", DefaultRate: 600, MaxBatchItems: 50000, Provider: "orgasoft"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	f := &processFixture{
		t: t, repo: repo, writer: writer, stock: stock, matcher: matcher,
		memory: memory, enqueuer: enqueuer, svc: svc, org: 7,
		tenantCtx: database.WithTenant(testCtx, 7),
	}
	// The S1 fake defaults the org to an approved vendor already.
	return f
}

func (f *processFixture) accept(items ...map[string]any) SyncBatch {
	f.t.Helper()
	body := intakeBatch(items...)
	p := Principal{OrganizationID: f.org, IntegrationID: 3, KeyID: 1, WarehouseID: 40, BranchID: 41, MaxBatchItems: 50000, Scopes: []string{"erp.products.sync"}, OrgType: "vendor", RatePerMinute: 600, Provider: "orgasoft"}
	accepted, err := f.svc.AcceptBatch(testCtx, p, body, "")
	if err != nil {
		f.t.Fatal(err)
	}
	return mustBatch(f.t, f.repo, accepted.SyncID)
}

func (f *processFixture) item(code string) SyncItemRow {
	f.t.Helper()
	items, err := f.repo.ListSyncItems(testCtx, f.org, f.lastBatchID(), 0, "", 1000)
	if err != nil {
		f.t.Fatal(err)
	}
	for _, row := range items {
		if row.ExternalCode == code {
			return row
		}
	}
	f.t.Fatalf("item %s not found", code)
	return SyncItemRow{}
}

func (f *processFixture) lastBatchID() int64 {
	f.t.Helper()
	var max int64
	for id := range f.repo.batches {
		if id > max {
			max = id
		}
	}
	return max
}

func TestProcessWaitForEarlierBatch(t *testing.T) {
	f := newProcessFixture(t, matchflow.StateAccepted)
	batch := f.accept(intakeItem("W1", "1.00"))
	f.repo.waitOnBatch = batch.ID
	err := f.svc.ProcessChunk(f.tenantCtx, batch.ID, 0)
	if err == nil {
		t.Fatal("expected ErrWaitForEarlierBatch")
	}
	if err != ErrWaitForEarlierBatch {
		t.Fatalf("err = %v", err)
	}
	if batch, _ := f.repo.GetBatchByID(testCtx, f.org, batch.ID); batch.State != "queued" {
		t.Errorf("state = %q, want still queued", batch.State)
	}
}

func TestProcessUnchangedByHashSkipsWriters(t *testing.T) {
	f := newProcessFixture(t, matchflow.StateAccepted)
	first := f.accept(intakeItem("U1", "1.00"))
	if err := f.svc.ProcessChunk(f.tenantCtx, first.ID, 0); err != nil {
		t.Fatal(err)
	}
	writesAfterFirst := len(f.writer.written)

	// The same item again under a fresh batch: the link's last_hash matches, so
	// the item is unchanged and no writer runs.
	second := f.accept(intakeItem("U1", "1.00"))
	if err := f.svc.ProcessChunk(f.tenantCtx, second.ID, 0); err != nil {
		t.Fatal(err)
	}
	if len(f.writer.written) != writesAfterFirst {
		t.Errorf("writer calls after unchanged = %d, want %d", len(f.writer.written), writesAfterFirst)
	}
	if len(f.stock.written) != writesAfterFirst/1 { // one stock write per write call
		t.Errorf("stock writes = %d", len(f.stock.written))
	}
	if rows := f.item("U1"); rows.State != "unchanged" {
		t.Errorf("second-run state = %q, want unchanged", rows.State)
	}
}

func TestProcessSKUTierLinksAndNeverCallsMatcher(t *testing.T) {
	f := newProcessFixture(t, matchflow.StateAccepted)
	f.repo.skuIndex["SKU9"] = []int64{4242}
	batch := f.accept(intakeItem("SKU9", "3.00"))
	if err := f.svc.ProcessChunk(f.tenantCtx, batch.ID, 0); err != nil {
		t.Fatal(err)
	}
	if f.matcher.requests != 0 {
		t.Errorf("matcher calls = %d, want 0 (tier hit)", f.matcher.requests)
	}
	row := f.item("SKU9")
	if row.State != "created" || row.VariantID == nil || *row.VariantID != 4242 {
		t.Fatalf("row = %+v", row)
	}
	// The tier's link write used sku_match.
	if len(f.repo.linkWrites) == 0 || f.repo.linkWrites[len(f.repo.linkWrites)-1].LinkSource != "sku_match" {
		t.Errorf("link source = %+v", f.repo.linkWrites)
	}
	if len(f.memory.kinds) != 0 {
		t.Errorf("tiers must never Remember, got %v", f.memory.kinds)
	}
}

func TestProcessBarcodeTier(t *testing.T) {
	f := newProcessFixture(t, matchflow.StateAccepted)
	f.repo.barcodeIndex["6220000000001"] = []int64{5151}
	batch := f.accept(map[string]any{
		"externalProductCode": "BAR1", "arName": "باركود", "publicPrice": "4.00", "stock": 2,
		"barcode": "6220000000001",
	})
	if err := f.svc.ProcessChunk(f.tenantCtx, batch.ID, 0); err != nil {
		t.Fatal(err)
	}
	if f.matcher.requests != 0 {
		t.Errorf("matcher calls = %d, want 0", f.matcher.requests)
	}
	if row := f.item("BAR1"); row.State != "created" || row.VariantID == nil || *row.VariantID != 5151 {
		t.Fatalf("row = %+v", row)
	}
	if last := f.repo.linkWrites[len(f.repo.linkWrites)-1]; last.LinkSource != "barcode_match" {
		t.Errorf("link source = %q", last.LinkSource)
	}
}

func TestProcessBarcodeConflictGoesToReview(t *testing.T) {
	f := newProcessFixture(t, matchflow.StateAccepted)
	f.repo.barcodeIndex["6220000000002"] = []int64{11, 22}
	batch := f.accept(map[string]any{
		"externalProductCode": "CONF1", "arName": "تعارض", "publicPrice": "4.00", "stock": 2,
		"barcode": "6220000000002",
	})
	if err := f.svc.ProcessChunk(f.tenantCtx, batch.ID, 0); err != nil {
		t.Fatal(err)
	}
	row := f.item("CONF1")
	if row.State != "needs_review" || row.Code == nil || *row.Code != "ERP_ITEM_BARCODE_CONFLICT" {
		t.Fatalf("row = %+v", row)
	}
	if len(f.repo.reviewRows) != 1 || f.repo.reviewRows[0].ReasonCode != "ERP_ITEM_BARCODE_CONFLICT" {
		t.Errorf("review rows = %+v", f.repo.reviewRows)
	}
}

func TestProcessMatcherScopeAndConsumerRef(t *testing.T) {
	f := newProcessFixture(t, matchflow.StateAccepted)
	batch := f.accept(intakeItem("MATCH1", "2.00"), intakeItem("MATCH2", "2.00"))
	if err := f.svc.ProcessChunk(f.tenantCtx, batch.ID, 0); err != nil {
		t.Fatal(err)
	}
	if f.matcher.requests != 1 {
		t.Fatalf("matcher calls = %d, want exactly 1 per chunk", f.matcher.requests)
	}
	req := f.matcher.lastRequest
	if req.Scope.SourceNamespace != "supplier:7" || req.Scope.Workflow != productmatch.WorkflowVendor || req.Scope.EntryPoint != "erp_sync" {
		t.Errorf("scope = %+v", req.Scope)
	}
	if req.Scope.EffectiveOrg != f.org {
		t.Errorf("EffectiveOrg = %d", req.Scope.EffectiveOrg)
	}
	if req.ConsumerRef != "erp.sync:"+itoa(batch.ID)+":0" {
		t.Errorf("ConsumerRef = %q", req.ConsumerRef)
	}
	if len(req.Rows) != 2 {
		t.Errorf("rows = %d, want both remaining rows in one call", len(req.Rows))
	}
	if len(f.memory.kinds) != 2 || f.memory.kinds[0] != "supplier_code" {
		t.Errorf("Remember = %v", f.memory.kinds)
	}
	if f.memory.namespaces[0] != "supplier:7" {
		t.Errorf("Remember namespace = %q", f.memory.namespaces[0])
	}
}

func itoa(v int64) string { return strconv.FormatInt(v, 10) }

func TestProcessMatcherNeedsReviewWritesReview(t *testing.T) {
	f := newProcessFixture(t, matchflow.StateNeedsReview)
	batch := f.accept(intakeItem("REV1", "2.00"))
	if err := f.svc.ProcessChunk(f.tenantCtx, batch.ID, 0); err != nil {
		t.Fatal(err)
	}
	row := f.item("REV1")
	if row.State != "needs_review" || row.Code == nil || *row.Code != "ERP_ITEM_MATCH_REVIEW" {
		t.Fatalf("row = %+v", row)
	}
	if len(f.repo.reviewRows) != 1 || f.repo.reviewRows[0].ReasonCode != "ERP_ITEM_MATCH_REVIEW" {
		t.Fatalf("reviews = %+v", f.repo.reviewRows)
	}
}

func TestProcessSoftDeletedLinkedVariantGoesToReview(t *testing.T) {
	f := newProcessFixture(t, matchflow.StateAccepted)
	batch := f.accept(intakeItem("DEAD1", "2.00"))
	// The link exists but its variant was soft-deleted.
	code := "DEAD1"
	f.repo.links[linkKey(3, code)] = &ProductLink{
		OrganizationID: f.org, IntegrationID: 3, ExternalCode: code,
		VariantID: 999, LinkSource: "sku_match", VariantDeleted: true,
	}
	if err := f.svc.ProcessChunk(f.tenantCtx, batch.ID, 0); err != nil {
		t.Fatal(err)
	}
	row := f.item("DEAD1")
	if row.State != "needs_review" || row.Code == nil || *row.Code != "ERP_ITEM_DELETED_LINK" {
		t.Fatalf("row = %+v", row)
	}
	if f.writer.written != nil && len(f.writer.written) != 0 {
		t.Errorf("deleted-link items must not be written, got %+v", f.writer.written)
	}
}

func TestProcessOneWriteCallPerChunk(t *testing.T) {
	f := newProcessFixture(t, matchflow.StateAccepted)
	batch := f.accept(intakeItem("W10", "1.00"), intakeItem("W11", "1.00"), intakeItem("W12", "1.00"))
	if err := f.svc.ProcessChunk(f.tenantCtx, batch.ID, 0); err != nil {
		t.Fatal(err)
	}
	if len(f.writer.written) != 3 {
		t.Fatalf("variant rows = %d, want 3", len(f.writer.written))
	}
	for _, row := range f.writer.written {
		if row.BranchID != 41 {
			t.Errorf("BranchID = %d, want the key's branch", row.BranchID)
		}
	}
	if len(f.stock.written) != 3 {
		t.Fatalf("stock rows = %d, want 3", len(f.stock.written))
	}
	for _, row := range f.stock.written {
		if !row.HasQuantity || row.WarehouseID != 40 {
			t.Errorf("stock row = %+v, want HasQuantity with the key's warehouse", row)
		}
	}
}

func TestProcessQuotaWrite(t *testing.T) {
	f := newProcessFixture(t, matchflow.StateAccepted)
	yes := true
	batch := f.accept(map[string]any{
		"externalProductCode": "QUO1", "arName": "حصص", "publicPrice": "3.00", "stock": 5,
		"isQuota": true, "quotaQuantity": 4,
	})
	_ = yes
	if err := f.svc.ProcessChunk(f.tenantCtx, batch.ID, 0); err != nil {
		t.Fatal(err)
	}
	if len(f.writer.quotaCalls) != 1 {
		t.Fatalf("quota calls = %d, want 1", len(f.writer.quotaCalls))
	}
	if len(f.writer.quotaCalls[0]) != 1 {
		t.Errorf("quota map = %+v, want one entry", f.writer.quotaCalls[0])
	}
}

func TestProcessWriteFailureIsolatesTheRow(t *testing.T) {
	f := newProcessFixture(t, matchflow.StateAccepted)
	f.writer.failures = map[int]string{0: " boom"}
	batch := f.accept(intakeItem("WF1", "1.00"), intakeItem("WF2", "1.00"))
	if err := f.svc.ProcessChunk(f.tenantCtx, batch.ID, 0); err != nil {
		t.Fatal(err)
	}
	first := f.item("WF1")
	if first.State != "failed" || first.Code == nil || *first.Code != "ERP_ITEM_WRITE_FAILED" {
		t.Fatalf("WF1 = %+v", first)
	}
	second := f.item("WF2")
	if second.State != "created" {
		t.Fatalf("WF2 = %+v, want created (rest continue)", second)
	}
}

func TestProcessLastChunkCompletesAndCountersFold(t *testing.T) {
	f := newProcessFixture(t, matchflow.StateAccepted)
	batch := f.accept(intakeItem("C1", "1.00"), intakeItem("BAD-C", "nope"), intakeItem("C2", "1.00"))
	if err := f.svc.ProcessChunk(f.tenantCtx, batch.ID, 0); err != nil {
		t.Fatal(err)
	}
	final, err := f.repo.GetBatchByID(testCtx, f.org, batch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if final.State != "completed" {
		t.Errorf("state = %q", final.State)
	}
	if final.CreatedCount != 2 || final.FailedCount != 1 {
		t.Errorf("counters created=%d failed=%d", final.CreatedCount, final.FailedCount)
	}
	if final.FinishedAt == nil {
		t.Error("finished_at missing")
	}
}

func TestProcessRetriedChunkDoesNotRewriteFinishedRows(t *testing.T) {
	f := newProcessFixture(t, matchflow.StateAccepted)
	batch := f.accept(intakeItem("RT1", "1.00"), intakeItem("RT2", "1.00"))
	if err := f.svc.ProcessChunk(f.tenantCtx, batch.ID, 0); err != nil {
		t.Fatal(err)
	}
	writesAfter := len(f.writer.written)
	// A second worker picks the same job up: pending is empty, the batch is
	// already completed, and nothing is rewritten or double-counted.
	if err := f.svc.ProcessChunk(f.tenantCtx, batch.ID, 0); err != nil {
		t.Fatal(err)
	}
	if len(f.writer.written) != writesAfter {
		t.Errorf("rewrite happened: %d -> %d", writesAfter, len(f.writer.written))
	}
	final, _ := f.repo.GetBatchByID(testCtx, f.org, batch.ID)
	if final.CreatedCount != 2 {
		t.Errorf("counters re-folded: %+v", final)
	}
}

func TestProcessIgnoresRevokedKeys(t *testing.T) {
	f := newProcessFixture(t, matchflow.StateAccepted)
	batch := f.accept(intakeItem("REVOKED-ITEM", "1.00"))
	// The key is revoked right after acceptance; the chunk still completes
	// because its context comes from the batch row, never from the key.
	revoked := NewKey{APIKey: erpKeyFor(1, f.org), KeyHash: "x"}
	revoked.Status = "revoked"
	f.repo.keys[1] = revoked
	if err := f.svc.ProcessChunk(f.tenantCtx, batch.ID, 0); err != nil {
		t.Fatalf("process after revoke: %v", err)
	}
}

func erpKeyFor(id, org int64) APIKey {
	return APIKey{ID: id, OrganizationID: org, Status: "active", Name: "k", KeyPrefix: "abcdefgh"}
}

// TestProcessIntakeFailedChunkStillCompletes: a chunk whose rows all failed at
// intake has no pending items, yet its FinishChunk must still fold the batch.
func TestProcessIntakeFailedChunkStillCompletes(t *testing.T) {
	f := newProcessFixture(t, matchflow.StateAccepted)
	batch := f.accept(
		map[string]any{"externalProductCode": "BADX", "arName": "سيء", "publicPrice": "1.234", "stock": 1},
		map[string]any{"externalProductCode": "BADC", "arName": "سيء"},
	)
	if err := f.svc.ProcessChunk(f.tenantCtx, batch.ID, 0); err != nil {
		t.Fatal(err)
	}
	final, _ := f.repo.GetBatchByID(testCtx, f.org, batch.ID)
	if final.State != "completed" {
		t.Errorf("state = %q, want completed", final.State)
	}
	if final.FailedCount != 2 {
		t.Errorf("failed = %d, want 2", final.FailedCount)
	}
}
