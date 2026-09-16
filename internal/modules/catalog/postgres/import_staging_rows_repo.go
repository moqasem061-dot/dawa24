package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// ReplaceStagingRows swaps in a freshly parsed set of rows.
//
// Reprocessing is normal — the admin corrects a column mapping and runs it
// again — so this is a replace rather than an append, and it uses COPY because
// the alternative at nine thousand rows is nine thousand round trips.
func (r *Repository) ReplaceStagingRows(ctx context.Context, sessionID int64, rows []*catalog.StagingRow) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(txCtx,
			`DELETE FROM catalog.import_staging_rows WHERE session_id = $1`, sessionID); err != nil {
			return fmt.Errorf("catalog postgres: clear staging rows: %w", err)
		}
		if len(rows) == 0 {
			return nil
		}

		source, err := newStagingCopySource(sessionID, rows)
		if err != nil {
			return err
		}

		_, err = tx.CopyFrom(txCtx,
			pgx.Identifier{"catalog", "import_staging_rows"},
			[]string{
				"session_id", "source_row", "block_index", "action", "included",
				"matched_product_id", "match_reason", "payload", "issues", "ai_changes",
				"search_name", "search_code", "has_error", "has_warning", "has_ai",
			},
			source)
		if err != nil {
			return fmt.Errorf("catalog postgres: copy staging rows: %w", err)
		}
		return nil
	})
}

// newStagingCopySource encodes the rows once, up front, so a JSON failure
// surfaces as an ordinary error rather than aborting a COPY midstream.
func newStagingCopySource(sessionID int64, rows []*catalog.StagingRow) (pgx.CopyFromSource, error) {
	values := make([][]any, 0, len(rows))
	for _, row := range rows {
		payload, err := json.Marshal(row.Product)
		if err != nil {
			return nil, fmt.Errorf("catalog postgres: encode staged product (row %d): %w", row.SourceRow, err)
		}
		issues, err := json.Marshal(nonNilIssues(row.Issues))
		if err != nil {
			return nil, fmt.Errorf("catalog postgres: encode staged issues (row %d): %w", row.SourceRow, err)
		}
		changes, err := json.Marshal(nonNilChanges(row.AIChanges))
		if err != nil {
			return nil, fmt.Errorf("catalog postgres: encode staged ai changes (row %d): %w", row.SourceRow, err)
		}

		hasError, hasWarning := false, false
		for _, issue := range row.Issues {
			if issue.Severity == catalog.SeverityError {
				hasError = true
			} else {
				hasWarning = true
			}
		}

		values = append(values, []any{
			sessionID, row.SourceRow, row.Block, string(row.Action), row.Included,
			row.MatchedProductID, string(row.MatchReason),
			string(payload), string(issues), string(changes),
			row.DisplayName(), stagingSearchCode(row),
			hasError, hasWarning, len(row.AIChanges) > 0,
		})
	}
	return pgx.CopyFromRows(values), nil
}

func stagingSearchCode(row *catalog.StagingRow) string {
	if row.Product == nil {
		return ""
	}
	return strings.TrimSpace(row.Product.SKU + " " + row.Product.Barcode)
}

// ListStagingRows reads a page of the review table.
func (r *Repository) ListStagingRows(
	ctx context.Context, sessionID int64, filter catalog.StagingFilter,
) ([]*catalog.StagingRow, int, error) {
	where := []string{"session_id = $1"}
	args := []any{sessionID}

	if filter.Action != "" {
		args = append(args, string(filter.Action))
		where = append(where, fmt.Sprintf("action = $%d", len(args)))
	}
	if filter.OnlyIssues {
		where = append(where, "(has_error OR has_warning)")
	}
	if filter.OnlyAI {
		where = append(where, "has_ai")
	}
	if search := strings.TrimSpace(filter.Search); search != "" {
		args = append(args, "%"+search+"%")
		where = append(where, fmt.Sprintf("(search_name ILIKE $%d OR search_code ILIKE $%d)", len(args), len(args)))
	}
	clause := strings.Join(where, " AND ")

	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	args = append(args, limit, max(filter.Offset, 0))

	var rows []*catalog.StagingRow
	var total int

	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(txCtx,
			`SELECT count(*) FROM catalog.import_staging_rows WHERE `+clause,
			args[:len(args)-2]...).Scan(&total); err != nil {
			return fmt.Errorf("catalog postgres: count staging rows: %w", err)
		}

		// Least certain first.
		//
		// The default used to be the row's position in the administrator's
		// file, which is an order that tells the reviewer nothing: the rows
		// needing a decision are scattered evenly among the ones that do not,
		// so finding them means reading all of them — and this is the import
		// whose mistakes overwrite the entry every pharmacy reads.
		//
		// There is no score column on this table, so the ordering is by how the
		// row was decided, worst class first: anything that failed, then rows a
		// model settled, then a similarity judgement, then a name, and last the
		// identifier hits that are facts rather than opinions.
		query := fmt.Sprintf(stagingRowSelect+`
			WHERE %s
			ORDER BY
			  CASE
			    WHEN r.has_error            THEN 0
			    WHEN r.matched_product_id IS NULL THEN 1
			    WHEN r.match_reason = 'ai'  THEN 2
			    WHEN r.has_warning          THEN 3
			    WHEN r.match_reason = 'similar' THEN 4
			    WHEN r.match_reason = 'name'    THEN 5
			    ELSE 6
			  END,
			  r.source_row
			LIMIT $%d OFFSET $%d`, qualify(clause), len(args)-1, len(args))

		cursor, err := tx.Query(txCtx, query, args...)
		if err != nil {
			return fmt.Errorf("catalog postgres: list staging rows: %w", err)
		}
		defer cursor.Close()

		for cursor.Next() {
			row, err := scanStagingRow(cursor)
			if err != nil {
				return err
			}
			rows = append(rows, row)
		}
		return cursor.Err()
	})
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// GetStagingRow returns one staged row.
func (r *Repository) GetStagingRow(ctx context.Context, sessionID, rowID int64) (*catalog.StagingRow, error) {
	var row *catalog.StagingRow
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		cursor, err := tx.Query(txCtx, stagingRowSelect+`
			WHERE r.id = $2 AND r.session_id = $1`, sessionID, rowID)
		if err != nil {
			return fmt.Errorf("catalog postgres: get staging row: %w", err)
		}
		defer cursor.Close()

		if !cursor.Next() {
			return apperr.NotFound("import_row")
		}
		row, err = scanStagingRow(cursor)
		return err
	})
	if err != nil {
		return nil, err
	}
	return row, nil
}

// SetRowIncluded flips one row's inclusion switch.
func (r *Repository) SetRowIncluded(ctx context.Context, sessionID, rowID int64, included bool) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `
			UPDATE catalog.import_staging_rows
			SET included = $3
			WHERE id = $2 AND session_id = $1
		`, sessionID, rowID, included)
		if err != nil {
			return fmt.Errorf("catalog postgres: set staging row inclusion: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("import_row")
		}
		return nil
	})
}

// SetRowsIncludedByAction flips every row sharing an action, for the review
// table's "select all" controls. An empty action covers the whole session.
func (r *Repository) SetRowsIncludedByAction(
	ctx context.Context, sessionID int64, action catalog.RowAction, included bool,
) (int64, error) {
	var affected int64
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `UPDATE catalog.import_staging_rows SET included = $2 WHERE session_id = $1`
		args := []any{sessionID, included}
		if action != "" {
			query += ` AND action = $3`
			args = append(args, string(action))
		}
		tag, err := tx.Exec(txCtx, query, args...)
		if err != nil {
			return fmt.Errorf("catalog postgres: bulk set staging inclusion: %w", err)
		}
		affected = tag.RowsAffected()
		return nil
	})
	return affected, err
}

// CountStagingActions tallies the rows the admin has left selected, which is
// what the confirmation screen promises will happen.
func (r *Repository) CountStagingActions(ctx context.Context, sessionID int64) (catalog.StagingCounts, error) {
	var counts catalog.StagingCounts
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `
			SELECT
				count(*) FILTER (WHERE included AND action = 'insert'),
				count(*) FILTER (WHERE included AND action = 'update'),
				count(*) FILTER (WHERE NOT included OR action = 'skip'),
				count(*) FILTER (WHERE has_error),
				count(*) FILTER (WHERE has_warning),
				count(*) FILTER (WHERE has_ai),
				count(*)
			FROM catalog.import_staging_rows WHERE session_id = $1
		`, sessionID).Scan(
			&counts.Insert, &counts.Update, &counts.Skip,
			&counts.Errors, &counts.Warnings, &counts.AIChanged, &counts.Total)
	})
	if err != nil {
		return catalog.StagingCounts{}, fmt.Errorf("catalog postgres: count staging actions: %w", err)
	}
	return counts, nil
}

// LoadCommittableRows reads back the rows the admin left selected.
//
// The action is re-read from the table rather than recomputed, so what commits
// is exactly what the confirmation screen showed.
func (r *Repository) LoadCommittableRows(ctx context.Context, sessionID int64) ([]*catalog.StagingRow, error) {
	var rows []*catalog.StagingRow
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		cursor, err := tx.Query(txCtx, stagingRowSelect+`
			WHERE r.session_id = $1 AND r.included
			  AND r.action <> 'skip' AND NOT r.has_error
			ORDER BY r.source_row`, sessionID)
		if err != nil {
			return fmt.Errorf("catalog postgres: load committable rows: %w", err)
		}
		defer cursor.Close()

		for cursor.Next() {
			row, err := scanStagingRow(cursor)
			if err != nil {
				return err
			}
			rows = append(rows, row)
		}
		return cursor.Err()
	})
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// ArchiveAllProducts soft-deletes the whole catalogue for the clear-and-add
// mode, returning how many rows it retired.
//
// Soft, never hard: orders, variants and saved lists reference these ids, and a
// physical delete would cascade through all of them. An archived product can be
// brought back; a deleted one takes an order's history with it.
func (r *Repository) ArchiveAllProducts(ctx context.Context, orgID int64) (int64, error) {
	var archived int64
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `
			UPDATE catalog.products
			SET deleted_at = now(), updated_at = now()
			WHERE organization_id = $1 AND deleted_at IS NULL
		`, orgID)
		if err != nil {
			return fmt.Errorf("catalog postgres: archive catalogue: %w", err)
		}
		archived = tag.RowsAffected()
		return nil
	})
	return archived, err
}
