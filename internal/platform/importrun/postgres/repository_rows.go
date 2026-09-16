package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/platform/importrun"
)

// ──────────────────────────────────────────────────────────────────────
// SQL fragments and scan helpers.
// ──────────────────────────────────────────────────────────────────────

const runSelectSQL = `
SELECT
  r.id, r.public_id::text, r.organization_id, r.user_id,
  r.kind, r.audience, r.filename,
  r.state, r.phase, r.percent,
  r.total_rows, r.processed_rows,
  r.payload, r.result, r.error_message,
  r.river_job_id, r.started_at, r.finished_at,
  r.created_at, r.updated_at
FROM platform.import_runs r`

func scanRun(row pgx.Row, run *importrun.Run) error {
	return row.Scan(
		&run.ID, &run.PublicID, &run.OrganizationID, &run.UserID,
		&run.Kind, &run.Audience, &run.Filename,
		&run.State, &run.Phase, &run.Percent,
		&run.TotalRows, &run.ProcessedRows,
		&run.Payload, &run.Result, &run.ErrorMessage,
		&run.RiverJobID, &run.StartedAt, &run.FinishedAt,
		&run.CreatedAt, &run.UpdatedAt,
	)
}

func scanRunRows(rows pgx.Rows, run *importrun.Run) error {
	return rows.Scan(
		&run.ID, &run.PublicID, &run.OrganizationID, &run.UserID,
		&run.Kind, &run.Audience, &run.Filename,
		&run.State, &run.Phase, &run.Percent,
		&run.TotalRows, &run.ProcessedRows,
		&run.Payload, &run.Result, &run.ErrorMessage,
		&run.RiverJobID, &run.StartedAt, &run.FinishedAt,
		&run.CreatedAt, &run.UpdatedAt,
	)
}

// ──────────────────────────────────────────────────────────────────────
// Row operations (bulk insert, list, update).
// ──────────────────────────────────────────────────────────────────────

// InsertRows bulk-inserts staged rows for a run.
//
// InLongTx, not InTx. This is the write that stages a whole spreadsheet — a
// vendor's price list is routinely twenty-five thousand rows — and the pool's
// ordinary statement_timeout is thirty seconds, which is the right ceiling for
// a web request and the wrong one for this. When the import ran in the web
// process (cmd/server runs the stage worker inline) a large file was cancelled
// by Postgres partway through, and the vendor was shown an import that failed
// on "statement timeout" naming nothing they could act on.
func (r *Repository) InsertRows(ctx context.Context, runID int64, rows []importrun.Row) error {
	if len(rows) == 0 {
		return nil
	}

	return r.db.InLongTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		// Build a multi-row INSERT in batches of 500 to keep the
		// statement size manageable.
		const batchSize = 500
		for start := 0; start < len(rows); start += batchSize {
			end := start + batchSize
			if end > len(rows) {
				end = len(rows)
			}
			batch := rows[start:end]

			var sb strings.Builder
			sb.WriteString(`
				INSERT INTO platform.import_run_rows
					(run_id, row_number, data, included, matched_product_id)
				VALUES `)

			args := make([]any, 0, len(batch)*5)
			for i, row := range batch {
				if i > 0 {
					sb.WriteString(", ")
				}
				base := i*5 + 1
				fmt.Fprintf(&sb, "($%d, $%d, $%d, $%d, $%d)",
					base, base+1, base+2, base+3, base+4)

				data := row.Data
				if data == nil {
					data = json.RawMessage(`{}`)
				}
				args = append(args, runID, row.RowNumber, data, row.Included, row.MatchedProductID)
			}

			sb.WriteString(" ON CONFLICT (run_id, row_number) DO NOTHING")

			if _, err := tx.Exec(txCtx, sb.String(), args...); err != nil {
				return fmt.Errorf("importrun: insert rows batch at offset %d: %w", start, err)
			}
		}
		return nil
	})
}

// ListRows returns paginated rows for a run.
func (r *Repository) ListRows(ctx context.Context, runID int64, onlyIncluded bool, limit, offset int) ([]importrun.Row, int, error) {
	var result []importrun.Row
	var total int

	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		where := "WHERE run_id = $1"
		args := []any{runID}
		if onlyIncluded {
			where += " AND included = true"
		}

		if err := tx.QueryRow(txCtx,
			"SELECT COUNT(*) FROM platform.import_run_rows "+where, args...,
		).Scan(&total); err != nil {
			return err
		}

		args = append(args, limit, offset)
		rows, err := tx.Query(txCtx, fmt.Sprintf(`
			SELECT id, run_id, row_number, data, included, matched_product_id, created_at, updated_at
			FROM platform.import_run_rows
			%s
			ORDER BY row_number ASC
			LIMIT $2 OFFSET $3
		`, where), args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var row importrun.Row
			if err := rows.Scan(
				&row.ID, &row.RunID, &row.RowNumber,
				&row.Data, &row.Included, &row.MatchedProductID,
				&row.CreatedAt, &row.UpdatedAt,
			); err != nil {
				return err
			}
			result = append(result, row)
		}
		return rows.Err()
	})
	return result, total, err
}

// UpdateRow updates a single row.
func (r *Repository) UpdateRow(ctx context.Context, rowID int64, data json.RawMessage, included *bool, matchedProductID *int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		sets := []string{}
		args := []any{rowID}
		n := 2

		if data != nil {
			sets = append(sets, fmt.Sprintf("data = $%d", n))
			args = append(args, data)
			n++
		}
		if included != nil {
			sets = append(sets, fmt.Sprintf("included = $%d", n))
			args = append(args, *included)
			n++
		}
		if matchedProductID != nil {
			sets = append(sets, fmt.Sprintf("matched_product_id = $%d", n))
			args = append(args, *matchedProductID)
			n++
		}

		if len(sets) == 0 {
			return nil
		}

		_, err := tx.Exec(txCtx, fmt.Sprintf(
			"UPDATE platform.import_run_rows SET %s WHERE id = $1",
			strings.Join(sets, ", "),
		), args...)
		return err
	})
}

// RecoverStaleRuns marks runs stuck in processing/committing for > 30 min
// as failed so their owners can retry.
func (r *Repository) RecoverStaleRuns(ctx context.Context) (int, error) {
	var count int
	err := r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `
			UPDATE platform.import_runs
			SET state = 'failed',
			    error_message = 'import timed out (process restart or crash)',
			    finished_at = now()
			WHERE state IN ('processing', 'committing')
			  AND updated_at < now() - interval '30 minutes'
		`)
		if err != nil {
			return err
		}
		count = int(tag.RowsAffected())
		return nil
	})
	return count, err
}
