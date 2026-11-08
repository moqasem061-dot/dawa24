package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// syncItemColumns mirrors erp.sync_items (T100: field names camel-to-Pascal).
const syncItemColumns = `s.id, s.organization_id, s.batch_id, s.position, s.chunk_no, s.external_code, s.payload, s.state, s.code, s.message, s.variant_id, s.processed_at`

func scanSyncItem(row scanner) (erp.SyncItemRow, error) {
	var s erp.SyncItemRow
	err := row.Scan(&s.ID, &s.OrganizationID, &s.BatchID, &s.Position, &s.ChunkNo, &s.ExternalCode, &s.Payload, &s.State, &s.Code, &s.Message, &s.VariantID, &s.ProcessedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return s, erp.ErrNotFound
	}
	return s, err
}

// ListSyncItems keyset-pages one batch's items by sync_items.id; an empty
// state means all states.
func (r *Repository) ListSyncItems(ctx context.Context, orgID, batchID, afterID int64, state string, limit int) ([]erp.SyncItemRow, error) {
	if limit <= 0 || limit > 50000 {
		limit = 5000
	}
	var out []erp.SyncItemRow
	err := r.db.InReadTx(database.WithTenant(ctx, orgID), func(ctx context.Context, tx pgx.Tx) error {
		cursor, err := tx.Query(ctx, `SELECT `+syncItemColumns+`
			FROM erp.sync_items s
			WHERE s.organization_id = $1 AND s.batch_id = $2 AND ($3 = 0 OR s.id > $3)
			  AND ($4 = '' OR s.state = $4)
			ORDER BY s.id LIMIT $5`, orgID, batchID, afterID, state, limit)
		if err != nil {
			return err
		}
		defer cursor.Close()
		for cursor.Next() {
			item, err := scanSyncItem(cursor)
			if err != nil {
				return err
			}
			out = append(out, item)
		}
		return cursor.Err()
	})
	return out, err
}

// LoadPendingItems reads one chunk's unprocessed rows in batch position order.
func (r *Repository) LoadPendingItems(ctx context.Context, orgID, batchID int64, chunkNo int) ([]erp.SyncItemRow, error) {
	var out []erp.SyncItemRow
	err := r.db.InReadTx(database.WithTenant(ctx, orgID), func(ctx context.Context, tx pgx.Tx) error {
		cursor, err := tx.Query(ctx, `SELECT `+syncItemColumns+`
			FROM erp.sync_items s
			WHERE s.organization_id = $1 AND s.batch_id = $2 AND s.chunk_no = $3 AND s.state = 'pending'
			ORDER BY s.position`, orgID, batchID, chunkNo)
		if err != nil {
			return err
		}
		defer cursor.Close()
		for cursor.Next() {
			item, err := scanSyncItem(cursor)
			if err != nil {
				return err
			}
			out = append(out, item)
		}
		return cursor.Err()
	})
	return out, err
}

// WriteResults persists one chunk's outcomes: item states (a retried chunk
// skips rows that are no longer pending — trap 25), links (upsert by
// integration + external code, carrying last_hash / tax_percent), and review
// rows (one open row per (integration, external code); a second sync updates
// it). One transaction for everything, org-scoped.
func (r *Repository) WriteResults(ctx context.Context, orgID int64, res []erp.ItemResult, links []erp.LinkWrite, reviews []erp.ReviewWrite) error {
	if err := r.db.InTx(database.WithTenant(ctx, orgID), func(ctx context.Context, tx pgx.Tx) error {
		for _, result := range res {
			if _, err := tx.Exec(ctx, `
				UPDATE erp.sync_items SET
					state = $3, code = NULLIF($4,''), message = $5::jsonb, variant_id = $6,
					processed_at = now(), payload = '{}'::jsonb
				WHERE organization_id = $1 AND id = $2 AND state = 'pending'`,
				orgID, result.ItemID, result.State, result.Code, messageJSONText(result.Message), result.VariantID); err != nil {
				return fmt.Errorf("erp: write item result %d: %w", result.ItemID, err)
			}
		}
		for _, link := range links {
			if _, err := tx.Exec(ctx, `
				INSERT INTO erp.product_links
					(organization_id, integration_id, external_code, variant_id, product_id, link_source, last_hash, tax_percent, last_synced_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
				ON CONFLICT (integration_id, external_code) DO UPDATE SET
					variant_id = EXCLUDED.variant_id, product_id = EXCLUDED.product_id,
					link_source = EXCLUDED.link_source, last_hash = EXCLUDED.last_hash,
					tax_percent = EXCLUDED.tax_percent, last_synced_at = now()`,
				orgID, link.IntegrationID, link.ExternalCode, link.VariantID, link.ProductID, link.LinkSource, link.LastHash, link.TaxPercent); err != nil {
				return fmt.Errorf("erp: upsert link %s: %w", link.ExternalCode, err)
			}
		}
		for _, review := range reviews {
			// Upsert one open row per (integration, external code): a second
			// sync of the same unknown code updates the payload and batch
			// rather than queueing a duplicate.
			candidates := string(review.Candidates)
			if candidates == "" {
				candidates = "[]"
			}
			tag, err := tx.Exec(ctx, revUpsertSQL,
				orgID, review.IntegrationID, review.ExternalCode,
				string(review.Payload), review.BatchID, candidates)
			if err != nil {
				return fmt.Errorf("erp: review upsert: %w", err)
			}
			if tag.RowsAffected() > 0 {
				continue
			}
			if _, err := tx.Exec(ctx, revInsertSQL,
				orgID, review.IntegrationID, review.ExternalCode,
				string(review.Payload), review.ReasonCode, review.BatchID, candidates); err != nil {
				return fmt.Errorf("erp: review insert: %w", err)
			}
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}
