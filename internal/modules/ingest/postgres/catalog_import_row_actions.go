package postgres

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/ingest"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// AssignRowMatch links a staged row to a master catalog product manually (or clears it if productID <= 0).
func (r *Repository) AssignRowMatch(
	ctx context.Context, importID, rowID, productID int64,
	productName, productSKU string,
) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		if productID > 0 {
			var rawName string
			var orgID int64
			err := tx.QueryRow(txCtx, `
				UPDATE ingest.catalog_import_rows
				SET product_id = $3,
				    match_level = 'exact',
				    match_score = 1.0000,
				    outcome = 'staged',
				    is_manually_matched = true,
				    message = 'مطابقة يدوية معتمدة من المستخدم'
				WHERE id = $1 AND import_id = $2
				RETURNING display_name, organization_id`,
				rowID, importID, productID).Scan(&rawName, &orgID)
			if err != nil {
				return err
			}

			normName := strings.ToLower(strings.TrimSpace(rawName))
			if normName != "" && orgID > 0 {
				var userID *int64
				if actor, ok := authctx.From(ctx); ok && actor.UserID > 0 {
					uid := actor.UserID
					userID = &uid
				}
				decKey := "manual:" + normName
				reason := "مطابقة يدوية معتمدة من المستخدم"

				// 1. Insert or update in catalog.match_decisions
				_, _ = tx.Exec(txCtx, `
					INSERT INTO catalog.match_decisions (
						organization_id, user_id, decision_key, norm_name, chosen_product_id,
						confidence, reason, prompt_version, hit_count, created_at, last_used_at,
						scope, source
					) VALUES (
						$1, $2, $3, $4, $5,
						1.000, $6, 'manual:v1', 1, now(), now(),
						'org', 'manual'
					)
					ON CONFLICT (COALESCE(organization_id, 0), decision_key)
					DO UPDATE SET
						chosen_product_id = EXCLUDED.chosen_product_id,
						confidence = 1.000,
						reason = EXCLUDED.reason,
						user_id = COALESCE(EXCLUDED.user_id, catalog.match_decisions.user_id),
						hit_count = catalog.match_decisions.hit_count + 1,
						last_used_at = now(),
						source = 'manual';
				`, orgID, userID, decKey, normName, productID, reason)

				// 2. Also register in catalog.customer_product_mappings
				_, _ = tx.Exec(txCtx, `
					INSERT INTO catalog.customer_product_mappings (
						organization_id, customer_org_id, raw_name, product_id,
						source, status, is_active, created_at, updated_at
					) VALUES (
						$1, $1, $2, $3,
						'manual', 'processed', true, now(), now()
					)
					ON CONFLICT DO NOTHING;
				`, orgID, rawName, productID)
			}
		} else {
			if _, err := tx.Exec(txCtx, `
				UPDATE ingest.catalog_import_rows
				SET product_id = NULL,
				    match_level = 'none',
				    match_score = 0.0000,
				    outcome = 'staged',
				    is_manually_matched = false,
				    message = 'تم إلغاء الربط بالكتالوج'
				WHERE id = $1 AND import_id = $2`,
				rowID, importID); err != nil {
				return err
			}
		}

		_, err := tx.Exec(txCtx, `
			UPDATE ingest.catalog_imports
			SET matched_rows = (SELECT COUNT(*) FROM ingest.catalog_import_rows WHERE import_id = $1 AND (is_manually_matched = true OR match_level IN ('barcode', 'code', 'exact', 'strong')) AND product_id IS NOT NULL AND product_id > 0),
			    review_rows = (SELECT COUNT(*) FROM ingest.catalog_import_rows WHERE import_id = $1 AND NOT is_manually_matched AND match_level IN ('review', 'ambiguous')),
			    unmatched_rows = (SELECT COUNT(*) FROM ingest.catalog_import_rows WHERE import_id = $1 AND NOT is_manually_matched AND (product_id IS NULL OR product_id = 0 OR match_level IN ('none', 'unmatched', '')) AND match_level NOT IN ('review', 'ambiguous', 'barcode', 'code', 'exact', 'strong'))
			WHERE id = $1`, importID)
		return err
	})
}

// ToggleRowExclude flips the is_excluded flag of a staged row.
func (r *Repository) ToggleRowExclude(ctx context.Context, importID, rowID int64) (bool, error) {
	var next bool
	err := r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `
			UPDATE ingest.catalog_import_rows
			SET is_excluded = NOT is_excluded
			WHERE id = $1 AND import_id = $2
			RETURNING is_excluded`,
			rowID, importID).Scan(&next)
	})
	return next, err
}

// StagedRowsForCommit returns the rows the vendor has actually confirmed.
//
// Three conditions, and the third is the one that changed. A row must be
// included, must carry a catalogue product, AND must be a match the engine
// settled or the vendor made by hand. A row sitting at 31% with a suggested
// product used to satisfy the first two and was written on commit exactly like
// a barcode hit — which is how a review queue became an import.
//
// The filter is here rather than in the commit loop on purpose: this is the
// only query that feeds the writer, so there is one place a row can enter the
// catalogue from and one predicate deciding it.
func (r *Repository) StagedRowsForCommit(ctx context.Context, importID int64) ([]*ingest.RowOutcome, error) {
	var out []*ingest.RowOutcome
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			SELECT r.id, r.source_row, r.outcome, r.match_level, r.match_score, r.product_id,
			       COALESCE(p.name->>'ar', p.name->>'en', ''), COALESCE(p.sku, ''),
			       r.variant_id, r.display_name, r.source_code, r.custom_variant_name,
			       r.is_excluded, r.is_manually_matched, r.payload, r.candidates, r.issues, r.message
			FROM ingest.catalog_import_rows r
			LEFT JOIN catalog.products p ON p.id = r.product_id
			WHERE r.import_id = $1
			  AND r.is_excluded = false
			  AND (
			       (r.product_id IS NOT NULL AND r.product_id > 0
			        AND (r.is_manually_matched OR r.match_level IN ('barcode', 'code', 'exact', 'strong')))
			       OR (r.variant_id IS NOT NULL AND r.variant_id > 0)
			      )
			ORDER BY r.source_row`, importID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var o ingest.RowOutcome
			var payload, candidates, issues []byte
			if err := rows.Scan(&o.ID, &o.SourceRow, &o.Outcome, &o.MatchLevel,
				&o.MatchScore, &o.ProductID, &o.MatchedProductName, &o.MatchedProductSKU,
				&o.VariantID, &o.DisplayName, &o.SourceCode, &o.CustomVariantName,
				&o.IsExcluded, &o.IsManuallyMatched, &payload, &candidates, &issues, &o.Message); err != nil {
				return err
			}
			_ = json.Unmarshal(payload, &o.Payload)
			_ = json.Unmarshal(candidates, &o.Candidates)
			_ = json.Unmarshal(issues, &o.Issues)
			out = append(out, &o)
		}
		return rows.Err()
	})
	return out, err
}

// UpdateCommittedRows updates rows after final execution.
func (r *Repository) UpdateCommittedRows(ctx context.Context, importID int64, rows []ingest.RowOutcome) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		for _, row := range rows {
			if _, err := tx.Exec(txCtx, `
				UPDATE ingest.catalog_import_rows
				SET outcome = $3, variant_id = $4, message = $5
				WHERE id = $1 AND import_id = $2`,
				row.ID, importID, row.Outcome, row.VariantID, row.Message); err != nil {
				return err
			}
		}
		return nil
	})
}

func trimTo(s string, limit int) string {
	s = sheet.CleanCell(s)
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	return string(r[:limit])
}

// MentionedRows lists the catalogue product and the codes every included row of
// an import refers to, whether or not the vendor confirmed the match.
//
// The barcode is read out of the stored payload rather than a column, because
// that is where the reader put it: the staging table keeps the supplier's code
// as its own column and everything else inside the parsed row.
func (r *Repository) MentionedRows(ctx context.Context, importID int64) ([]ingest.RowMention, error) {
	var out []ingest.RowMention
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			SELECT COALESCE(product_id, 0), COALESCE(source_code, ''),
			       COALESCE(payload->>'barcode', '')
			FROM ingest.catalog_import_rows
			WHERE import_id = $1 AND is_excluded = false`, importID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var m ingest.RowMention
			if err := rows.Scan(&m.ProductID, &m.SourceCode, &m.Barcode); err != nil {
				return err
			}
			out = append(out, m)
		}
		return rows.Err()
	})
	return out, err
}
