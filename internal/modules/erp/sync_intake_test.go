package erp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp/provider"
)

var testCtx = context.Background()

// The intake unit tests run against the in-memory fake and the fake ports:
// invalid rows fail at intake with their code, duplicates resolve last-wins,
// oversized batches are refused, chunks are counted and enqueued in the batch
// transaction, and the Idempotency-Key replays the first accepted result.

func intakeResult(err error) *Error {
	if err == nil {
		return nil
	}
	e, _ := err.(*Error)
	return e
}

func intakeItem(code, price string) map[string]any {
	return map[string]any{
		"externalProductCode": code,
		"arName":              "صنف " + code,
		"publicPrice":         price,
		"stock":               5,
		"isQuota":             false,
		"quotaQuantity":       0,
	}
}

func intakeBatch(items ...map[string]any) []byte {
	var buf []byte
	buf = append(buf, `{"items":[`...)
	for i, item := range items {
		if i > 0 {
			buf = append(buf, ',')
		}
		data, _ := json.Marshal(item)
		buf = append(buf, data...)
	}
	buf = append(buf, `]}`...)
	return buf
}

func intakeService(pMax int, scopes []string) (*Service, *fakeRepo, *fakeEnqueuer, Principal) {
	repo := newFakeRepo()
	enqueuer := &fakeEnqueuer{}
	matcher := &fakeMatcher{state: "accepted"}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(repo, Ports{
		VariantWriter: &fakeVariantWriter{},
		StockWriter:   &fakeStockWriter{},
		Matcher:       matcher,
		MatchMemory:   &fakeMatchMemory{},
		Enqueuer:      enqueuer,
	}, Config{Pepper: []byte("intake-pepper-32-bytes-long!!!"), Tag: "test", DefaultRate: 600, MaxBatchItems: pMax, Provider: provider.DefaultProvider}, log)
	p := Principal{OrganizationID: 7, IntegrationID: 3, KeyID: 1, WarehouseID: 40, BranchID: 41, MaxBatchItems: pMax, Scopes: scopes, OrgType: "vendor", RatePerMinute: 600, Provider: provider.DefaultProvider}
	return svc, repo, enqueuer, p
}

func TestIntakeInvalidItemFailsAtIntake(t *testing.T) {
	svc, repo, enqueuer, p := intakeService(50000, []string{"erp.products.sync"})
	body := intakeBatch(
		intakeItem("OK01", "10.00"),
		map[string]any{"externalProductCode": "BAD01", "arName": "سيء", "publicPrice": "12.345", "stock": 5},
	)
	accepted, err := svc.AcceptBatch(testCtx, p, body, "")
	if err != nil {
		t.Fatal(err)
	}
	if accepted.RejectedAtIntake != 1 {
		t.Errorf("RejectedAtIntake = %d, want 1", accepted.RejectedAtIntake)
	}
	if accepted.ItemCount != 2 {
		t.Errorf("ItemCount = %d, want 2 (invalid rows are stored)", accepted.ItemCount)
	}
	items, _ := repo.ListSyncItems(testCtx, 7, batchIDFor(t, repo, accepted.SyncID), 0, "failed", 10)
	if len(items) != 1 || items[0].ExternalCode != "BAD01" || items[0].Code == nil || *items[0].Code != "ERP_ITEM_PRICE_PRECISION" {
		t.Fatalf("failed rows = %+v", items)
	}
	if len(enqueuer.jobs) != 1 {
		t.Errorf("jobs = %d, want 1 (the single chunk)", len(enqueuer.jobs))
	}
}

func TestIntakeDuplicateCodesLastWins(t *testing.T) {
	svc, repo, _, p := intakeService(50000, []string{"erp.products.sync"})
	body := intakeBatch(intakeItem("DUP1", "10.00"), intakeItem("DUP1", "12.00"))
	accepted, err := svc.AcceptBatch(testCtx, p, body, "")
	if err != nil {
		t.Fatal(err)
	}
	batchID := batchIDFor(t, repo, accepted.SyncID)
	items, _ := repo.ListSyncItems(testCtx, 7, batchID, 0, "", 10)
	if len(items) != 2 {
		t.Fatalf("rows = %d, want 2", len(items))
	}
	first, second := items[0], items[1]
	if first.Position > second.Position {
		first, second = second, first
	}
	if first.State != "unchanged" || first.Code == nil || *first.Code != "ERP_ITEM_SUPERSEDED" {
		t.Errorf("earlier row = %+v, want unchanged ERP_ITEM_SUPERSEDED", first)
	}
	if second.State != "pending" {
		t.Errorf("last row = %+v, want pending", second)
	}
}

func TestIntakeEmptyBatch(t *testing.T) {
	svc, _, _, p := intakeService(50000, []string{"erp.products.sync"})
	_, err := svc.AcceptBatch(testCtx, p, intakeBatch(), "")
	e := intakeResult(err)
	if e == nil || e.Code != "ERP_BATCH_EMPTY" {
		t.Fatalf("err = %v, want ERP_BATCH_EMPTY", err)
	}
}

func TestIntakeTooLarge(t *testing.T) {
	svc, _, _, p := intakeService(2, []string{"erp.products.sync"})
	body := intakeBatch(intakeItem("A1", "1.00"), intakeItem("A2", "1.00"), intakeItem("A3", "1.00"))
	_, err := svc.AcceptBatch(testCtx, p, body, "")
	e := intakeResult(err)
	if e == nil || e.Code != "ERP_BATCH_TOO_LARGE" {
		t.Fatalf("err = %v, want ERP_BATCH_TOO_LARGE", err)
	}
	if e.Details["limit"] != 2 {
		t.Errorf("details limit = %v", e.Details["limit"])
	}
	if received, _ := e.Details["received"].(int); received != 3 {
		t.Errorf("details received = %v", e.Details["received"])
	}
}

func TestIntakeChunkCountingAndEnqueue(t *testing.T) {
	svc, repo, enqueuer, p := intakeService(50000, []string{"erp.products.sync"})
	items := make([]map[string]any, 0, 2500)
	for i := 0; i < 2500; i++ {
		items = append(items, intakeItem(fmt.Sprintf("C%05d", i), "1.00"))
	}
	accepted, err := svc.AcceptBatch(testCtx, p, intakeBatch(items...), "")
	if err != nil {
		t.Fatal(err)
	}
	if batch := mustBatch(t, repo, accepted.SyncID); batch.ChunkCount != 1 {
		t.Errorf("chunk_count = %d, want 1", batch.ChunkCount)
	}
	if len(enqueuer.jobs) != 1 {
		t.Fatalf("jobs = %d, want 1", len(enqueuer.jobs))
	}

	items = append(items, intakeItem("C25000", "1.00"))
	accepted2, err := svc.AcceptBatch(testCtx, p, intakeBatch(items...), "")
	if err != nil {
		t.Fatal(err)
	}
	if batch := mustBatch(t, repo, accepted2.SyncID); batch.ChunkCount != 2 {
		t.Errorf("chunk_count = %d, want 2", batch.ChunkCount)
	}
	if len(enqueuer.jobs) != 3 { // 1 from batch 1 + 2 from batch 2
		t.Errorf("jobs = %d, want 3", len(enqueuer.jobs))
	}
}

func TestIntakeIdempotencyReplay(t *testing.T) {
	svc, _, enqueuer, p := intakeService(50000, []string{"erp.products.sync"})
	body := intakeBatch(intakeItem("IDEM1", "3.00"))
	first, err := svc.AcceptBatch(testCtx, p, body, "idem-1")
	if err != nil {
		t.Fatal(err)
	}
	jobsAfterFirst := len(enqueuer.jobs)

	replay, err := svc.AcceptBatch(testCtx, p, body, "idem-1")
	if err != nil {
		t.Fatal(err)
	}
	if replay.SyncID != first.SyncID || replay.ItemCount != first.ItemCount || replay.State != first.State {
		t.Errorf("replay = %+v, want %+v", replay, first)
	}
	if len(enqueuer.jobs) != jobsAfterFirst {
		t.Errorf("replay enqueued jobs: %d -> %d, want unchanged", jobsAfterFirst, len(enqueuer.jobs))
	}

	_, err = svc.AcceptBatch(testCtx, p, intakeBatch(intakeItem("IDEM1", "9.00")), "idem-1")
	e := intakeResult(err)
	if e == nil || e.Code != "ERP_IDEMPOTENCY_CONFLICT" {
		t.Fatalf("err = %v, want ERP_IDEMPOTENCY_CONFLICT", err)
	}
}

func TestIntakeScopeAndCapability(t *testing.T) {
	svc, _, _, p := intakeService(50000, []string{"erp.orders.read"})
	_, err := svc.AcceptBatch(testCtx, p, intakeBatch(intakeItem("S1", "3.00")), "")
	if e := intakeResult(err); e == nil || e.Code != "ERP_SCOPE_MISSING" {
		t.Fatalf("err = %v, want ERP_SCOPE_MISSING", err)
	}

	_, _, _, customer := intakeService(50000, []string{"erp.products.sync"})
	customer.OrgType = "customer"
	_, err = svc.AcceptBatch(testCtx, customer, intakeBatch(intakeItem("S2", "3.00")), "")
	if e := intakeResult(err); e == nil || e.Code != "ERP_CAPABILITY_REQUIRED" {
		t.Fatalf("err = %v, want ERP_CAPABILITY_REQUIRED", err)
	}
}

func TestIntakeBadJSON(t *testing.T) {
	svc, _, _, p := intakeService(50000, []string{"erp.products.sync"})
	_, err := svc.AcceptBatch(testCtx, p, []byte(`{"items": "not an array"}`), "")
	e := intakeResult(err)
	if e == nil || e.Code != "ERP_BAD_JSON" {
		t.Fatalf("err = %v, want ERP_BAD_JSON", err)
	}
}

func mustBatch(t *testing.T, repo Repository, syncID string) SyncBatch {
	t.Helper()
	batch, err := repo.GetBatchByPublicID(testCtx, 7, syncID)
	if err != nil {
		t.Fatal(err)
	}
	return batch
}

func batchIDFor(t *testing.T, repo Repository, syncID string) int64 {
	t.Helper()
	batch := mustBatch(t, repo, syncID)
	return batch.ID
}
