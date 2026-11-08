package erp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// messageJSON serialises a bilingual item message, never nil.
func messageJSON(m map[string]string) []byte {
	if m == nil {
		return []byte("{}")
	}
	data, _ := json.Marshal(m)
	return data
}

// fakeSync is the in-memory SyncRepository the S2 unit tests run against.
// State lives in maps keyed exactly like the real tables, so behaviours the
// tests assert (last-wins dedupe, chunk counting, idempotency, pending-row
// retries) are exercised without a database.
type fakeSync struct {
	batches       map[int64]*SyncBatch
	items         map[int64]*SyncItemRow
	links         map[string]*ProductLink
	reviews       map[int64]*ReviewItem
	reviewsByKey  map[string]*ReviewItem
	skuIndex      map[string][]int64
	barcodeIndex  map[string][]int64
	waitOnBatch   int64
	sequences     map[string]int64
	inserted      []NewItemRow
	results       []ItemResult
	linkWrites    []LinkWrite
	reviewRows    []ReviewWrite
	audits        []AuditRecord
	purgerCalls   int
	purgerCutoffs Cutoffs
}

func newFakeSync() *fakeSync {
	return &fakeSync{
		batches:      map[int64]*SyncBatch{},
		items:        map[int64]*SyncItemRow{},
		links:        map[string]*ProductLink{},
		reviews:      map[int64]*ReviewItem{},
		reviewsByKey: map[string]*ReviewItem{},
		skuIndex:     map[string][]int64{},
		barcodeIndex: map[string][]int64{},
		sequences:    map[string]int64{},
	}
}

func (f *fakeSync) next(table string) int64 {
	f.sequences[table]++
	return f.sequences[table]
}

func (f *fakeSync) InsertBatch(_ context.Context, orgID int64, b NewBatch, items []NewItemRow, enqueue func(ctx context.Context, tx pgx.Tx, batch SyncBatch) error) (SyncBatch, error) {
	id := f.next("sync_batches")
	now := time.Now().UTC()
	batch := &SyncBatch{
		ID: id, PublicID: fmt.Sprintf("sync-%d", id), OrganizationID: orgID,
		IntegrationID: b.IntegrationID, APIKeyID: ptrInt64(b.APIKeyID),
		WarehouseID: b.WarehouseID, BranchID: b.BranchID,
		ClientBatchID: b.ClientBatchID, IdempotencyKey: b.IdempotencyKey, BodySHA256: b.BodySHA256,
		State: "queued", ItemCount: b.ItemCount, ChunkCount: b.ChunkCount, AcceptedAt: now,
	}
	f.batches[id] = batch
	for _, row := range items {
		itemID := f.next("sync_items")
		var code *string
		if row.Code != "" {
			text := row.Code
			code = &text
		}
		f.items[itemID] = &SyncItemRow{
			ID: itemID, OrganizationID: orgID, BatchID: id, Position: row.Position,
			ChunkNo: row.ChunkNo, ExternalCode: row.ExternalCode, Payload: row.Payload,
			State: row.State, Code: code, Message: messageJSON(row.Message),
		}
		f.inserted = append(f.inserted, row)
	}
	if enqueue != nil {
		if err := enqueue(context.Background(), nil, *batch); err != nil {
			return *batch, err
		}
	}
	return *batch, nil
}

func (f *fakeSync) FindBatchByIdempotency(_ context.Context, orgID, integrationID int64, key string) (SyncBatch, bool, error) {
	for _, batch := range f.batches {
		if batch.OrganizationID == orgID && batch.IntegrationID == integrationID && batch.IdempotencyKey == key {
			return *batch, true, nil
		}
	}
	return SyncBatch{}, false, nil
}

func (f *fakeSync) GetBatchByPublicID(_ context.Context, orgID int64, publicID string) (SyncBatch, error) {
	for _, batch := range f.batches {
		if batch.OrganizationID == orgID && batch.PublicID == publicID {
			return *batch, nil
		}
	}
	return SyncBatch{}, ErrNotFound
}

func (f *fakeSync) GetBatchByID(_ context.Context, orgID, batchID int64) (SyncBatch, error) {
	batch, ok := f.batches[batchID]
	if !ok || batch.OrganizationID != orgID {
		return SyncBatch{}, ErrNotFound
	}
	return *batch, nil
}

func (f *fakeSync) ListSyncItems(_ context.Context, orgID, batchID, afterID int64, state string, limit int) ([]SyncItemRow, error) {
	var out []SyncItemRow
	for _, item := range f.items {
		if item.OrganizationID != orgID || item.BatchID != batchID || item.ID <= afterID {
			continue
		}
		if state != "" && item.State != state {
			continue
		}
		out = append(out, *item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (f *fakeSync) EarlierBatchActive(_ context.Context, orgID, integrationID, batchID int64) (bool, error) {
	if f.waitOnBatch == batchID {
		return true, nil
	}
	for _, batch := range f.batches {
		if batch.OrganizationID == orgID && batch.IntegrationID == integrationID &&
			batch.ID < batchID && (batch.State == "queued" || batch.State == "processing") {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeSync) MarkBatchProcessing(_ context.Context, orgID, batchID int64) error {
	batch, ok := f.batches[batchID]
	if !ok || batch.OrganizationID != orgID {
		return ErrNotFound
	}
	if batch.State == "queued" {
		batch.State = "processing"
		now := time.Now().UTC()
		batch.StartedAt = &now
	}
	return nil
}

func (f *fakeSync) LoadPendingItems(_ context.Context, orgID, batchID int64, chunkNo int) ([]SyncItemRow, error) {
	var out []SyncItemRow
	for _, item := range f.items {
		if item.OrganizationID == orgID && item.BatchID == batchID && item.ChunkNo == chunkNo && item.State == "pending" {
			out = append(out, *item)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Position < out[j].Position })
	return out, nil
}

func (f *fakeSync) LoadLinksByCode(_ context.Context, orgID, integrationID int64, codes []string) (map[string]ProductLink, error) {
	out := map[string]ProductLink{}
	for _, code := range codes {
		if link, ok := f.links[linkKey(integrationID, code)]; ok && link.OrganizationID == orgID {
			out[code] = *link
		}
	}
	return out, nil
}

func linkKey(integrationID int64, code string) string {
	return strconv.FormatInt(integrationID, 10) + "|" + code
}

func (f *fakeSync) FindVariantsBySKU(_ context.Context, orgID int64, codes []string) (map[string][]int64, error) {
	return f.tierIndex(codes, f.skuIndex), nil
}

func (f *fakeSync) FindVariantsByBarcode(_ context.Context, orgID int64, barcodes []string) (map[string][]int64, error) {
	return f.tierIndex(barcodes, f.barcodeIndex), nil
}

func (f *fakeSync) tierIndex(codes []string, index map[string][]int64) map[string][]int64 {
	out := map[string][]int64{}
	for _, code := range codes {
		if ids, ok := index[code]; ok {
			out[code] = append([]int64{}, ids...)
		}
	}
	return out
}

func (f *fakeSync) WriteResults(_ context.Context, orgID int64, res []ItemResult, links []LinkWrite, reviews []ReviewWrite) error {
	for _, r := range res {
		item, ok := f.items[r.ItemID]
		if !ok || item.OrganizationID != orgID {
			continue
		}
		item.State = r.State
		item.VariantID = r.VariantID
		if r.Code != "" || len(r.Message) > 0 {
			item.Message = messageJSON(r.Message)
			text := r.Code
			item.Code = &text
		}
		if r.State != "pending" {
			now := time.Now().UTC()
			item.ProcessedAt = &now
			item.Payload = nil // processed rows drop their payload (T102)
		}
	}
	for _, link := range links {
		if existing, ok := f.links[linkKey(link.IntegrationID, link.ExternalCode)]; ok {
			existing.VariantID = link.VariantID
			existing.ProductID = link.ProductID
			existing.LastHash = link.LastHash
			existing.LinkSource = link.LinkSource
			existing.TaxPercent = link.TaxPercent
			existing.LastSyncedAt = ptrTime(time.Now().UTC())
		} else {
			f.links[linkKey(link.IntegrationID, link.ExternalCode)] = &ProductLink{
				OrganizationID: orgID, IntegrationID: link.IntegrationID, ExternalCode: link.ExternalCode,
				VariantID: link.VariantID, ProductID: link.ProductID, LinkSource: link.LinkSource,
				LastHash: link.LastHash, TaxPercent: link.TaxPercent,
			}
		}
		f.linkWrites = append(f.linkWrites, link)
	}
	for _, review := range reviews {
		key := linkKey(review.IntegrationID, review.ExternalCode)
		existing, ok := f.reviewsByKey[key]
		if !ok {
			id := f.next("review_items")
			existing = &ReviewItem{ID: id, OrganizationID: orgID, State: "open"}
			f.reviewsByKey[key] = existing
			f.reviews[id] = existing
		}
		existing.IntegrationID = review.IntegrationID
		existing.BatchID = ptrInt64(review.BatchID)
		existing.ExternalCode = review.ExternalCode
		existing.ReasonCode = review.ReasonCode
		existing.Payload = review.Payload
		existing.Candidates = review.Candidates
		f.reviewRows = append(f.reviewRows, review)
	}
	f.results = append(f.results, res...)
	return nil
}

func (f *fakeSync) FinishChunk(_ context.Context, orgID, batchID int64) (bool, error) {
	batch, ok := f.batches[batchID]
	if !ok || batch.OrganizationID != orgID {
		return false, ErrNotFound
	}
	// Idempotent on retries: only a queued or processing batch advances. A
	// already-completed batch just reports done.
	if batch.State != "queued" && batch.State != "processing" {
		return true, nil
	}
	now := time.Now().UTC()
	if batch.State == "processing" && batch.StartedAt == nil {
		batch.StartedAt = &now
	}
	batch.ChunksDone++
	if batch.ChunksDone >= batch.ChunkCount {
		batch.State = "completed"
		batch.FinishedAt = &now
		// Fold the per-item states into the counters, like the SQL does.
		for _, item := range f.items {
			if item.BatchID != batchID {
				continue
			}
			switch item.State {
			case "created":
				batch.CreatedCount++
			case "updated":
				batch.UpdatedCount++
			case "unchanged":
				batch.UnchangedCount++
			case "needs_review":
				batch.NeedsReviewCount++
			case "failed":
				batch.FailedCount++
			}
		}
		return true, nil
	}
	return false, nil
}

func (f *fakeSync) ListReview(_ context.Context, orgID int64, page Page) ([]ReviewItem, error) {
	var out []ReviewItem
	for _, review := range f.reviews {
		if review.OrganizationID != orgID || review.State != "open" {
			continue
		}
		if page.After > 0 && review.ID <= page.After {
			continue
		}
		out = append(out, *review)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	if len(out) > page.Limit {
		out = out[:page.Limit]
	}
	return out, nil
}

func (f *fakeSync) GetReview(_ context.Context, orgID, id int64) (ReviewItem, error) {
	review, ok := f.reviews[id]
	if !ok || review.OrganizationID != orgID {
		return ReviewItem{}, ErrNotFound
	}
	return *review, nil
}

func (f *fakeSync) ResolveReviewRow(_ context.Context, orgID, id, variantID, productID, actorUserID int64, audit AuditRecord) error {
	review, ok := f.reviews[id]
	if !ok || review.OrganizationID != orgID || review.State != "open" {
		return ErrNotFound
	}
	review.State = "resolved"
	review.ResolvedVariantID = &variantID
	review.ResolvedProductID = &productID
	review.ResolvedBy = &actorUserID
	now := time.Now().UTC()
	review.ResolvedAt = &now
	f.audits = append(f.audits, audit)
	return nil
}

func (f *fakeSync) DismissReviewRow(_ context.Context, orgID, id, actorUserID int64, audit AuditRecord) error {
	review, ok := f.reviews[id]
	if !ok || review.OrganizationID != orgID {
		return ErrNotFound
	}
	review.State = "dismissed"
	review.ResolvedBy = &actorUserID
	now := time.Now().UTC()
	review.ResolvedAt = &now
	f.audits = append(f.audits, audit)
	return nil
}

func (f *fakeSync) PurgeExpiredAsSystem(_ context.Context, cut Cutoffs) error {
	f.purgerCalls++
	f.purgerCutoffs = cut
	return nil
}

// ports fakes for the S2 unit tests.

type fakeVariantWriter struct {
	written    []VariantWrite
	quotaCalls []map[int64]*int
	failures   map[int]string
	nextID     int64
	ids        map[int]int64
}

func (w *fakeVariantWriter) Write(_ context.Context, _ int64, rows []VariantWrite) (VariantWriteResult, error) {
	result := VariantWriteResult{IDs: map[int]int64{}, InsertedRefs: map[int]bool{}, Failures: map[int]string{}}
	for _, row := range rows {
		w.written = append(w.written, row)
		if failure, bad := w.failures[row.Ref]; bad {
			result.Failures[row.Ref] = failure
			continue
		}
		w.nextID++
		id := int64(10000 + int(w.nextID))
		result.IDs[row.Ref] = id
		if row.VariantID == 0 {
			result.Inserted++
			result.InsertedRefs[row.Ref] = true
		} else {
			result.Updated++
		}
	}
	return result, nil
}

func (w *fakeVariantWriter) SetQuotaLimits(_ context.Context, _ int64, limits map[int64]*int) error {
	w.quotaCalls = append(w.quotaCalls, limits)
	return nil
}

type fakeStockWriter struct {
	written  []StockWrite
	failures map[int]string
}

func (w *fakeStockWriter) Replace(_ context.Context, rows []StockWrite) (map[int]string, error) {
	failures := map[int]string{}
	for _, row := range rows {
		w.written = append(w.written, row)
		if failure, bad := w.failures[row.Ref]; bad {
			failures[row.Ref] = failure
		}
	}
	return failures, nil
}

// fakeMatcher records every Prepare and answers with the configured decision.
type fakeMatcher struct {
	state       matchflow.State
	productID   *int64
	err         error
	requests    int
	lastRequest productmatch.PrepareRequest
}

func (m *fakeMatcher) Prepare(_ context.Context, req productmatch.PrepareRequest) (productmatch.PrepareResult, error) {
	m.requests++
	m.lastRequest = req
	if m.err != nil {
		return productmatch.PrepareResult{}, m.err
	}
	decisions := make([]matchflow.RowDecision, len(req.Rows))
	for i := range decisions {
		decisions[i] = matchflow.RowDecision{State: m.state, ProductID: m.productID}
	}
	return productmatch.PrepareResult{Decisions: decisions}, nil
}

type fakeMatchMemory struct {
	namespaces []string
	kinds      []string
}

func (m *fakeMatchMemory) Remember(_ context.Context, sc productmatch.Scope, k KnowledgeWrite) error {
	m.namespaces = append(m.namespaces, sc.SourceNamespace)
	m.kinds = append(m.kinds, k.Kind)
	return nil
}

type fakeEnqueuer struct {
	jobs []river.JobArgs
}

func (f *fakeEnqueuer) EnqueueTx(_ context.Context, _ pgx.Tx, args river.JobArgs, _ *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	f.jobs = append(f.jobs, args)
	return &rivertype.JobInsertResult{}, nil
}

// small pointer helpers shared by the fake members and the S2 tests.
func ptrTime(t time.Time) *time.Time { return &t }
func ptrInt64(v int64) *int64        { return &v }
