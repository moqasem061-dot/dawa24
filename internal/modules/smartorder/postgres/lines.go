package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

const lineColumns = `
	id, run_id, organization_id, row_number, raw,
	COALESCE(raw_name, '')        AS raw_name,
	COALESCE(raw_sku, '')         AS raw_sku,
	COALESCE(raw_barcode, '')     AS raw_barcode,
	imported_qty,
	COALESCE(qty_parse_note, '')  AS qty_parse_note,
	edited_qty, effective_qty,
	COALESCE(norm_name, '')       AS norm_name,
	COALESCE(identity_key, '')    AS identity_key,
	matched_product_id, match_method, match_confidence,
	match_corrected_by_user, outcome,
	COALESCE(outcome_reason, '')  AS outcome_reason,
	consolidated_into_line_id,
	created_at, updated_at`

func scanLine(row pgx.Row) (*smartorder.Line, error) {
	var l smartorder.Line
	var raw []byte
	var method, outcome string
	if err := row.Scan(
		&l.ID, &l.RunID, &l.OrganizationID, &l.RowNumber, &raw,
		&l.RawName, &l.RawSKU, &l.RawBarcode,
		&l.ImportedQty, &l.QtyParseNote, &l.EditedQty, &l.EffectiveQty,
		&l.NormName, &l.IdentityKey, &l.MatchedProductID, &method, &l.MatchConfidence,
		&l.CorrectedByUser, &outcome, &l.OutcomeReason, &l.ConsolidatedInto,
		&l.CreatedAt, &l.UpdatedAt,
	); err != nil {
		return nil, err
	}
	l.MatchMethod = smartorder.MatchMethod(method)
	l.Outcome = smartorder.Outcome(outcome)
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &l.Raw)
	}
	return &l, nil
}

// insertBatchSize matches the ingest pipeline. Large enough that a ten-thousand
// row file is twenty round trips rather than ten thousand; small enough that one
// failed batch does not roll back an hour of work.
const insertBatchSize = 500

// InsertLines writes staged rows in batches inside one transaction.
//
// Built as a multi-row VALUES rather than a loop of single inserts: FR-017a
// forbids per-row work, and at ten thousand rows the difference is minutes.
func (r *Repository) InsertLines(ctx context.Context, lines []*smartorder.Line) error {
	if len(lines) == 0 {
		return nil
	}
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		for start := 0; start < len(lines); start += insertBatchSize {
			end := start + insertBatchSize
			if end > len(lines) {
				end = len(lines)
			}
			if err := insertLineBatch(txCtx, tx, lines[start:end]); err != nil {
				return fmt.Errorf("insert lines %d-%d: %w", start, end, err)
			}
		}
		return nil
	})
}

func insertLineBatch(ctx context.Context, tx pgx.Tx, batch []*smartorder.Line) error {
	const cols = 14
	values := make([]string, 0, len(batch))
	args := make([]any, 0, len(batch)*cols)

	for i, l := range batch {
		base := i * cols
		ph := make([]string, cols)
		for j := 0; j < cols; j++ {
			ph[j] = "$" + strconv.Itoa(base+j+1)
		}
		values = append(values, "("+strings.Join(ph, ",")+")")

		rawJSON, _ := json.Marshal(l.Raw)
		if l.MatchMethod == "" {
			l.MatchMethod = smartorder.MethodNone
		}
		if l.Outcome == "" {
			l.Outcome = smartorder.OutcomeUnmatched
		}
		args = append(args,
			l.RunID, l.OrganizationID, l.RowNumber, rawJSON,
			l.RawName, l.RawSKU, l.RawBarcode,
			l.ImportedQty, l.QtyParseNote, l.EditedQty, l.EffectiveQty,
			l.NormName, l.IdentityKey, string(l.Outcome))
	}

	query := `
		INSERT INTO smartorder.run_lines (
			run_id, organization_id, row_number, raw, raw_name, raw_sku, raw_barcode,
			imported_qty, qty_parse_note, edited_qty, effective_qty,
			norm_name, identity_key, outcome
		) VALUES ` + strings.Join(values, ",") + `
		ON CONFLICT (run_id, row_number) DO NOTHING;`

	_, err := tx.Exec(ctx, query, args...)
	return err
}

// ListLines returns a filtered page of results plus the total matching count.
func (r *Repository) ListLines(ctx context.Context, runID int64, f smartorder.LineFilter) ([]*smartorder.Line, int, error) {
	if !f.All && (f.Limit <= 0 || f.Limit > 500) {
		f.Limit = 50
	}
	var out []*smartorder.Line
	var total int

	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		where := []string{"run_id = $1"}
		args := []any{runID}

		if f.MatchGroup != "" {
			switch f.MatchGroup {
			case "unmatched":
				where = append(where, "(matched_product_id IS NULL OR matched_product_id = 0)")
			case "matched", "matched_product":
				where = append(where, "(matched_product_id IS NOT NULL AND matched_product_id > 0)")
			case "matched_supplier":
				where = append(where, "(EXISTS (SELECT 1 FROM smartorder.line_candidates c WHERE c.line_id = smartorder.run_lines.id AND c.eligible))")
			case "available_stock":
				where = append(where, "(EXISTS (SELECT 1 FROM smartorder.line_candidates c WHERE c.line_id = smartorder.run_lines.id AND c.eligible AND c.stock_qty > 0))")
			case "covered_branch":
				where = append(where, "(EXISTS (SELECT 1 FROM smartorder.line_candidates c WHERE c.line_id = smartorder.run_lines.id AND c.eligible AND (c.ineligible_reason IS NULL OR c.ineligible_reason != 'coverage')))")
			case "price_available":
				where = append(where, "(EXISTS (SELECT 1 FROM smartorder.line_candidates c WHERE c.line_id = smartorder.run_lines.id AND c.eligible AND c.net_unit_price > 0))")
			case "ready_to_order", "matched_available", "ordered":
				where = append(where, "(outcome = 'ordered')")
			case "review":
				where = append(where, "(outcome IN ('no_supplier', 'coverage_blocked', 'institutional_blocked', 'out_of_stock', 'below_min_qty', 'zero_qty'))")
			case "blocked":
				// Everything the buyer is not getting, whatever the reason —
				// what the collapsed section on the results screen holds.
				// 'removed' is excluded: that is a decision, not a problem.
				where = append(where, "(outcome NOT IN ('ordered', 'removed'))")
			}
		}

		if f.Outcome != "" {
			args = append(args, f.Outcome)
			where = append(where, "outcome = $"+strconv.Itoa(len(args)))
		}
		if f.Method != "" {
			args = append(args, f.Method)
			where = append(where, "match_method = $"+strconv.Itoa(len(args)))
		}
		if f.Search != "" {
			args = append(args, "%"+f.Search+"%")
			param := "$" + strconv.Itoa(len(args))
			where = append(where, "(raw_name ILIKE "+param+" OR raw_sku ILIKE "+param+" OR raw_barcode ILIKE "+param+")")
		}
		clause := strings.Join(where, " AND ")

		if err := tx.QueryRow(txCtx,
			`SELECT count(*) FROM smartorder.run_lines WHERE `+clause+`;`, args...).Scan(&total); err != nil {
			return err
		}

		// Sort column resolution
		sortCol := "row_number"
		switch f.SortBy {
		case "name":
			sortCol = "raw_name"
		case "matched_name":
			sortCol = "COALESCE((SELECT COALESCE(name->>'ar', name->>'en') FROM catalog.products WHERE id = matched_product_id), '')"
		case "method":
			sortCol = "match_method"
		case "confidence":
			sortCol = "match_confidence"
		case "qty", "quantity":
			sortCol = "effective_qty"
		case "outcome", "status":
			sortCol = "outcome"
		default:
			sortCol = "row_number"
		}

		sortOrder := "ASC"
		if strings.ToUpper(f.SortOrder) == "DESC" {
			sortOrder = "DESC"
		} else if f.SortBy == "confidence" && f.SortOrder == "" {
			sortOrder = "ASC"
		}

		nullsOrder := ""
		if f.SortBy == "confidence" || f.SortBy == "matched_name" {
			nullsOrder = " NULLS LAST"
		}

		page := ""
		if !f.All {
			args = append(args, f.Limit, f.Offset)
			page = ` LIMIT $` + strconv.Itoa(len(args)-1) + ` OFFSET $` + strconv.Itoa(len(args))
		}
		rows, err := tx.Query(txCtx,
			`SELECT `+lineColumns+` FROM smartorder.run_lines WHERE `+clause+`
			 ORDER BY `+sortCol+` `+sortOrder+nullsOrder+`, row_number ASC`+page+`;`, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			l, err := scanLine(rows)
			if err != nil {
				return err
			}
			out = append(out, l)
		}
		return rows.Err()
	})
	return out, total, err
}

// GetLine loads one line, scoped to the caller's organisation.
func (r *Repository) GetLine(ctx context.Context, orgID, lineID int64) (*smartorder.Line, error) {
	var l *smartorder.Line
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		row := tx.QueryRow(txCtx,
			`SELECT `+lineColumns+` FROM smartorder.run_lines WHERE id = $1 AND organization_id = $2;`,
			lineID, orgID)
		var err error
		l, err = scanLine(row)
		if err == pgx.ErrNoRows {
			return apperr.NotFound("smart_order_line")
		}
		return err
	})
	return l, err
}

// UpdateLines writes match results back for a whole batch.
//
// One statement per batch using unnest, so a ten-thousand-row file costs twenty
// statements rather than ten thousand.
func (r *Repository) UpdateLines(ctx context.Context, lines []*smartorder.Line) error {
	if len(lines) == 0 {
		return nil
	}
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		for start := 0; start < len(lines); start += insertBatchSize {
			end := start + insertBatchSize
			if end > len(lines) {
				end = len(lines)
			}
			batch := lines[start:end]

			ids := make([]int64, len(batch))
			productIDs := make([]*int64, len(batch))
			methods := make([]string, len(batch))
			confidences := make([]float64, len(batch))
			outcomes := make([]string, len(batch))
			reasons := make([]string, len(batch))
			quantities := make([]float64, len(batch))
			normNames := make([]string, len(batch))
			identityKeys := make([]string, len(batch))

			for i, l := range batch {
				ids[i] = l.ID
				productIDs[i] = l.MatchedProductID
				methods[i] = string(l.MatchMethod)
				confidences[i] = l.MatchConfidence
				outcomes[i] = string(l.Outcome)
				reasons[i] = l.OutcomeReason
				quantities[i] = l.EffectiveQty
				normNames[i] = l.NormName
				identityKeys[i] = l.IdentityKey
			}

			_, err := tx.Exec(txCtx, `
				UPDATE smartorder.run_lines AS l SET
					matched_product_id = u.product_id,
					match_method       = u.method,
					match_confidence   = u.confidence,
					outcome            = u.outcome,
					outcome_reason     = u.reason,
					effective_qty      = u.qty,
					norm_name          = u.norm_name,
					identity_key       = u.identity_key
				FROM (
					SELECT * FROM unnest(
						$1::bigint[], $2::bigint[], $3::text[], $4::numeric[],
						$5::text[], $6::text[], $7::numeric[], $8::text[], $9::text[]
					) AS t(id, product_id, method, confidence, outcome, reason, qty, norm_name, identity_key)
				) AS u
				WHERE l.id = u.id;`,
				ids, productIDs, methods, confidences, outcomes, reasons,
				quantities, normNames, identityKeys)
			if err != nil {
				return fmt.Errorf("update lines %d-%d: %w", start, end, err)
			}
		}
		return nil
	})
}

// UpdateLineQuantity applies a buyer's edit to one line.
func (r *Repository) UpdateLineQuantity(ctx context.Context, orgID, lineID int64, qty float64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `
			UPDATE smartorder.run_lines
			SET edited_qty = $3, effective_qty = $3
			WHERE id = $1 AND organization_id = $2;`, lineID, orgID, qty)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("smart_order_line")
		}
		return nil
	})
}

// SetDefaultQuantity applies a quantity to all (or selected) lines in a run, and updates selections.
func (r *Repository) SetDefaultQuantity(ctx context.Context, orgID, runID int64, lineIDs []int64, qty float64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		// 1. Update lines in run_lines, capping at available supplier stock if selected
		var err error
		if len(lineIDs) > 0 {
			_, err = tx.Exec(txCtx, `
				UPDATE smartorder.run_lines l
				SET edited_qty = LEAST($1::numeric, COALESCE((
					SELECT NULLIF(c.stock_qty, 0)
					FROM smartorder.line_selections s
					JOIN smartorder.line_candidates c ON c.id = s.candidate_id
					WHERE s.line_id = l.id AND s.organization_id = $3
				), $1::numeric)),
				    effective_qty = LEAST($1::numeric, COALESCE((
					SELECT NULLIF(c.stock_qty, 0)
					FROM smartorder.line_selections s
					JOIN smartorder.line_candidates c ON c.id = s.candidate_id
					WHERE s.line_id = l.id AND s.organization_id = $3
				), $1::numeric))
				WHERE run_id = $2 AND organization_id = $3 AND id = ANY($4);`,
				qty, runID, orgID, lineIDs)
		} else {
			_, err = tx.Exec(txCtx, `
				UPDATE smartorder.run_lines l
				SET edited_qty = LEAST($1::numeric, COALESCE((
					SELECT NULLIF(c.stock_qty, 0)
					FROM smartorder.line_selections s
					JOIN smartorder.line_candidates c ON c.id = s.candidate_id
					WHERE s.line_id = l.id AND s.organization_id = $3
				), $1::numeric)),
				    effective_qty = LEAST($1::numeric, COALESCE((
					SELECT NULLIF(c.stock_qty, 0)
					FROM smartorder.line_selections s
					JOIN smartorder.line_candidates c ON c.id = s.candidate_id
					WHERE s.line_id = l.id AND s.organization_id = $3
				), $1::numeric))
				WHERE run_id = $2 AND organization_id = $3 AND outcome = 'ordered';`,
				qty, runID, orgID)
		}
		if err != nil {
			return err
		}

		// 2. Update selections line_net
		if qty == 0 {
			if len(lineIDs) > 0 {
				_, err = tx.Exec(txCtx, `
					DELETE FROM smartorder.line_selections
					WHERE organization_id = $1 AND line_id = ANY($2);`,
					orgID, lineIDs)
			} else {
				_, err = tx.Exec(txCtx, `
					DELETE FROM smartorder.line_selections s
					USING smartorder.run_lines l
					WHERE s.line_id = l.id AND l.run_id = $1 AND s.organization_id = $2;`,
					runID, orgID)
			}
			return err
		}

		// Recalculate line_net for updated selections using candidate net_unit_price and line effective_qty
		_, err = tx.Exec(txCtx, `
			UPDATE smartorder.line_selections s
			SET line_net = ROUND(c.net_unit_price * l.effective_qty, 2),
			    updated_at = now()
			FROM smartorder.line_candidates c, smartorder.run_lines l
			WHERE s.candidate_id = c.id
			  AND s.line_id = l.id
			  AND l.run_id = $1
			  AND s.organization_id = $2
			  AND ($3::bigint[] IS NULL OR cardinality($3::bigint[]) = 0 OR l.id = ANY($3));
		`, runID, orgID, lineIDs)
		return err
	})
}


// BlockedCounts tallies the lines that will not be ordered, by reason.
func (r *Repository) BlockedCounts(ctx context.Context, runID int64) (smartorder.BlockedCounts, error) {
	var bc smartorder.BlockedCounts
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `
			SELECT
				COUNT(*) FILTER (WHERE outcome = 'unmatched'),
				COUNT(*) FILTER (WHERE outcome = 'no_supplier'),
				COUNT(*) FILTER (WHERE outcome = 'coverage_blocked'),
				COUNT(*) FILTER (WHERE outcome = 'institutional_blocked'),
				COUNT(*) FILTER (WHERE outcome = 'out_of_stock'),
				COUNT(*) FILTER (WHERE outcome = 'below_min_qty'),
				COUNT(*) FILTER (WHERE outcome = 'zero_qty'),
				COUNT(*) FILTER (WHERE outcome = 'removed'),
				COUNT(*) FILTER (WHERE outcome = 'quota_blocked')
			FROM smartorder.run_lines
			WHERE run_id = $1;`, runID).Scan(
			&bc.Unmatched, &bc.NoSupplier, &bc.CoverageBlocked, &bc.InstitutionalBlocked,
			&bc.OutOfStock, &bc.BelowMinQty, &bc.ZeroQty, &bc.Removed, &bc.QuotaBlocked)
	})
	return bc, err
}

// FilterCounts returns exact row counts for each independent smart order filter tab.
func (r *Repository) FilterCounts(ctx context.Context, runID int64) (smartorder.FilterCounts, error) {
	var fc smartorder.FilterCounts
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT
				COUNT(*),
				COUNT(*) FILTER (WHERE matched_product_id IS NULL OR matched_product_id = 0),
				COUNT(*) FILTER (WHERE matched_product_id IS NOT NULL AND matched_product_id > 0),
				COUNT(*) FILTER (WHERE EXISTS (SELECT 1 FROM smartorder.line_candidates c WHERE c.line_id = l.id AND c.eligible)),
				COUNT(*) FILTER (WHERE EXISTS (SELECT 1 FROM smartorder.line_candidates c WHERE c.line_id = l.id AND c.eligible AND c.stock_qty > 0)),
				COUNT(*) FILTER (WHERE EXISTS (SELECT 1 FROM smartorder.line_candidates c WHERE c.line_id = l.id AND c.eligible AND (c.ineligible_reason IS NULL OR c.ineligible_reason != 'coverage'))),
				COUNT(*) FILTER (WHERE EXISTS (SELECT 1 FROM smartorder.line_candidates c WHERE c.line_id = l.id AND c.eligible AND c.net_unit_price > 0)),
				COUNT(*) FILTER (WHERE outcome = 'ordered')
			FROM smartorder.run_lines l
			WHERE l.run_id = $1;
		`
		return tx.QueryRow(txCtx, query, runID).Scan(
			&fc.Total,
			&fc.Unmatched,
			&fc.MatchedProduct,
			&fc.MatchedSupplier,
			&fc.AvailableStock,
			&fc.CoveredBranch,
			&fc.PriceAvailable,
			&fc.ReadyToOrder,
		)
	})
	return fc, err
}
