package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// The uploaded workbook, persisted between step 1 and step 2.
//
// See migration 127 for why this is a table rather than a map in process
// memory: a redeploy between upload and mapping used to lose the file, and a
// pharmacy re-uploading a nine-thousand-line workbook because a deployment
// happened is not an acceptable failure.

// SaveRunFile stores the uploaded workbook against the run.
func (r *Repository) SaveRunFile(ctx context.Context, runID, orgID int64, filename string, content []byte) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `
			INSERT INTO smartorder.run_files (run_id, organization_id, filename, content, size_bytes)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (run_id) DO UPDATE SET
				filename = EXCLUDED.filename,
				content = EXCLUDED.content,
				size_bytes = EXCLUDED.size_bytes,
				created_at = now();`,
			runID, orgID, filename, content, len(content))
		return err
	})
}

// GetRunFile returns the stored workbook.
func (r *Repository) GetRunFile(ctx context.Context, runID, orgID int64) ([]byte, string, error) {
	var content []byte
	var filename string
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		err := tx.QueryRow(txCtx, `
			SELECT content, COALESCE(filename, '')
			FROM smartorder.run_files
			WHERE run_id = $1 AND organization_id = $2;`, runID, orgID).Scan(&content, &filename)
		if err == pgx.ErrNoRows {
			return apperr.NotFound("smart_order_file")
		}
		return err
	})
	return content, filename, err
}

// DeleteRunFile drops the workbook once its rows are staged.
//
// Called as soon as the rows exist, because from that point the file has served
// its purpose and keeping it is storage nobody asked for.
func (r *Repository) DeleteRunFile(ctx context.Context, runID, orgID int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx,
			`DELETE FROM smartorder.run_files WHERE run_id = $1 AND organization_id = $2;`, runID, orgID)
		return err
	})
}
