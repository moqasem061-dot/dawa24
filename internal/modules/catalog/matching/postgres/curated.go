package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// ListActive retrieves all active (non-revoked) curated identity facts and pack relationships.
// Platform-owned tables are accessed through database.AsSystem(ctx).
func (r *Repository) ListActive(ctx context.Context) (map[int64][]productmatch.CuratedFact, map[int64][]productmatch.PackRelationship, error) {
	facts := make(map[int64][]productmatch.CuratedFact)
	rels := make(map[int64][]productmatch.PackRelationship)

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// 1. Query curated identity facts
		factsQuery := `
			SELECT id, product_id, attribute, value, reason, created_by, created_at, revoked_at, revoked_by
			FROM catalog.curated_identity_facts
			WHERE revoked_at IS NULL
			ORDER BY id ASC;
		`
		rows, err := tx.Query(txCtx, factsQuery)
		if err != nil {
			return fmt.Errorf("query curated facts: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var f productmatch.CuratedFact
			var val []byte
			if err := rows.Scan(
				&f.ID,
				&f.ProductID,
				&f.Attribute,
				&val,
				&f.Reason,
				&f.CreatedBy,
				&f.CreatedAt,
				&f.RevokedAt,
				&f.RevokedBy,
			); err != nil {
				return fmt.Errorf("scan curated fact: %w", err)
			}
			f.Value = json.RawMessage(val)
			facts[f.ProductID] = append(facts[f.ProductID], f)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate curated facts: %w", err)
		}

		// 2. Query pack relationships
		relsQuery := `
			SELECT id, product_id, from_unit, to_unit, factor, created_by, created_at, revoked_at, revoked_by
			FROM catalog.pack_relationships
			WHERE revoked_at IS NULL
			ORDER BY id ASC;
		`
		relRows, err := tx.Query(txCtx, relsQuery)
		if err != nil {
			return fmt.Errorf("query pack relationships: %w", err)
		}
		defer relRows.Close()

		for relRows.Next() {
			var rel productmatch.PackRelationship
			if err := relRows.Scan(
				&rel.ID,
				&rel.ProductID,
				&rel.FromUnit,
				&rel.ToUnit,
				&rel.Factor,
				&rel.CreatedBy,
				&rel.CreatedAt,
				&rel.RevokedAt,
				&rel.RevokedBy,
			); err != nil {
				return fmt.Errorf("scan pack relationship: %w", err)
			}
			rels[rel.ProductID] = append(rels[rel.ProductID], rel)
		}
		return relRows.Err()
	})

	if err != nil {
		return nil, nil, err
	}
	return facts, rels, nil
}
