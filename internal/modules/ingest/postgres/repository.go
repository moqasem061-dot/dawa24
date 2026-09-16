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

// Repository implements ingest.Repository using PostgreSQL.
type Repository struct {
	db *database.DB
}

// NewRepository creates a new ingest PostgreSQL repository.
func NewRepository(db *database.DB) *Repository {
	return &Repository{db: db}
}

const DocumentTypeImportFile = "import_file"

// CreateFileUpload records a pointer to a stored import file.
func (r *Repository) CreateFileUpload(ctx context.Context, f *ingest.FileUpload) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO platform_admin.documents (
				organization_id, user_id, title, document_type, storage_key,
				original_name, file_url, status, mime_type, size_bytes
			) VALUES ($1, $2, $3, $4, $5, $6, '', 'pending', $7, $8)
			RETURNING id, public_id, created_at;
		`
		return tx.QueryRow(txCtx, query,
			f.OrganizationID, f.UserID, f.Filename, DocumentTypeImportFile,
			f.StorageKey, f.Filename, f.MimeType, f.FileSizeBytes,
		).Scan(&f.ID, &f.PublicID, &f.CreatedAt)
	})
}

// GetFileUploadByID retrieves an upload record by ID.
func (r *Repository) GetFileUploadByID(ctx context.Context, id int64) (*ingest.FileUpload, error) {
	var f ingest.FileUpload
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, organization_id, user_id, original_name, storage_key, size_bytes, mime_type, created_at
			FROM platform_admin.documents
			WHERE id = $1 AND document_type = $2;
		`
		return tx.QueryRow(txCtx, query, id, DocumentTypeImportFile).Scan(
			&f.ID, &f.PublicID, &f.OrganizationID, &f.UserID, &f.Filename,
			&f.StorageKey, &f.FileSizeBytes, &f.MimeType, &f.CreatedAt,
		)
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil, apperr.NotFound("file_upload")
		}
		return nil, err
	}
	return &f, nil
}

// CreateImportSession creates a session for tracking import execution.
func (r *Repository) CreateImportSession(ctx context.Context, s *ingest.ImportSession) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		mappingJSON, err := json.Marshal(s.ColumnMapping)
		if err != nil {
			return err
		}

		mode := string(s.ImportMode)
		if mode == "" {
			mode = string(ingest.ModeUpdateAndAdd)
		}

		query := `
			INSERT INTO ingest.import_sessions (
				organization_id, file_upload_id, warehouse_id, import_mode,
				enable_ai_matching, enable_savings_matching,
				status, column_mapping, min_similarity_score, total_rows
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			RETURNING id, public_id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query,
			s.OrganizationID, s.FileUploadID, s.WarehouseID, mode,
			s.EnableAIMatching, s.EnableSavingsMatching,
			string(s.Status), mappingJSON, s.MinSimilarityScore, s.TotalRows,
		).Scan(&s.ID, &s.PublicID, &s.CreatedAt, &s.UpdatedAt)
	})
}

// GetImportSessionByID retrieves an import session.
func (r *Repository) GetImportSessionByID(ctx context.Context, id int64) (*ingest.ImportSession, error) {
	var s ingest.ImportSession
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, organization_id, file_upload_id, warehouse_id, import_mode,
			       enable_ai_matching, enable_savings_matching, status, column_mapping,
			       min_similarity_score, total_rows, processed_rows, matched_rows,
			       review_rows, unmatched_rows, error_message, started_at, completed_at,
			       created_at, updated_at
			FROM ingest.import_sessions
			WHERE id = $1;
		`
		var statusStr, modeStr string
		var mappingJSON []byte
		var errMsg *string
		err := tx.QueryRow(txCtx, query, id).Scan(
			&s.ID, &s.PublicID, &s.OrganizationID, &s.FileUploadID, &s.WarehouseID, &modeStr,
			&s.EnableAIMatching, &s.EnableSavingsMatching, &statusStr, &mappingJSON,
			&s.MinSimilarityScore, &s.TotalRows, &s.ProcessedRows, &s.MatchedRows,
			&s.ReviewRows, &s.UnmatchedRows, &errMsg, &s.StartedAt, &s.CompletedAt,
			&s.CreatedAt, &s.UpdatedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("import_session")
			}
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
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// UpdateImportSessionConfig updates configuration and toggles for an active session.
func (r *Repository) UpdateImportSessionConfig(
	ctx context.Context,
	id int64,
	warehouseID *int64,
	mode ingest.ImportMode,
	aiMatching, savingsMatching bool,
) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE ingest.import_sessions
			SET warehouse_id = $2, import_mode = $3, enable_ai_matching = $4,
			    enable_savings_matching = $5, updated_at = now()
			WHERE id = $1;
		`
		_, err := tx.Exec(txCtx, query, id, warehouseID, string(mode), aiMatching, savingsMatching)
		return err
	})
}

// UpdateImportSessionStats updates all processing counters and lifecycle state.
func (r *Repository) UpdateImportSessionStats(
	ctx context.Context,
	id int64,
	total, processed, matched, review, unmatched int,
	status ingest.SessionStatus,
	errMsg string,
) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE ingest.import_sessions
			SET total_rows = $2, processed_rows = $3, matched_rows = $4,
			    review_rows = $5, unmatched_rows = $6, status = $7,
			    error_message = $8, updated_at = now()
			WHERE id = $1;
		`
		_, err := tx.Exec(txCtx, query, id, total, processed, matched, review, unmatched, string(status), errMsg)
		return err
	})
}

// UpdateImportSessionProgress updates processed count and lifecycle state.
func (r *Repository) UpdateImportSessionProgress(
	ctx context.Context,
	id int64,
	processed, matched int,
	status ingest.SessionStatus,
	errMsg string,
) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE ingest.import_sessions
			SET processed_rows = $2, matched_rows = $3, status = $4, error_message = $5, updated_at = now()
			WHERE id = $1;
		`
		_, err := tx.Exec(txCtx, query, id, processed, matched, string(status), errMsg)
		return err
	})
}

// InsertImportRows writes a batch of staged rows using high-performance pipelined batch execution.
func (r *Repository) InsertImportRows(ctx context.Context, rows []*ingest.ImportRow) error {
	if len(rows) == 0 {
		return nil
	}
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		batch := &pgx.Batch{}
		const query = `
			INSERT INTO ingest.import_rows (
				session_id, organization_id, row_number, raw_data, normalized_name,
				confidence_level, is_approved, import_action, status
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);
		`
		for _, row := range rows {
			rawJSON, err := json.Marshal(row.RawData)
			if err != nil {
				rawJSON = []byte("{}")
			}
			conf := string(row.ConfidenceLevel)
			if conf == "" {
				conf = string(ingest.ConfidenceUnmatched)
			}
			action := row.ImportAction
			if action == "" {
				action = "pending"
			}
			batch.Queue(query,
				row.SessionID, row.OrganizationID, row.RowNumber, rawJSON, row.NormalizedName,
				conf, row.IsApproved, action, row.Status,
			)
		}

		br := tx.SendBatch(txCtx, batch)
		defer br.Close()

		for i := 0; i < len(rows); i++ {
			if _, err := br.Exec(); err != nil {
				return fmt.Errorf("ingest postgres: batch insert row %d: %w", rows[i].RowNumber, err)
			}
		}
		return nil
	})
}

// ListImportRows retrieves staged rows for a session with matched product details.
func (r *Repository) ListImportRows(ctx context.Context, sessionID int64, status string, limit, offset int) ([]*ingest.ImportRow, error) {
	var rowsList []*ingest.ImportRow
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
			WHERE ir.session_id = $1 AND ($2 = '' OR ir.status = $2 OR ir.confidence_level = $2)
			ORDER BY ir.row_number ASC
			LIMIT $3 OFFSET $4;
		`
		if limit <= 0 || limit > 500 {
			limit = 100
		}
		rows, err := tx.Query(txCtx, query, sessionID, status, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var ir ingest.ImportRow
			var rawJSON []byte
			var candidatesJSON []byte
			var normName, matchReason, errorDetails *string
			var confLevel, matchedName, matchedSKU string
			if err := rows.Scan(
				&ir.ID, &ir.SessionID, &ir.OrganizationID, &ir.RowNumber, &rawJSON,
				&normName, &ir.MatchedProductID, &ir.SimilarityScore,
				&confLevel, &matchReason, &candidatesJSON,
				&ir.IsApproved, &ir.ImportAction, &errorDetails, &ir.Status, &ir.CreatedAt,
				&matchedName, &matchedSKU,
			); err != nil {
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
			rowsList = append(rowsList, &ir)
		}
		return rows.Err()
	})
	return rowsList, err
}
