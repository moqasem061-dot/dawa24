package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/ingest"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
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
		records = append(records, []any{
			importID, orgID, row.SourceRow, row.Outcome, row.MatchLevel,
			clampScore(row.MatchScore), row.ProductID, row.VariantID,
			trimTo(row.DisplayName, 300), trimTo(row.SourceCode, 100),
			trimTo(row.CustomVariantName, 300), row.IsExcluded, row.IsManuallyMatched,
			payload, candidates, issues, trimTo(row.Message, 500),
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
			       r.is_excluded, r.is_manually_matched, r.payload, r.candidates, r.issues, r.message
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
			var payload, candidates, issues []byte
			if err := rows.Scan(&o.ID, &o.SourceRow, &o.Outcome, &o.MatchLevel,
				&o.MatchScore, &o.ProductID, &o.MatchedProductName, &o.MatchedProductSKU,
				&o.VariantID, &o.DisplayName, &o.SourceCode, &o.CustomVariantName,
				&o.IsExcluded, &o.IsManuallyMatched, &payload, &candidates, &issues, &o.Message); err != nil {
				return fmt.Errorf("ingest postgres: scan import row: %w", err)
			}
			_ = json.Unmarshal(payload, &o.Payload)
			_ = json.Unmarshal(candidates, &o.Candidates)
			_ = json.Unmarshal(issues, &o.Issues)
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

// UpdateRow updates fields on a staged row before commit.
func (r *Repository) UpdateRow(
	ctx context.Context, importID, rowID int64,
	displayName, customVariantName string,
	price, discount, costPrice, costDiscount *float64, quantity *int, isExcluded *bool,
) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		var payload []byte
		var currName, currVarName string
		var currExcluded bool
		err := tx.QueryRow(txCtx, `
			SELECT display_name, custom_variant_name, is_excluded, payload
			FROM ingest.catalog_import_rows
			WHERE id = $1 AND import_id = $2`, rowID, importID).Scan(&currName, &currVarName, &currExcluded, &payload)
		if err != nil {
			return err
		}

		var rowData productmatch.Row
		_ = json.Unmarshal(payload, &rowData)

		if displayName != "" {
			currName = displayName
			rowData.Name = displayName
		}
		if customVariantName != "" {
			currVarName = customVariantName
		}
		if isExcluded != nil {
			currExcluded = *isExcluded
		}
		if price != nil {
			m := money.FromMinor(int64(math.Round(*price * 100)))
			rowData.PublicPrice = m
			if rowData.DiscountBps > 0 {
				disc := m.ApplyPercent(rowData.DiscountBps)
				if net, subErr := m.Sub(disc); subErr == nil && net.Minor() >= 0 {
					rowData.NetPrice = net
				} else {
					rowData.NetPrice = m
				}
			} else {
				rowData.NetPrice = m
			}
			if rowData.CostDiscountBps > 0 {
				disc := m.ApplyPercent(rowData.CostDiscountBps)
				if cost, subErr := m.Sub(disc); subErr == nil && cost.Minor() >= 0 {
					rowData.CostPrice = cost
				}
			}
		}
		if discount != nil {
			bps := int64(math.Round(*discount * 100))
			rowData.DiscountBps = bps
			if rowData.PublicPrice.IsPositive() {
				disc := rowData.PublicPrice.ApplyPercent(bps)
				if net, subErr := rowData.PublicPrice.Sub(disc); subErr == nil && net.Minor() >= 0 {
					rowData.NetPrice = net
				}
			}
		}
		if costPrice != nil {
			rowData.CostPrice = money.FromMinor(int64(math.Round(*costPrice * 100)))
		}
		if costDiscount != nil {
			bps := int64(math.Round(*costDiscount * 100))
			rowData.CostDiscountBps = bps
			if rowData.PublicPrice.IsPositive() {
				disc := rowData.PublicPrice.ApplyPercent(bps)
				if cost, subErr := rowData.PublicPrice.Sub(disc); subErr == nil && cost.Minor() >= 0 {
					rowData.CostPrice = cost
				}
			}
		}
		if quantity != nil {
			rowData.Quantity = *quantity
			rowData.HasQuantity = true
		}

		newPayload, _ := json.Marshal(rowData)

		_, err = tx.Exec(txCtx, `
			UPDATE ingest.catalog_import_rows
			SET display_name = $3, custom_variant_name = $4, is_excluded = $5, payload = $6
			WHERE id = $1 AND import_id = $2`,
			rowID, importID, currName, currVarName, currExcluded, newPayload)
		return err
	})
}

// SetBatchQuantity applies a uniform quantity to all staged rows of an import.
func (r *Repository) SetBatchQuantity(ctx context.Context, importID int64, quantity int) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			SELECT id, payload
			FROM ingest.catalog_import_rows
			WHERE import_id = $1`, importID)
		if err != nil {
			return err
		}
		defer rows.Close()

		type rowPayloadItem struct {
			id      int64
			payload []byte
		}
		var items []rowPayloadItem
		for rows.Next() {
			var it rowPayloadItem
			if err := rows.Scan(&it.id, &it.payload); err != nil {
				return err
			}
			items = append(items, it)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		rows.Close()

		for _, it := range items {
			var rowData productmatch.Row
			_ = json.Unmarshal(it.payload, &rowData)
			rowData.Quantity = quantity
			rowData.HasQuantity = true
			newPayload, _ := json.Marshal(rowData)
			if _, err := tx.Exec(txCtx, `
				UPDATE ingest.catalog_import_rows
				SET payload = $2
				WHERE id = $1 AND import_id = $3`, it.id, newPayload, importID); err != nil {
				return err
			}
		}
		return nil
	})
}
