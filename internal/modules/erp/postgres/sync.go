package postgres

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// The batch half of the sync persistence: acceptance writes the batch and its
// items inside one transaction (the enqueue callback runs in that transaction
// too, so a chunk job can never outlive a rolled-back batch), and the chunk
// lifecycle advances through queued -> processing -> completed.

const batchColumns = `b.id, b.organization_id, b.integration_id, b.api_key_id, b.warehouse_id, b.branch_id, COALESCE(b.client_batch_id, ''), COALESCE(b.idempotency_key, ''), b.body_sha256, b.state, b.item_count, b.chunk_count, b.chunks_done, b.created_count, b.updated_count, b.unchanged_count, b.needs_review_count, b.failed_count, COALESCE(b.error_code, ''), b.accepted_at, b.started_at, b.finished_at, i.provider`

func scanBatch(row scanner) (erp.SyncBatch, error) {
	var b erp.SyncBatch
	err := row.Scan(&b.ID, &b.OrganizationID, &b.IntegrationID, &b.APIKeyID, &b.WarehouseID, &b.BranchID, &b.ClientBatchID, &b.IdempotencyKey, &b.BodySHA256, &b.State, &b.ItemCount, &b.ChunkCount, &b.ChunksDone, &b.CreatedCount, &b.UpdatedCount, &b.UnchangedCount, &b.NeedsReviewCount, &b.FailedCount, &b.ErrorCode, &b.AcceptedAt, &b.StartedAt, &b.FinishedAt, &b.Provider)
	if errors.Is(err, pgx.ErrNoRows) {
		return b, erp.ErrNotFound
	}
	// The sync id the API returns is the decimal batch id (at most 20
	// characters). PublicID keeps its name and now holds that id as text.
	b.PublicID = strconv.FormatInt(b.ID, 10)
	return b, err
}

// InsertBatch writes the batch, COPYs its items, and runs the enqueue callback
// inside the same transaction (plan.md T101). Never db.Pool() — trap 1.
func (r *Repository) InsertBatch(ctx context.Context, orgID int64, b erp.NewBatch, items []erp.NewItemRow, enqueue func(ctx context.Context, tx pgx.Tx, batch erp.SyncBatch) error) (erp.SyncBatch, error) {
	var out erp.SyncBatch
	err := r.db.InTx(database.WithTenant(ctx, orgID), func(ctx context.Context, tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `INSERT INTO erp.sync_batches
			(organization_id, integration_id, api_key_id, warehouse_id, branch_id, client_batch_id, idempotency_key, body_sha256, item_count, chunk_count)
			VALUES ($1,$2,NULLIF($3,0),$4,$5,NULLIF($6,''),NULLIF($7,''),$8,$9,$10)
			RETURNING id`, orgID, b.IntegrationID, b.APIKeyID, b.WarehouseID, b.BranchID, b.ClientBatchID, b.IdempotencyKey, b.BodySHA256, b.ItemCount, b.ChunkCount)
		var id int64
		if err := row.Scan(&id); err != nil {
			return fmt.Errorf("erp: insert batch: %w", err)
		}
		// The plan's CopyFrom is refused by PostgreSQL on tables with RLS
		// (SQLSTATE 0A000); the equivalent batched insert through unnest
		// carries the same rows inside the same transaction. payload and
		// message arrive as text and cast to JSONB server-side.
		if len(items) > 0 {
			positions := make([]int64, 0, len(items))
			chunks := make([]int64, 0, len(items))
			codes := make([]string, 0, len(items))
			payloads := make([]string, 0, len(items))
			states := make([]string, 0, len(items))
			itemCodes := make([]*string, 0, len(items))
			messages := make([]string, 0, len(items))
			for _, item := range items {
				positions = append(positions, int64(item.Position))
				chunks = append(chunks, int64(item.ChunkNo))
				codes = append(codes, item.ExternalCode)
				payloads = append(payloads, string(item.Payload))
				states = append(states, item.State)
				itemCodes = append(itemCodes, nilIfEmpty(item.Code))
				messages = append(messages, messageJSONText(item.Message))
			}
			if _, err := tx.Exec(ctx, `INSERT INTO erp.sync_items
				(organization_id, batch_id, position, chunk_no, external_code, payload, state, code, message)
				SELECT $1, $2, u.position, u.chunk_no, u.external_code, u.payload::jsonb, u.state, u.item_code, u.message::jsonb
				FROM unnest($3::bigint[], $4::bigint[], $5::text[], $6::text[], $7::text[], $8::text[], $9::text[])
					AS u(position, chunk_no, external_code, payload, state, item_code, message)`,
				orgID, id, positions, chunks, codes, payloads, states, itemCodes, messages); err != nil {
				return fmt.Errorf("erp: insert sync items: %w", err)
			}
		}
		// Re-read so the caller sees exactly what the database stored,
		// counters included.
		batch, err := scanBatch(tx.QueryRow(ctx, `SELECT `+batchColumns+`
			FROM erp.sync_batches b JOIN erp.integrations i ON i.id = b.integration_id
			WHERE b.organization_id = $1 AND b.id = $2`, orgID, id))
		if err != nil {
			return err
		}
		if enqueue != nil {
			if err := enqueue(ctx, tx, batch); err != nil {
				return err
			}
		}
		out = batch
		return nil
	})
	return out, err
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (r *Repository) FindBatchByIdempotency(ctx context.Context, orgID, integrationID int64, key string) (erp.SyncBatch, bool, error) {
	var out erp.SyncBatch
	err := r.db.InReadTx(database.WithTenant(ctx, orgID), func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, err = scanBatch(tx.QueryRow(ctx, `SELECT `+batchColumns+`
			FROM erp.sync_batches b JOIN erp.integrations i ON i.id = b.integration_id
			WHERE b.organization_id = $1 AND b.integration_id = $2 AND b.idempotency_key = $3`, orgID, integrationID, key))
		return err
	})
	if errors.Is(err, erp.ErrNotFound) {
		return erp.SyncBatch{}, false, nil
	}
	return out, err == nil, err
}

// GetBatchByPublicID reads a batch by the sync id the caller holds. The id is
// the decimal batch id; the UUID that older clients were given is still
// accepted and looked up by public_id. Any other text is not found.
func (r *Repository) GetBatchByPublicID(ctx context.Context, orgID int64, syncID string) (erp.SyncBatch, error) {
	if id, ok := erp.ParseShortID(syncID); ok {
		return r.GetBatchByID(ctx, orgID, id)
	}
	if !uuidPattern.MatchString(syncID) {
		return erp.SyncBatch{}, erp.ErrNotFound
	}
	var out erp.SyncBatch
	err := r.db.InReadTx(database.WithTenant(ctx, orgID), func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, err = scanBatch(tx.QueryRow(ctx, `SELECT `+batchColumns+`
			FROM erp.sync_batches b JOIN erp.integrations i ON i.id = b.integration_id
			WHERE b.organization_id = $1 AND b.public_id = $2::uuid`, orgID, syncID))
		return err
	})
	return out, err
}

func (r *Repository) GetBatchByID(ctx context.Context, orgID, batchID int64) (erp.SyncBatch, error) {
	var out erp.SyncBatch
	err := r.db.InReadTx(database.WithTenant(ctx, orgID), func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, err = scanBatch(tx.QueryRow(ctx, `SELECT `+batchColumns+`
			FROM erp.sync_batches b JOIN erp.integrations i ON i.id = b.integration_id
			WHERE b.organization_id = $1 AND b.id = $2`, orgID, batchID))
		return err
	})
	return out, err
}

// EarlierBatchActive enforces the cross-batch order (research D-09): an earlier
// batch of the same integration that is queued or processing holds back the
// later one.
func (r *Repository) EarlierBatchActive(ctx context.Context, orgID, integrationID, batchID int64) (bool, error) {
	var yes bool
	err := r.db.InReadTx(database.WithTenant(ctx, orgID), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT EXISTS (
			SELECT 1 FROM erp.sync_batches
			WHERE organization_id = $1 AND integration_id = $2 AND id < $3
			  AND state IN ('queued','processing')
		)`, orgID, integrationID, batchID).Scan(&yes)
	})
	return yes, err
}

// MarkBatchProcessing flips queued -> processing exactly once.
func (r *Repository) MarkBatchProcessing(ctx context.Context, orgID, batchID int64) error {
	return r.db.InTx(database.WithTenant(ctx, orgID), func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE erp.sync_batches SET state = 'processing', started_at = COALESCE(started_at, now())
			WHERE organization_id = $1 AND id = $2 AND state = 'queued'`, orgID, batchID)
		if err != nil {
			return fmt.Errorf("erp: mark batch processing: %w", err)
		}
		_ = tag
		return nil
	})
}

// FinishChunk advances chunks_done once per chunk; the chunk that reaches
// chunk_count folds the per-item counters into the batch, marks it completed
// and stamps finished_at. The state guard makes a retried FinishChunk a no-op
// (trap 25: idempotent everywhere).
func (r *Repository) FinishChunk(ctx context.Context, orgID, batchID int64) (bool, error) {
	var done bool
	err := r.db.InTx(database.WithTenant(ctx, orgID), func(ctx context.Context, tx pgx.Tx) error {
		var chunksDone, chunkCount int
		var state string
		err := tx.QueryRow(ctx, `UPDATE erp.sync_batches SET chunks_done = LEAST(chunks_done + 1, chunk_count)
			WHERE organization_id = $1 AND id = $2 AND state IN ('queued','processing')
			RETURNING chunks_done, chunk_count, state`, orgID, batchID).Scan(&chunksDone, &chunkCount, &state)
		if errors.Is(err, pgx.ErrNoRows) {
			// Already completed (a retried job): done, nothing to change.
			done = true
			return nil
		}
		if err != nil {
			return fmt.Errorf("erp: finish chunk: %w", err)
		}
		// A retried job (River is at-least-once) can bump the counter for a
		// chunk already counted, so the counter alone cannot complete the
		// batch: completion also needs every item to be out of 'pending'.
		var pendingLeft bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM erp.sync_items
			WHERE organization_id = $1 AND batch_id = $2 AND state = 'pending')`, orgID, batchID).Scan(&pendingLeft); err != nil {
			return fmt.Errorf("erp: finish chunk pending check: %w", err)
		}
		if chunksDone < chunkCount || pendingLeft {
			return nil
		}
		if _, err := tx.Exec(ctx, `UPDATE erp.sync_batches SET
			state = 'completed', finished_at = now(),
			created_count = u.created, updated_count = u.updated, unchanged_count = u.unchanged,
			needs_review_count = u.needs_review, failed_count = u.failed
			FROM (
				SELECT
					count(*) FILTER (WHERE s.state = 'created') AS created,
					count(*) FILTER (WHERE s.state = 'updated') AS updated,
					count(*) FILTER (WHERE s.state = 'unchanged') AS unchanged,
					count(*) FILTER (WHERE s.state = 'needs_review') AS needs_review,
					count(*) FILTER (WHERE s.state = 'failed') AS failed
				FROM erp.sync_items s WHERE s.organization_id = $1 AND s.batch_id = $2
			) u
			WHERE erp.sync_batches.organization_id = $1 AND erp.sync_batches.id = $2`, orgID, batchID); err != nil {
			return fmt.Errorf("erp: fold batch counters: %w", err)
		}
		done = true
		return nil
	})
	return done, err
}
