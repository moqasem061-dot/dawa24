package erp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp/provider"
	"github.com/moqasem061-dot/dawa24/internal/platform/queue"
)

// Accepted is the 202 response's data: where to poll and how the batch was cut.
type Accepted struct {
	SyncID           string `json:"syncId"`
	State            string `json:"state"`
	ItemCount        int    `json:"itemCount"`
	RejectedAtIntake int    `json:"rejectedAtIntake"`
}

// bodySHA256 is the idempotency fingerprint of the raw body.
func bodySHA256(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// vendorCapability are the org types that may sell into the marketplace. The
// key's org comes from authentication; a pharmacy key may never create
// products.
func vendorCapability(orgType string) bool {
	switch orgType {
	case "vendor", "supplier", "company", "agency":
		return true
	}
	return false
}

// AcceptBatch reads a product-sync body, cuts it into chunks, stores every row
// (valid ones pending, invalid ones failed with their item-level code) and
// enqueues one erp.sync_chunk job per chunk inside the acceptance transaction.
//
// Deduplication: the same external code twice in one batch is last-wins; the
// earlier occurrence is stored unchanged with ERP_ITEM_SUPERSEDED and is never
// processed. Idempotency: the same Idempotency-Key with the same body returns
// the first accepted result untouched; the same key with a different body is
// ERP_IDEMPOTENCY_CONFLICT (trap 25).
func (s *Service) AcceptBatch(ctx context.Context, p Principal, body []byte, idemKey string) (Accepted, error) {
	if !p.HasScope(ScopeProductsSync) {
		return Accepted{}, NewError("ERP_SCOPE_MISSING", 403, nil)
	}
	if !vendorCapability(p.OrgType) {
		return Accepted{}, NewError("ERP_CAPABILITY_REQUIRED", 403, nil)
	}
	sha := bodySHA256(body)

	if idemKey != "" {
		existing, found, err := s.repo.FindBatchByIdempotency(ctx, p.OrganizationID, p.IntegrationID, idemKey)
		if err != nil {
			return Accepted{}, NewError("ERP_INTERNAL", 500, nil)
		}
		if found {
			if existing.BodySHA256 == sha {
				return Accepted{
					SyncID:           existing.PublicID,
					State:            existing.State,
					ItemCount:        existing.ItemCount,
					RejectedAtIntake: s.intakeRejected(ctx, p.OrganizationID, existing.ID),
				}, nil
			}
			return Accepted{}, NewError("ERP_IDEMPOTENCY_CONFLICT", 409, nil)
		}
	}

	dec, ok := provider.Get(p.Provider)
	if !ok {
		return Accepted{}, NewError("ERP_UNAVAILABLE", 503, nil)
	}
	batch, err := dec.DecodeProductBatch(body)
	if err != nil {
		return Accepted{}, NewError("ERP_BAD_JSON", 400, nil)
	}

	type staged struct {
		code    string
		state   string // pending until a later duplicate supersedes it
		itemErr *ItemError
		raw     provider.RawItem
	}
	rows := make([]staged, 0, len(batch.Items))
	positionOf := map[string]int{} // code -> position of the surviving row

	// Normalize first, then dedupe with last-wins.
	for _, raw := range batch.Items {
		item, itemErr := NormalizeItem(raw)
		if itemErr != nil {
			rows = append(rows, staged{code: raw.Code, state: "failed", itemErr: itemErr, raw: raw})
			positionOf[raw.Code] = len(rows) - 1 // an invalid row still owns its code
			continue
		}
		if existing, dup := positionOf[item.Code]; dup {
			// An earlier valid row loses to this one (last wins); a superseded
			// row is stored as unchanged and is never processed.
			if rows[existing].state == "pending" {
				rows[existing].state = "unchanged"
			}
			rows = append(rows, staged{code: item.Code, state: "pending", raw: raw})
			positionOf[item.Code] = len(rows) - 1
			continue
		}
		rows = append(rows, staged{code: item.Code, state: "pending", raw: raw})
		positionOf[item.Code] = len(rows) - 1
	}

	total := len(rows)
	if total == 0 {
		return Accepted{}, NewError("ERP_BATCH_EMPTY", 422, nil)
	}
	if total > p.MaxBatchItems {
		return Accepted{}, NewError("ERP_BATCH_TOO_LARGE", 413, map[string]any{"limit": p.MaxBatchItems, "received": total})
	}

	chunkCount := (total + SyncChunkSize - 1) / SyncChunkSize
	itemRows := make([]NewItemRow, 0, total)
	rejected := 0
	for position, row := range rows {
		newRow := NewItemRow{Position: position, ChunkNo: position / SyncChunkSize, ExternalCode: row.code}
		switch {
		case row.itemErr != nil:
			newRow.State = "failed"
			newRow.Code = row.itemErr.Code
			newRow.Payload, _ = json.Marshal(row.raw)
			rejected++
		case row.state == "unchanged":
			newRow.State = "unchanged"
			newRow.Code = "ERP_ITEM_SUPERSEDED"
			newRow.Payload, _ = json.Marshal(row.raw)
		default:
			newRow.State = "pending"
			newRow.Payload, _ = json.Marshal(row.raw)
		}
		itemRows = append(itemRows, newRow)
	}

	if s.ports.Enqueuer == nil {
		return Accepted{}, NewError("ERP_UNAVAILABLE", 503, nil)
	}
	inserted, err := s.repo.InsertBatch(ctx, p.OrganizationID, NewBatch{
		IntegrationID: p.IntegrationID, APIKeyID: p.KeyID,
		WarehouseID: p.WarehouseID, BranchID: p.BranchID,
		ClientBatchID: batch.ClientBatchID, IdempotencyKey: idemKey, BodySHA256: sha,
		ItemCount: total, ChunkCount: chunkCount,
	}, itemRows, func(ctx context.Context, tx pgx.Tx, inserted SyncBatch) error {
		// One erp.sync_chunk job per chunk, inside the acceptance transaction:
		// a job that outlives a rolled-back batch can never exist.
		for chunk := 0; chunk < inserted.ChunkCount; chunk++ {
			if _, err := s.ports.Enqueuer.EnqueueTx(ctx, tx, queue.ERPSyncChunkArgs{
				BatchID: inserted.ID, OrganizationID: p.OrganizationID, ChunkNo: chunk,
			}, &river.InsertOpts{Queue: "erpsync", MaxAttempts: 3}); err != nil {
				return fmt.Errorf("erp: enqueue chunk %d: %w", chunk, err)
			}
		}
		return nil
	})
	if err != nil {
		// Hiding the persistence error behind ERP_INTERNAL here would make a
		// real bug invisible; Return a wrapped error, which writeError maps
		// to 500 with just the generic text.
		return Accepted{}, fmt.Errorf("erp: insert batch: %w", err)
	}

	return Accepted{SyncID: inserted.PublicID, State: inserted.State, ItemCount: total, RejectedAtIntake: rejected}, nil
}

// intakeRejected counts the rows intake itself refused: failed rows that no
// processing has touched (write failures carry ERP_ITEM_WRITE_FAILED and have
// been processed; intake refusals never are).
func (s *Service) intakeRejected(ctx context.Context, orgID, batchID int64) int {
	items, err := s.repo.ListSyncItems(ctx, orgID, batchID, 0, "failed", intakeScanLimit)
	if err != nil {
		return 0
	}
	count := 0
	for _, item := range items {
		if item.ProcessedAt == nil {
			count++
		}
	}
	return count
}

const intakeScanLimit = 100000
