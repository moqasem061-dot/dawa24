package postgres

// Writing a whole matching run.
//
// A supplier price list is tens of thousands of rows and the matching stage
// resolves all of them in one pass over an in-memory index — a few seconds of
// arithmetic. Writing the result one row at a time would put a round trip
// behind each of those, which does not make the stage slower so much as make it
// a different kind of operation: a minute of database chatter instead of a few
// seconds of CPU.
//
// So the run is written in chunks of a few hundred through one UPDATE … FROM
// (VALUES …) per chunk, scoped by file_id as well as by row id so a stale or
// forged row id from another file matches nothing.

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
)

// matchWriteChunk is how many rows go in one statement.
//
// Postgres accepts far more parameters than this; the limit is chosen so a
// failure retries a small amount of work and so one statement's parameter list
// stays inside what the driver will happily marshal.
const matchWriteChunk = 500

// BulkUpdateFileRowMatches writes a matching run onto its file's rows.
func (r *Repository) BulkUpdateFileRowMatches(
	ctx context.Context, fileID int64, matches []compare.RowMatch,
) error {
	if len(matches) == 0 {
		return nil
	}
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		for start := 0; start < len(matches); start += matchWriteChunk {
			end := start + matchWriteChunk
			if end > len(matches) {
				end = len(matches)
			}
			if err := writeMatchChunk(txCtx, tx, fileID, matches[start:end]); err != nil {
				return err
			}
		}
		return nil
	})
}

func writeMatchChunk(
	ctx context.Context, tx pgx.Tx, fileID int64, chunk []compare.RowMatch,
) error {
	const cols = 4
	values := make([]string, 0, len(chunk))
	args := make([]any, 0, len(chunk)*cols+1)
	args = append(args, fileID)
	for i, m := range chunk {
		base := i*cols + 1
		values = append(values, fmt.Sprintf("($%d::bigint,$%d::bigint,$%d::text,$%d::numeric)",
			base+1, base+2, base+3, base+4))
		args = append(args, m.RowID, m.ProductID, toDBMatchMethod(m.Method), m.Confidence)
	}
	_, err := tx.Exec(ctx, `
		UPDATE compare.file_rows AS t
		SET matched_product_id = v.product_id,
		    match_method       = v.method,
		    match_confidence   = v.confidence
		FROM (VALUES `+strings.Join(values, ",")+`) AS v(row_id, product_id, method, confidence)
		WHERE t.file_id = $1 AND t.id = v.row_id;`, args...)
	if err != nil {
		return fmt.Errorf("compare postgres: write match chunk: %w", err)
	}
	return nil
}
