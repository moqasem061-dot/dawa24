package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

const fileColumns = `id, public_id, organization_id, user_id, supplier_name, original_filename, COALESCE(storage_key, ''), COALESCE(mime_type, ''), size_bytes, row_count, status, COALESCE(mapping_config, '{}'::jsonb), archived_at, COALESCE(archive_reason, ''), COALESCE(error_message, ''), is_temp_warehouse, COALESCE(visibility, 'private'), created_at, updated_at, deleted_at`

func scanFile(row pgx.Row) (*compare.CompareFile, error) {
	var f compare.CompareFile
	var statusStr string
	var mappingBytes []byte
	if err := row.Scan(
		&f.ID, &f.PublicID, &f.OrganizationID, &f.UserID,
		&f.SupplierName, &f.OriginalFilename, &f.StorageKey, &f.MIMEType,
		&f.SizeBytes, &f.RowCount, &statusStr, &mappingBytes,
		&f.ArchivedAt, &f.ArchiveReason, &f.ErrorMessage, &f.IsTempWarehouse, &f.Visibility,
		&f.CreatedAt, &f.UpdatedAt, &f.DeletedAt,
	); err != nil {
		return nil, err
	}
	f.Status = compare.CompareFileStatus(statusStr)
	if len(mappingBytes) > 0 {
		_ = json.Unmarshal(mappingBytes, &f.MappingConfig)
	}
	return &f, nil
}

func (r *Repository) CreateFile(ctx context.Context, f *compare.CompareFile) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO compare.files (
				organization_id, user_id, supplier_name, original_filename, storage_key,
				mime_type, size_bytes, row_count, status, mapping_config, error_message, is_temp_warehouse, visibility
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
			RETURNING id, public_id, created_at, updated_at;
		`
		mappingJSON, _ := json.Marshal(f.MappingConfig)
		if f.Status == "" {
			f.Status = compare.FileUploaded
		}
		if f.Visibility == "" {
			f.Visibility = compare.VisibilityPrivate
		}
		return tx.QueryRow(txCtx, query,
			f.OrganizationID, f.UserID, f.SupplierName, f.OriginalFilename, f.StorageKey,
			f.MIMEType, f.SizeBytes, f.RowCount, string(f.Status), mappingJSON, f.ErrorMessage, f.IsTempWarehouse, f.Visibility,
		).Scan(&f.ID, &f.PublicID, &f.CreatedAt, &f.UpdatedAt)
	})
}

func (r *Repository) GetFileByID(ctx context.Context, id int64) (*compare.CompareFile, error) {
	var f *compare.CompareFile
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT ` + fileColumns + ` FROM compare.files WHERE id = $1 AND deleted_at IS NULL;`
		var err error
		f, err = scanFile(tx.QueryRow(txCtx, query, id))
		if err != nil {
			if err == pgx.ErrNoRows {
				return apperr.NotFound("compare file")
			}
			return err
		}
		return nil
	})
	return f, err
}

func (r *Repository) GetFileByPublicID(ctx context.Context, publicID string) (*compare.CompareFile, error) {
	var f *compare.CompareFile
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT ` + fileColumns + ` FROM compare.files WHERE public_id = $1::uuid AND deleted_at IS NULL;`
		var err error
		f, err = scanFile(tx.QueryRow(txCtx, query, publicID))
		if err != nil {
			if err == pgx.ErrNoRows {
				return apperr.NotFound("compare file")
			}
			return err
		}
		return nil
	})
	return f, err
}

func (r *Repository) ListFiles(ctx context.Context, userID int64, orgID *int64, status *compare.CompareFileStatus) ([]*compare.CompareFile, error) {
	var list []*compare.CompareFile
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT ` + fileColumns + `
			FROM compare.files
			WHERE deleted_at IS NULL
			  AND is_temp_warehouse = FALSE
			  AND (
			      ($1::text IS NULL AND status != 'archived')
			      OR ($1::text IS NOT NULL AND status = $1)
			  )
			  AND (
			      ($2::bigint IS NOT NULL AND (organization_id = $2 OR (organization_id IS NULL AND user_id = $3)))
			      OR ($2::bigint IS NULL AND user_id = $3)
			  )
			ORDER BY created_at DESC;
		`
		var statusStr *string
		if status != nil {
			s := string(*status)
			statusStr = &s
		}
		rows, err := tx.Query(txCtx, query, statusStr, orgID, userID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			f, err := scanFile(rows)
			if err != nil {
				return err
			}
			list = append(list, f)
		}
		return rows.Err()
	})
	return list, err
}

func (r *Repository) ListAllFiles(ctx context.Context, search string, status *compare.CompareFileStatus) ([]*compare.CompareFile, error) {
	var list []*compare.CompareFile
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var whereClauses []string
		var args []any
		argIdx := 1

		whereClauses = append(whereClauses, "deleted_at IS NULL")
		whereClauses = append(whereClauses, "is_temp_warehouse = TRUE")

		if status != nil {
			whereClauses = append(whereClauses, fmt.Sprintf("status = $%d", argIdx))
			args = append(args, string(*status))
			argIdx++
		}

		if search != "" {
			q := "%" + strings.TrimSpace(search) + "%"
			whereClauses = append(whereClauses, fmt.Sprintf("(supplier_name ILIKE $%d OR original_filename ILIKE $%d)", argIdx, argIdx))
			args = append(args, q)
			argIdx++
		}

		sql := `SELECT ` + fileColumns + ` FROM compare.files WHERE ` + strings.Join(whereClauses, " AND ") + ` ORDER BY created_at DESC;`
		rows, err := tx.Query(txCtx, sql, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			f, err := scanFile(rows)
			if err != nil {
				return err
			}
			list = append(list, f)
		}
		return rows.Err()
	})
	return list, err
}

func (r *Repository) CountActiveFiles(ctx context.Context, userID int64, orgID *int64) (int, error) {
	var count int
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT COUNT(*)
			FROM compare.files
			WHERE deleted_at IS NULL
			  AND status != 'archived'
			  AND is_temp_warehouse = FALSE
			  AND (
			      ($1::bigint IS NOT NULL AND (organization_id = $1 OR (organization_id IS NULL AND user_id = $2)))
			      OR ($1::bigint IS NULL AND user_id = $2)
			  );
		`
		return tx.QueryRow(txCtx, query, orgID, userID).Scan(&count)
	})
	return count, err
}

func (r *Repository) UpdateFile(ctx context.Context, f *compare.CompareFile) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE compare.files
			SET supplier_name = $2, row_count = $3, status = $4, mapping_config = $5,
			    archived_at = $6, archive_reason = $7, error_message = $8, updated_at = now()
			WHERE id = $1 AND deleted_at IS NULL;
		`
		mappingJSON, _ := json.Marshal(f.MappingConfig)
		res, err := tx.Exec(txCtx, query,
			f.ID, f.SupplierName, f.RowCount, string(f.Status), mappingJSON,
			f.ArchivedAt, f.ArchiveReason, f.ErrorMessage,
		)
		if err != nil {
			return err
		}
		if res.RowsAffected() == 0 {
			return apperr.NotFound("compare file")
		}
		return nil
	})
}

func (r *Repository) RenameFile(ctx context.Context, id int64, newSupplierName string) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		res, err := tx.Exec(txCtx, `UPDATE compare.files SET supplier_name = $2, updated_at = now() WHERE id = $1 AND deleted_at IS NULL;`, id, newSupplierName)
		if err != nil {
			return err
		}
		if res.RowsAffected() == 0 {
			return apperr.NotFound("compare file")
		}
		return nil
	})
}

// ArchiveOldestFiles implements the retention policy: archives oldest active files to maintain max quota limit (Laravel parity).
func (r *Repository) ArchiveOldestFiles(ctx context.Context, userID int64, orgID *int64, keepCount int, reason string) ([]string, error) {
	var archivedNames []string
	err := r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			WITH active AS (
				SELECT id, supplier_name, ROW_NUMBER() OVER (ORDER BY updated_at ASC) AS rn,
				       COUNT(*) OVER () AS total_active
				FROM compare.files
				WHERE deleted_at IS NULL
				  AND status != 'archived'
				  AND is_temp_warehouse = FALSE
				  AND (
				      ($1::bigint IS NOT NULL AND (organization_id = $1 OR (organization_id IS NULL AND user_id = $2)))
				      OR ($1::bigint IS NULL AND user_id = $2)
				  )
			),
			to_archive AS (
				SELECT id, supplier_name
				FROM active
				WHERE total_active > $3 AND rn <= (total_active - $3)
			)
			UPDATE compare.files f
			SET status = 'archived',
			    supplier_name = f.supplier_name || ' - مؤرشف ' || (
			        SELECT COUNT(*) + 1 
			        FROM compare.files f2 
			        WHERE f2.status = 'archived' 
			          AND f2.supplier_name LIKE f.supplier_name || ' - مؤرشف %'
			    ),
			    archived_at = now(),
			    archive_reason = $4,
			    updated_at = now()
			FROM to_archive ta
			WHERE f.id = ta.id
			RETURNING ta.supplier_name;
		`
		rows, err := tx.Query(txCtx, query, orgID, userID, keepCount, reason)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				return err
			}
			archivedNames = append(archivedNames, name)
		}
		return rows.Err()
	})
	return archivedNames, err
}

// ArchiveActiveFiles soft-archives all currently active compare files for a user/organization.
// This replaces previous comparisons on bulk upload, while keeping historical records
// fully preserved in the database for Super Admin and review.
func (r *Repository) ArchiveActiveFiles(ctx context.Context, userID int64, orgID *int64, reason string) ([]string, error) {
	var archivedNames []string
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE compare.files
			SET status = 'archived',
			    archived_at = now(),
			    archive_reason = $3,
			    updated_at = now()
			WHERE deleted_at IS NULL
			  AND status != 'archived'
			  AND is_temp_warehouse = FALSE
			  AND (
			      user_id = $2
			      OR ($1::bigint IS NOT NULL AND organization_id = $1)
			  )
			RETURNING supplier_name;
		`
		rows, err := tx.Query(txCtx, query, orgID, userID, reason)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				return err
			}
			archivedNames = append(archivedNames, name)
		}
		return rows.Err()
	})
	return archivedNames, err
}

func (r *Repository) ArchiveFile(ctx context.Context, id int64, reason string) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		now := time.Now().UTC()
		res, err := tx.Exec(txCtx, `UPDATE compare.files SET status = 'archived', archived_at = $2, archive_reason = $3, updated_at = $2 WHERE id = $1 AND deleted_at IS NULL;`, id, now, reason)
		if err != nil {
			return err
		}
		if res.RowsAffected() == 0 {
			return apperr.NotFound("compare file")
		}
		return nil
	})
}

func (r *Repository) UnarchiveFile(ctx context.Context, id int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		res, err := tx.Exec(txCtx, `UPDATE compare.files SET status = 'ready', archived_at = NULL, archive_reason = NULL, updated_at = now() WHERE id = $1 AND deleted_at IS NULL;`, id)
		if err != nil {
			return err
		}
		if res.RowsAffected() == 0 {
			return apperr.NotFound("compare file")
		}
		return nil
	})
}

func (r *Repository) DeleteFile(ctx context.Context, id int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		res, err := tx.Exec(txCtx, `UPDATE compare.files SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL;`, id)
		if err != nil {
			return err
		}
		if res.RowsAffected() == 0 {
			return apperr.NotFound("compare file")
		}
		return nil
	})
}

// ---------------------------------------------------------------------------
// File Rows
// ---------------------------------------------------------------------------

const rowColumns = `id, file_id, organization_id, row_number, raw_name, normalized_name, COALESCE(sku, ''), price, discount, price_after_discount, matched_product_id, match_confidence, match_method, COALESCE(meta, '{}'::jsonb), created_at`

func toDBMatchMethod(m compare.MatchMethod) string {
	switch string(m) {
	case "exact", "exact_name", "direct_id":
		return "exact"
	case "sku", "barcode":
		return "sku"
	case "normalized", "partial":
		return "normalized"
	case "fuzzy":
		return "fuzzy"
	case "ai":
		return "ai"
	case "manual", "saved_mapping":
		return "manual"
	default:
		return "none"
	}
}
