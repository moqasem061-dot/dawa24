package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/ingest"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// GetImportRowByID retrieves a single staged row.
func (r *Repository) GetImportRowByID(ctx context.Context, id int64) (*ingest.ImportRow, error) {
	var ir ingest.ImportRow
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT ir.id, ir.session_id, ir.organization_id, ir.row_number, ir.raw_data,
			       ir.normalized_name, ir.matched_product_id, ir.similarity_score,
			       ir.confidence_level, ir.match_reason, ir.candidate_matches,
			       ir.is_approved, ir.import_action, ir.error_details, ir.status, ir.created_at,
			       COALESCE(p.name->>'ar', p.name->>'en', '') AS matched_name,
			       COALESCE(p.sku, '') AS matched_sku
			FROM ingest.import_rows ir
			LEFT JOIN catalog.products p ON p.id = ir.matched_product_id
			WHERE ir.id = $1;
		`
		var rawJSON []byte
		var candidatesJSON []byte
		var normName, matchReason, errorDetails *string
		var confLevel, matchedName, matchedSKU string
		err := tx.QueryRow(txCtx, query, id).Scan(
			&ir.ID, &ir.SessionID, &ir.OrganizationID, &ir.RowNumber, &rawJSON,
			&normName, &ir.MatchedProductID, &ir.SimilarityScore,
			&confLevel, &matchReason, &candidatesJSON,
			&ir.IsApproved, &ir.ImportAction, &errorDetails, &ir.Status, &ir.CreatedAt,
			&matchedName, &matchedSKU,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("import_row")
			}
			return err
		}
		if normName != nil {
			ir.NormalizedName = *normName
		}
		if matchReason != nil {
			ir.MatchReason = *matchReason
		}
		if errorDetails != nil {
			ir.ErrorDetails = *errorDetails
		}
		ir.ConfidenceLevel = ingest.ConfidenceLevel(confLevel)
		ir.MatchedProdName = matchedName
		ir.MatchedProdSKU = matchedSKU

		if len(rawJSON) > 0 {
			_ = json.Unmarshal(rawJSON, &ir.RawData)
		}
		if len(candidatesJSON) > 0 {
			_ = json.Unmarshal(candidatesJSON, &ir.CandidateMatches)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &ir, nil
}

// ListImportSessions returns sessions for an organization.
func (r *Repository) ListImportSessions(ctx context.Context, orgID int64, limit, offset int) ([]*ingest.ImportSession, error) {
	var list []*ingest.ImportSession
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, organization_id, file_upload_id, warehouse_id, import_mode,
			       enable_ai_matching, enable_savings_matching, status, column_mapping,
			       min_similarity_score, total_rows, processed_rows, matched_rows,
			       review_rows, unmatched_rows, error_message, started_at, completed_at,
			       created_at, updated_at
			FROM ingest.import_sessions
			WHERE organization_id = $1
			ORDER BY id DESC
			LIMIT $2 OFFSET $3;
		`
		if limit <= 0 || limit > 100 {
			limit = 20
		}
		rows, err := tx.Query(txCtx, query, orgID, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var s ingest.ImportSession
			var statusStr, modeStr string
			var mappingJSON []byte
			var errMsg *string
			if err := rows.Scan(
				&s.ID, &s.PublicID, &s.OrganizationID, &s.FileUploadID, &s.WarehouseID, &modeStr,
				&s.EnableAIMatching, &s.EnableSavingsMatching, &statusStr, &mappingJSON,
				&s.MinSimilarityScore, &s.TotalRows, &s.ProcessedRows, &s.MatchedRows,
				&s.ReviewRows, &s.UnmatchedRows, &errMsg, &s.StartedAt, &s.CompletedAt,
				&s.CreatedAt, &s.UpdatedAt,
			); err != nil {
				return err
			}
			s.Status = ingest.SessionStatus(statusStr)
			s.ImportMode = ingest.ImportMode(modeStr)
			if errMsg != nil {
				s.ErrorMessage = *errMsg
			}
			if len(mappingJSON) > 0 {
				_ = json.Unmarshal(mappingJSON, &s.ColumnMapping)
			}
			list = append(list, &s)
		}
		return rows.Err()
	})
	return list, err
}

// UpdateColumnMapping modifies column definitions.
func (r *Repository) UpdateColumnMapping(ctx context.Context, id int64, mapping map[string]string) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		mappingJSON, err := json.Marshal(mapping)
		if err != nil {
			return err
		}
		_, err = tx.Exec(txCtx, `UPDATE ingest.import_sessions SET column_mapping = $1, updated_at = now() WHERE id = $2;`, mappingJSON, id)
		return err
	})
}

// UpdateSessionStatus changes the status of a session.
func (r *Repository) UpdateSessionStatus(ctx context.Context, id int64, status ingest.SessionStatus) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `UPDATE ingest.import_sessions SET status = $1, updated_at = now() WHERE id = $2;`, string(status), id)
		return err
	})
}

// UpdateImportRowMatch updates matching product result on a staged row.
func (r *Repository) UpdateImportRowMatch(ctx context.Context, rowID int64, matchedProductID *int64, score float64, status string) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE ingest.import_rows
			SET matched_product_id = $2, similarity_score = $3, status = $4
			WHERE id = $1;
		`
		_, err := tx.Exec(txCtx, query, rowID, matchedProductID, score, status)
		return err
	})
}

// UpdateImportRowMatchDetailed updates all matching details, confidence level, reason, and candidate list.
func (r *Repository) UpdateImportRowMatchDetailed(
	ctx context.Context,
	rowID int64,
	matchedProductID *int64,
	score float64,
	confLevel ingest.ConfidenceLevel,
	reason string,
	candidates []ingest.CandidateMatch,
	isApproved bool,
	status string,
) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		candidatesJSON, err := json.Marshal(candidates)
		if err != nil {
			candidatesJSON = []byte("[]")
		}

		query := `
			UPDATE ingest.import_rows
			SET matched_product_id = $2, similarity_score = $3, confidence_level = $4,
			    match_reason = $5, candidate_matches = $6, is_approved = $7, status = $8
			WHERE id = $1;
		`
		_, err = tx.Exec(txCtx, query, rowID, matchedProductID, score, string(confLevel), reason, candidatesJSON, isApproved, status)
		return err
	})
}

// UpdateImportRowApproval updates whether a row is approved for final import.
func (r *Repository) UpdateImportRowApproval(ctx context.Context, rowID int64, isApproved bool) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `UPDATE ingest.import_rows SET is_approved = $2 WHERE id = $1;`, rowID, isApproved)
		return err
	})
}

// UpdateImportRowAction records the execution outcome of an imported row.
func (r *Repository) UpdateImportRowAction(ctx context.Context, rowID int64, action, errorDetails string) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE ingest.import_rows
			SET import_action = $2, error_details = $3
			WHERE id = $1;
		`
		_, err := tx.Exec(txCtx, query, rowID, action, errorDetails)
		return err
	})
}

// BatchUpdateImportRowMatches updates matching results for multiple staged rows in a single pipelined transaction.
func (r *Repository) BatchUpdateImportRowMatches(ctx context.Context, updates []ingest.RowMatchUpdate) error {
	if len(updates) == 0 {
		return nil
	}
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		batch := &pgx.Batch{}
		const query = `
			UPDATE ingest.import_rows
			SET matched_product_id = $2, similarity_score = $3, confidence_level = $4,
			    match_reason = $5, candidate_matches = $6, is_approved = $7, status = $8
			WHERE id = $1;
		`
		for _, u := range updates {
			candidatesJSON, err := json.Marshal(u.Candidates)
			if err != nil {
				candidatesJSON = []byte("[]")
			}
			conf := string(u.ConfidenceLevel)
			if conf == "" {
				conf = string(ingest.ConfidenceUnmatched)
			}
			batch.Queue(query,
				u.RowID, u.MatchedProductID, u.Score, conf,
				u.MatchReason, candidatesJSON, u.IsApproved, u.Status,
			)
		}

		br := tx.SendBatch(txCtx, batch)
		defer br.Close()

		for i := 0; i < len(updates); i++ {
			if _, err := br.Exec(); err != nil {
				return fmt.Errorf("ingest postgres: batch update match row %d: %w", updates[i].RowID, err)
			}
		}
		return nil
	})
}

// BatchUpdateImportRowActions records execution outcomes for multiple committed rows in a single pipelined transaction.
func (r *Repository) BatchUpdateImportRowActions(ctx context.Context, updates []ingest.RowActionUpdate) error {
	if len(updates) == 0 {
		return nil
	}
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		batch := &pgx.Batch{}
		const query = `
			UPDATE ingest.import_rows
			SET import_action = $2, error_details = $3
			WHERE id = $1;
		`
		for _, u := range updates {
			batch.Queue(query, u.RowID, u.ImportAction, u.ErrorDetails)
		}

		br := tx.SendBatch(txCtx, batch)
		defer br.Close()

		for i := 0; i < len(updates); i++ {
			if _, err := br.Exec(); err != nil {
				return fmt.Errorf("ingest postgres: batch update action row %d: %w", updates[i].RowID, err)
			}
		}
		return nil
	})
}
