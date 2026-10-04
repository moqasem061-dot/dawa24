package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// ReplaceFileRows serializes replacements and rejects work prepared from stale metadata.
func (r *Repository) ReplaceFileRows(ctx context.Context, file *compare.CompareFile, rows []*compare.CompareFileRow) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var updated time.Time
		err := tx.QueryRow(txCtx, `SELECT updated_at FROM compare.files WHERE id=$1 AND deleted_at IS NULL AND archived_at IS NULL FOR UPDATE`, file.ID).Scan(&updated)
		if err != nil {
			return err
		}
		if !updated.Equal(file.UpdatedAt) {
			return fmt.Errorf("compare: file changed during matching; retry the import")
		}
		mapping, err := json.Marshal(file.MappingConfig)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM compare.file_rows WHERE file_id=$1`, file.ID); err != nil {
			return err
		}
		if err := insertFileRows(txCtx, tx, rows); err != nil {
			return err
		}
		return tx.QueryRow(txCtx, `UPDATE compare.files SET row_count=$2, status='ready', mapping_config=$3, error_message=NULL, updated_at=now() WHERE id=$1 RETURNING updated_at`, file.ID, len(rows), mapping).Scan(&file.UpdatedAt)
	})
}

func (r *Repository) FailFileStaging(ctx context.Context, file *compare.CompareFile, message string) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `UPDATE compare.files SET status='failed', error_message=$3, updated_at=now() WHERE id=$1 AND updated_at=$2 AND status='processing' AND archived_at IS NULL AND deleted_at IS NULL`, file.ID, file.UpdatedAt, message)
		return err
	})
}
