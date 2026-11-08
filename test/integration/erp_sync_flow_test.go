package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/moqasem061-dot/dawa24/internal/composition/erpports"
	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
	catalogpg "github.com/moqasem061-dot/dawa24/internal/modules/catalog/postgres"
	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	erppg "github.com/moqasem061-dot/dawa24/internal/modules/erp/postgres"
	"github.com/moqasem061-dot/dawa24/internal/modules/erp/provider"
	"github.com/moqasem061-dot/dawa24/internal/modules/inventory"
	inventorypg "github.com/moqasem061-dot/dawa24/internal/modules/inventory/postgres"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/platform/queue"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// The ERP product-sync flow, driven exactly as the worker will drive it: real
// Postgres, the real catalog and inventory adapters from
// internal/composition/erpports, and a deterministic matcher stub. No River —
// ProcessChunk is called directly (plan S2 step 14).

// flowMatcher answers Prepare for every remaining row, always with the same
// state and the seeded product id.
type flowMatcher struct {
	state       matchflow.State
	productID   int64
	requests    int
	lastRequest productmatch.PrepareRequest
}

func (m *flowMatcher) Prepare(_ context.Context, req productmatch.PrepareRequest) (productmatch.PrepareResult, error) {
	m.requests++
	m.lastRequest = req
	pid := m.productID
	decisions := make([]matchflow.RowDecision, len(req.Rows))
	for i := range decisions {
		decisions[i] = matchflow.RowDecision{State: m.state, ProductID: &pid}
	}
	return productmatch.PrepareResult{Decisions: decisions}, nil
}

// flowMemory records Remember calls.
type flowMemory struct {
	namespaces []string
	kinds      []string
}

func (m *flowMemory) Remember(_ context.Context, sc productmatch.Scope, k erp.KnowledgeWrite) error {
	m.namespaces = append(m.namespaces, sc.SourceNamespace)
	m.kinds = append(m.kinds, k.Kind)
	return nil
}

// flowEnqueuer records the chunk jobs acceptance inserts.
type flowEnqueuer struct {
	jobs []queue.ERPSyncChunkArgs
}

func (f *flowEnqueuer) EnqueueTx(_ context.Context, _ pgx.Tx, args river.JobArgs, _ *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	if chunk, ok := args.(queue.ERPSyncChunkArgs); ok {
		f.jobs = append(f.jobs, chunk)
	}
	return &rivertype.JobInsertResult{}, nil
}

type syncFixture struct {
	t           *testing.T
	ctx         context.Context
	log         *slog.Logger
	db          *database.DB
	org         int64
	keyID       int64
	integration int64
	branch      int64
	warehouse   int64
	seedProduct int64
	principal   erp.Principal
	svc         *erp.Service
	repo        *erppg.Repository
	matcher     *flowMatcher
	memory      *flowMemory
	enqueuer    *flowEnqueuer
	nonce       int64
}

func newFlowFixture(t *testing.T, matcherState matchflow.State) *syncFixture {
	t.Helper()
	db := getTestDB(t)
	if db == nil {
		t.Fatal("flow test requires a database")
	}
	t.Cleanup(db.Close)
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	f := &syncFixture{t: t, ctx: ctx, log: log, db: db, nonce: time.Now().UnixNano() / 1000}
	f.org = f.nonce + 40000

	err := db.InTx(database.AsSystem(ctx), func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO org.organizations (id, name, type, status) VALUES ($1, '{"ar":"مورد","en":"Sync supplier"}', 'vendor', 'approved')`, f.org); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO org.branches (organization_id, name) VALUES ($1, '{"en":"Flow branch"}') RETURNING id`, f.org).Scan(&f.branch); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO inventory.warehouses (organization_id, name) VALUES ($1, 'Flow warehouse') RETURNING id`, f.org).Scan(&f.warehouse); err != nil {
			return err
		}
		integration, _, err := seedERPKey(ctx, tx, f.org, f.warehouse, f.branch, []string{"erp.products.sync"})
		if err != nil {
			return err
		}
		f.integration = integration
		if err := tx.QueryRow(ctx, `INSERT INTO catalog.products (organization_id, name) VALUES ($1, '{"ar":"منتج معروف","en":"Known product"}') RETURNING id`, f.org).Scan(&f.seedProduct); err != nil {
			return err
		}
		var knownVariant, quietVariant int64
		if err := tx.QueryRow(ctx, `INSERT INTO catalog.product_variants (organization_id, product_id, name, sku, barcode, price)
			VALUES ($1, $2, '{"ar":"معروف","en":"Known"}', '4001', '6221790408214', 999.99) RETURNING id`, f.org, f.seedProduct).Scan(&knownVariant); err != nil {
			return err
		}
		// A variant the batch never mentions: the sync must not touch it.
		if err := tx.QueryRow(ctx, `INSERT INTO catalog.product_variants (organization_id, product_id, name, sku, price)
			VALUES ($1, $2, '{"ar":"هادئ","en":"Quiet"}', $3, 5.00) RETURNING id`, f.org, f.seedProduct, "quiet-"+strconv.FormatInt(f.org%100000, 10)).Scan(&quietVariant); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO erp.product_links (organization_id, integration_id, external_code, variant_id, product_id, link_source) VALUES ($1, $2, '4001', $3, $4, 'sku_match')`, f.org, integration, knownVariant, f.seedProduct)
		return err
	})
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	t.Cleanup(func() {
		_ = db.InTx(database.AsSystem(context.Background()), func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `DELETE FROM org.organizations WHERE id = $1`, f.org)
			return err
		})
	})

	f.matcher = &flowMatcher{state: matcherState, productID: f.seedProduct}
	f.memory = &flowMemory{}
	f.enqueuer = &flowEnqueuer{}
	catSvc := catalog.NewService(catalogpg.NewRepository(db), log)
	invSvc := inventory.NewService(inventorypg.NewRepository(db), log)
	f.repo = erppg.New(db)
	f.svc = erp.NewService(erppg.New(db), erp.Ports{
		VariantWriter: erpports.NewVariantWriter(catSvc),
		StockWriter:   erpports.NewStockWriter(invSvc),
		Matcher:       f.matcher,
		MatchMemory:   f.memory,
		Enqueuer:      f.enqueuer,
	}, erp.Config{Pepper: []byte("flow-erp-pepper-32-bytes-long!!!"), Tag: "test", DefaultRate: 600, MaxBatchItems: 50000, Provider: provider.DefaultProvider}, log)
	f.principal = erp.Principal{
		OrganizationID: f.org, IntegrationID: f.integration, KeyID: f.keyID,
		Provider: provider.DefaultProvider, Scopes: []string{"erp.products.sync"},
		WarehouseID: f.warehouse, BranchID: f.branch, MaxBatchItems: 50000, RatePerMinute: 600,
		OrgType: "vendor", KeyPrefix: "flowtest",
	}
	return f
}

// seedERPKey inserts the integration and one active key.
func seedERPKey(ctx context.Context, tx pgx.Tx, org, warehouse, branch int64, scopes []string) (int64, int64, error) {
	var integration int64
	if err := tx.QueryRow(ctx, `INSERT INTO erp.integrations (organization_id, provider) VALUES ($1, 'orgasoft') RETURNING id`, org).Scan(&integration); err != nil {
		return 0, 0, err
	}
	var keyID int64
	err := tx.QueryRow(ctx, `INSERT INTO erp.api_keys (organization_id, integration_id, name, key_prefix, key_hash, scopes, warehouse_id, branch_id)
		VALUES ($1, $2, 'flow', $3, '0000000000000000000000000000000000000000000000000000000000000000', $4, $5, $6) RETURNING id`,
		org, integration, fmt.Sprintf("fl%06x", uint32(org)&0xffffff), scopes, warehouse, branch).Scan(&keyID)
	return integration, keyID, err
}

// erpItem builds one camelCase wire item.
func erpItem(code, price string, stock int) map[string]any {
	return map[string]any{
		"externalProductCode": code,
		"arName":              "صنف " + code,
		"publicPrice":         price,
		"stock":               stock,
		"discount":            0,
		"tax":                 14,
		"isQuota":             false,
		"quotaQuantity":       0,
		"barcode":             "1234567890" + strconv.Itoa(len(code)%100),
	}
}

func erpBatchBody(items ...map[string]any) []byte {
	var buf bytes.Buffer
	buf.WriteString(`{"items":[`)
	for i, item := range items {
		if i > 0 {
			buf.WriteByte(',')
		}
		data, _ := json.Marshal(item)
		buf.Write(data)
	}
	buf.WriteString(`]}`)
	return buf.Bytes()
}

// acceptAndProcess accepts the batch and processes every chunk with the tenant
// context pinned to the organization.
func (f *syncFixture) acceptAndProcess(body []byte) (erp.Accepted, erp.SyncBatch) {
	f.t.Helper()
	accepted, err := f.svc.AcceptBatch(f.ctx, f.principal, body, "")
	if err != nil {
		f.t.Fatalf("AcceptBatch: %v", err)
	}
	batch := f.batchBySyncID(accepted.SyncID)
	for chunk := 0; chunk < batch.ChunkCount; chunk++ {
		if err := f.svc.ProcessChunk(database.WithTenant(f.ctx, f.org), batch.ID, chunk); err != nil {
			f.t.Fatalf("ProcessChunk %d: %v", chunk, err)
		}
	}
	return accepted, f.batchBySyncID(accepted.SyncID)
}

// acceptOnly accepts without processing (for order and repeat-control tests).
func (f *syncFixture) acceptOnly(body []byte, idemKey string) (erp.Accepted, erp.SyncBatch) {
	f.t.Helper()
	accepted, err := f.svc.AcceptBatch(f.ctx, f.principal, body, idemKey)
	if err != nil {
		f.t.Fatalf("AcceptBatch: %v", err)
	}
	return accepted, f.batchBySyncID(accepted.SyncID)
}

func (f *syncFixture) batchBySyncID(syncID string) erp.SyncBatch {
	f.t.Helper()
	batch, err := f.repo.GetBatchByPublicID(f.ctx, f.org, syncID)
	if err != nil {
		f.t.Fatalf("GetBatchByPublicID %q: %v", syncID, err)
	}
	return batch
}

func (f *syncFixture) itemsByCode(batchID int64) map[string][]erp.SyncItemRow {
	f.t.Helper()
	items, err := f.repo.ListSyncItems(f.ctx, f.org, batchID, 0, "", 10000)
	if err != nil {
		f.t.Fatalf("ListSyncItems: %v", err)
	}
	byCode := map[string][]erp.SyncItemRow{}
	for _, item := range items {
		byCode[item.ExternalCode] = append(byCode[item.ExternalCode], item)
	}
	return byCode
}

func (f *syncFixture) variantCount() int {
	f.t.Helper()
	var n int
	if err := f.db.InReadTx(database.AsSystem(f.ctx), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM catalog.product_variants WHERE organization_id = $1 AND deleted_at IS NULL`, f.org).Scan(&n)
	}); err != nil {
		f.t.Fatal(err)
	}
	return n
}

func (f *syncFixture) linkCount() int {
	f.t.Helper()
	var n int
	if err := f.db.InReadTx(database.AsSystem(f.ctx), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM erp.product_links WHERE organization_id = $1`, f.org).Scan(&n)
	}); err != nil {
		f.t.Fatal(err)
	}
	return n
}

func (f *syncFixture) stockQuantity(variantID int64) (int, bool) {
	f.t.Helper()
	var qty int
	err := f.db.InReadTx(database.AsSystem(f.ctx), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT quantity FROM inventory.stocks WHERE warehouse_id = $1 AND product_variant_id = $2`, f.warehouse, variantID).Scan(&qty)
	})
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			return 0, false
		}
		f.t.Fatal(err)
	}
	return qty, true
}

func (f *syncFixture) linkedVariantID(code string) int64 {
	f.t.Helper()
	var variant int64
	err := f.db.InReadTx(database.AsSystem(f.ctx), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT l.variant_id FROM erp.product_links l WHERE l.organization_id = $1 AND l.external_code = $2`, f.org, code).Scan(&variant)
	})
	if err != nil {
		f.t.Fatalf("link for %s: %v", code, err)
	}
	return variant
}

// TestERPSyncFlow_Basic is the slice's independent test: new, known, invalid;
// stock 0; a duplicate code inside one batch; the absent product untouched.
func TestERPSyncFlow_Basic(t *testing.T) {
	f := newFlowFixture(t, matchflow.StateAccepted)
	beforeVariants := f.variantCount()
	body := erpBatchBody(
		erpItem("NEW100", "61.25", 4), // new: matcher accepts
		erpItem("4001", "55.00", 7),   // known: linked, hash differs -> updated
		erpItem("BAD01", "12.345", 1), // invalid: rejected at intake
		erpItem("ZERO01", "30.00", 0), // stock 0 is a real quantity
		erpItem("DUPX", "10.00", 1),   // superseded by the row below
		erpItem("DUPX", "10.50", 2),   // last wins
	)
	accepted, batch := f.acceptAndProcess(body)

	if accepted.ItemCount != 6 {
		t.Errorf("ItemCount = %d, want 6", accepted.ItemCount)
	}
	if accepted.RejectedAtIntake != 1 {
		t.Errorf("RejectedAtIntake = %d, want 1 (the 12.345 price)", accepted.RejectedAtIntake)
	}
	if batch.State != "completed" {
		t.Errorf("batch state = %q", batch.State)
	}
	if batch.CreatedCount != 3 { // NEW100, ZERO01, DUPX (last wins)
		t.Errorf("CreatedCount = %d, want 3", batch.CreatedCount)
	}
	if batch.UpdatedCount != 1 {
		t.Errorf("UpdatedCount = %d, want 1 (4001 via the link)", batch.UpdatedCount)
	}
	if batch.UnchangedCount != 1 {
		t.Errorf("UnchangedCount = %d, want 1 (DUPX first occurrence)", batch.UnchangedCount)
	}
	if batch.FailedCount != 1 {
		t.Errorf("FailedCount = %d, want 1", batch.FailedCount)
	}
	if batch.ChunkCount != 1 || batch.ChunksDone != 1 {
		t.Errorf("chunks = %d done %d, want 1/1", batch.ChunkCount, batch.ChunksDone)
	}

	byCode := f.itemsByCode(batch.ID)
	if failed := byCode["BAD01"]; len(failed) != 1 || failed[0].State != "failed" || failed[0].Code == nil || *failed[0].Code != "ERP_ITEM_PRICE_PRECISION" {
		t.Errorf("BAD01 rows = %+v, want one failed ERP_ITEM_PRICE_PRECISION", failed)
	}
	if superseded := byCode["DUPX"]; len(superseded) != 2 {
		t.Fatalf("DUPX rows = %d, want 2", len(superseded))
	} else {
		first, last := superseded[0], superseded[1]
		if first.Position > last.Position {
			first, last = last, first
		}
		if first.State != "unchanged" || first.Code == nil || *first.Code != "ERP_ITEM_SUPERSEDED" {
			t.Errorf("earlier DUPX = %+v, want unchanged ERP_ITEM_SUPERSEDED", first)
		}
		if last.State != "created" {
			t.Errorf("last DUPX = %+v, want created", last)
		}
	}
	newRow, gotNew := byCode["NEW100"]
	if !gotNew || len(newRow) != 1 || newRow[0].State != "created" || newRow[0].VariantID == nil {
		t.Fatalf("NEW100 = %v, want created with a variant", newRow)
	}
	if qty, ok := f.stockQuantity(*newRow[0].VariantID); !ok || qty != 4 {
		t.Errorf("NEW100 stock = %d (%v), want 4", qty, ok)
	}
	if qty, ok := f.stockQuantity(*byCode["ZERO01"][0].VariantID); !ok || qty != 0 {
		t.Errorf("ZERO01 stock = %d (%v), want a real 0", qty, ok)
	}
	// A variant the batch never mentioned is untouched: the seeded quiet
	// variant keeps its SKU and price.
	quietSKU := "quiet-" + strconv.FormatInt(f.org%100000, 10)
	var quietPrice string
	err := f.db.InReadTx(database.AsSystem(f.ctx), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT price::text FROM catalog.product_variants WHERE organization_id = $1 AND sku = $2 AND deleted_at IS NULL`, f.org, quietSKU).Scan(&quietPrice)
	})
	if err != nil {
		t.Fatal(err)
	}
	if quietPrice != "5.00" {
		t.Errorf("unmentioned variant changed: price = %s, want 5.00", quietPrice)
	}
	if f.variantCount() != beforeVariants+3 {
		t.Errorf("variants = %d, want seed + 3 created", f.variantCount())
	}
	// One matcher call for the whole chunk, scoped as plan step 7 (f).
	if f.matcher.requests != 1 {
		t.Fatalf("matcher calls = %d, want 1", f.matcher.requests)
	}
	if want := fmt.Sprintf("supplier:%d", f.org); f.matcher.lastRequest.Scope.SourceNamespace != want {
		t.Errorf("matcher SourceNamespace = %q, want %q", f.matcher.lastRequest.Scope.SourceNamespace, want)
	}
	if f.matcher.lastRequest.Scope.Workflow != productmatch.WorkflowVendor || f.matcher.lastRequest.Scope.EntryPoint != "erp_sync" {
		t.Errorf("matcher scope workflow/entry = %s/%s", f.matcher.lastRequest.Scope.Workflow, f.matcher.lastRequest.Scope.EntryPoint)
	}
	if f.matcher.lastRequest.ConsumerRef != fmt.Sprintf("erp.sync:%d:0", batch.ID) {
		t.Errorf("matcher ConsumerRef = %q", f.matcher.lastRequest.ConsumerRef)
	}
	// Accepted links call Remember once with the supplier_code knowledge kind.
	if len(f.memory.kinds) == 0 || f.memory.kinds[0] != "supplier_code" {
		t.Errorf("Remember kinds = %v, want supplier_code first", f.memory.kinds)
	}
}
