package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
)

func nullableMatchText(value string) any {
	if value == "" {
		return nil
	}
	return value
}
func nullableDecision(value json.RawMessage) any {
	if len(value) == 0 {
		return nil
	}
	return value
}
func insertFileRows(txCtx context.Context, tx pgx.Tx, rows []*compare.CompareFileRow) error {
	cols := []string{
		"file_id", "organization_id", "row_number", "raw_name", "normalized_name", "sku",
		"price", "discount", "price_after_discount", "matched_product_id", "match_confidence", "match_method", "meta",
		"match_state", "match_basis", "match_granularity", "match_revision", "human_locked", "ai_state", "match_run_id", "match_decision",
	}

	const chunkSize = 5000
	for i := 0; i < len(rows); i += chunkSize {
		end := i + chunkSize
		if end > len(rows) {
			end = len(rows)
		}
		chunk := rows[i:end]

		copyRows := make([][]any, len(chunk))
		for j, row := range chunk {
			metaJSON, err := json.Marshal(row.Meta)
			if err != nil {
				return fmt.Errorf("encode row metadata: %w", err)
			}
			dbMethod := toDBMatchMethod(row.MatchMethod)

			var skuVal any
			if strings.TrimSpace(row.SKU) != "" {
				skuVal = strings.TrimSpace(row.SKU)
			}

			copyRows[j] = []any{
				row.FileID,
				row.OrganizationID,
				row.RowNumber,
				row.RawName,
				row.NormalizedName,
				skuVal,
				row.Price.String(),
				row.Discount,
				row.PriceAfterDiscount.String(),
				row.MatchedProductID,
				row.MatchConfidence,
				dbMethod,
				metaJSON,
				nullableMatchText(row.MatchState), row.MatchBasis, row.MatchGranularity, row.MatchRevision, row.HumanLocked, row.AIState, row.MatchRunID, nullableDecision(row.MatchDecision),
			}
		}

		_, err := tx.CopyFrom(
			txCtx,
			pgx.Identifier{"compare", "file_rows"},
			cols,
			pgx.CopyFromRows(copyRows),
		)
		if err != nil {
			return fmt.Errorf("CopyFrom chunk [%d:%d]: %w", i, end, err)
		}
	}
	return nil
}
