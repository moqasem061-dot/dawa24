package postgres

import (
	"context"
	"encoding/json"
	"math"

	"github.com/jackc/pgx/v5"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

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
