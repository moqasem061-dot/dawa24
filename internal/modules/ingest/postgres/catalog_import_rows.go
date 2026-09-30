package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/moqasem061-dot/dawa24/internal/modules/ingest"
)

// clampScore keeps a match score inside what match_score NUMERIC(5,4) accepts.
// The scores are 0..1 by construction; this is the guard against an upstream
// regression feeding a percentage or a NaN through — which would otherwise
// overflow one CopyFrom call and lose every row outcome in the batch.
func clampScore(score float64) float64 {
	switch {
	case score < 0:
		return 0
	case score > 1:
		return 1
	default:
		return score
	}
}

// AppendRows records a batch of row outcomes.
func (r *Repository) AppendRows(
	ctx context.Context, importID, orgID int64, rows []ingest.RowOutcome,
) error {
	if len(rows) == 0 {
		return nil
	}
	records := make([][]any, 0, len(rows))
	for _, row := range rows {
		payload, err := json.Marshal(row.Payload)
		if err != nil {
			payload = []byte(`{}`)
		}
		candidates, err := json.Marshal(row.Candidates)
		if err != nil || row.Candidates == nil {
			candidates = []byte(`[]`)
		}
		issues, err := json.Marshal(row.Issues)
		if err != nil || row.Issues == nil {
			issues = []byte(`[]`)
		}

		var matchDecisionJSON, baselineDecisionJSON []byte
		if row.MatchDecision != nil {
			matchDecisionJSON, _ = json.Marshal(row.MatchDecision)
		}
		if row.BaselineDecision != nil {
			baselineDecisionJSON, _ = json.Marshal(row.BaselineDecision)
		}

		var matchState, matchBasis, matchGranularity, aiState any
		if row.MatchState != "" {
			matchState = row.MatchState
		}
		if row.MatchBasis != "" {
			matchBasis = row.MatchBasis
		}
		if row.MatchGranularity != "" {
			matchGranularity = row.MatchGranularity
		}
		if row.AIState != "" {
			aiState = row.AIState
		}
		var matchDecVal, baselineDecVal any
		if len(matchDecisionJSON) > 0 {
			matchDecVal = matchDecisionJSON
		}
		if len(baselineDecisionJSON) > 0 {
			baselineDecVal = baselineDecisionJSON
		}

		records = append(records, []any{
			importID, orgID, row.SourceRow, row.Outcome, row.MatchLevel,
			clampScore(row.MatchScore), row.ProductID, row.VariantID,
			trimTo(row.DisplayName, 300), trimTo(row.SourceCode, 100),
			trimTo(row.CustomVariantName, 300), row.IsExcluded, row.IsManuallyMatched,
			payload, candidates, issues, trimTo(row.Message, 500),
			matchState, matchBasis, matchGranularity, row.MatchRevision,
			row.HumanLocked, aiState, row.MatchRunID, matchDecVal, baselineDecVal,
		})
	}

	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.CopyFrom(txCtx,
			pgx.Identifier{"ingest", "catalog_import_rows"},
			[]string{
				"import_id", "organization_id", "source_row", "outcome", "match_level",
				"match_score", "product_id", "variant_id", "display_name",
				"source_code", "custom_variant_name", "is_excluded", "is_manually_matched",
				"payload", "candidates", "issues", "message",
				"match_state", "match_basis", "match_granularity", "match_revision",
				"human_locked", "ai_state", "match_run_id", "match_decision", "baseline_decision",
			},
			pgx.CopyFromRows(records))
		if err != nil {
			return fmt.Errorf("ingest postgres: append import rows: %w", err)
		}
		return nil
	})
}

// ClearRows wipes previously staged rows for an import.
func (r *Repository) ClearRows(ctx context.Context, importID int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `DELETE FROM ingest.catalog_import_rows WHERE import_id = $1`, importID)
		return err
	})
}

// Rows reads a page of the results table.
func (r *Repository) Rows(
	ctx context.Context, importID int64, filter ingest.RowFilter,
) ([]*ingest.RowOutcome, int, error) {
	if filter.Limit <= 0 || filter.Limit > 500 {
		filter.Limit = 50
	}
	clause, args := rowsWhere(importID, filter)

	var out []*ingest.RowOutcome
	var total int
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(txCtx,
			`SELECT count(*) 
			 FROM ingest.catalog_import_rows r
			 LEFT JOIN catalog.products p ON p.id = r.product_id
			 WHERE `+clause, args...,
		).Scan(&total); err != nil {
			return fmt.Errorf("ingest postgres: count import rows: %w", err)
		}

		orderDir := "ASC"
		if strings.EqualFold(filter.SortOrder, "desc") {
			orderDir = "DESC"
		}

		var orderByCol string
		switch filter.SortBy {
		case "row", "source_row":
			orderByCol = "r.source_row"
		case "name", "display_name":
			orderByCol = "r.display_name"
		case "catalog_name":
			orderByCol = "COALESCE(p.name->>'ar', p.name->>'en', '')"
		case "score", "match_score":
			orderByCol = "r.match_score"
		case "price":
			orderByCol = "COALESCE((r.payload->'net_price'->>'minor')::bigint, (r.payload->'public_price'->>'minor')::bigint, 0)"
		case "quantity":
			orderByCol = "COALESCE((r.payload->>'quantity')::int, 0)"
		case "discount":
			// The review table offers this header, so it has to mean something
			// here; without the case it fell through to the default and sorted
			// by match score while claiming to sort by discount.
			orderByCol = "COALESCE((r.payload->>'discount_bps')::bigint, 0)"
		default:
			// Weakest match first, and this default is the whole review
			// workflow.
			//
			// It used to be source-row order, which is the order the rows
			// happen to sit in the vendor's file — an order that tells the
			// reviewer nothing. Nine thousand rows in file order means reading
			// nine thousand rows to find the forty that need a decision,
			// because the ones that need it are scattered evenly among the ones
			// that do not. Worst first puts every doubtful row on page one, and
			// a reviewer who stops when the scores look right has still seen
			// everything that mattered.
			//
			// A row with no match at all scores zero and therefore sorts first,
			// which is correct: unmatched is the worst outcome there is.
			orderByCol = "r.match_score"
		}
		orderByClause := fmt.Sprintf("%s %s, r.source_row ASC", orderByCol, orderDir)

		paged := append(append([]any{}, args...), filter.Limit, filter.Offset)
		rows, err := tx.Query(txCtx, fmt.Sprintf(`
			SELECT r.id, r.source_row, r.outcome, r.match_level, r.match_score, r.product_id,
			       COALESCE(p.name->>'ar', p.name->>'en', ''), COALESCE(p.sku, ''),
			       r.variant_id, r.display_name, r.source_code, r.custom_variant_name,
			       r.is_excluded, r.is_manually_matched, r.payload, r.candidates, r.issues, r.message,
			       COALESCE(r.match_state, ''), COALESCE(r.match_basis, ''), COALESCE(r.match_granularity, ''),
			       r.match_revision, r.human_locked, COALESCE(r.ai_state, ''), r.match_run_id,
			       r.match_decision, r.baseline_decision
			FROM ingest.catalog_import_rows r
			LEFT JOIN catalog.products p ON p.id = r.product_id
			WHERE %s
			ORDER BY %s
			LIMIT $%d OFFSET $%d`, clause, orderByClause, len(args)+1, len(args)+2), paged...)
		if err != nil {
			return fmt.Errorf("ingest postgres: list import rows: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var o ingest.RowOutcome
			var payload, candidates, issues, matchDecBytes, baseDecBytes []byte
			if err := rows.Scan(&o.ID, &o.SourceRow, &o.Outcome, &o.MatchLevel,
				&o.MatchScore, &o.ProductID, &o.MatchedProductName, &o.MatchedProductSKU,
				&o.VariantID, &o.DisplayName, &o.SourceCode, &o.CustomVariantName,
				&o.IsExcluded, &o.IsManuallyMatched, &payload, &candidates, &issues, &o.Message,
				&o.MatchState, &o.MatchBasis, &o.MatchGranularity, &o.MatchRevision,
				&o.HumanLocked, &o.AIState, &o.MatchRunID, &matchDecBytes, &baseDecBytes); err != nil {
				return fmt.Errorf("ingest postgres: scan import row: %w", err)
			}
			_ = json.Unmarshal(payload, &o.Payload)
			_ = json.Unmarshal(candidates, &o.Candidates)
			_ = json.Unmarshal(issues, &o.Issues)
			if len(matchDecBytes) > 0 {
				_ = json.Unmarshal(matchDecBytes, &o.MatchDecision)
			}
			if len(baseDecBytes) > 0 {
				_ = json.Unmarshal(baseDecBytes, &o.BaselineDecision)
			}
			out = append(out, &o)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// RowCounts tallies the ledger by outcome, for the results screen's tabs.
func (r *Repository) RowCounts(ctx context.Context, importID int64) (map[string]int, error) {
	counts := map[string]int{}
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			SELECT outcome, count(*) FROM ingest.catalog_import_rows
			WHERE import_id = $1 GROUP BY outcome`, importID)
		if err != nil {
			return fmt.Errorf("ingest postgres: count import outcomes: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var key string
			var n int
			if err := rows.Scan(&key, &n); err != nil {
				return err
			}
			counts[key] = n
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return counts, nil
}
