package erpports

import (
	"context"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/modules/inventory"
)

// NewStockWriter adapts the inventory service to the ERP's StockWriter port.
// Field conversion only (ADR-ERP-1): the sync's replace semantics map onto
// inventory's BulkWriteStocks(StockReplace), keyed by the caller's Ref.
func NewStockWriter(svc *inventory.Service) erp.StockWriter {
	return inventoryStockWriter{svc: svc}
}

type inventoryStockWriter struct {
	svc *inventory.Service
}

func (w inventoryStockWriter) Replace(ctx context.Context, rows []erp.StockWrite) (map[int]string, error) {
	out := make([]inventory.StockWriteRow, 0, len(rows))
	for _, row := range rows {
		quantity := row.Quantity
		out = append(out, inventory.StockWriteRow{
			Ref: row.Ref,
			Stock: &inventory.Stock{
				OrganizationID:   row.OrganizationID,
				WarehouseID:      row.WarehouseID,
				ProductID:        row.ProductID,
				ProductVariantID: row.VariantID,
				Quantity:         quantity,
			},
			HasQuantity: row.HasQuantity,
		})
	}
	result, err := w.svc.BulkWriteStocks(ctx, inventory.StockReplace, out)
	if err != nil {
		return nil, err
	}
	failures := map[int]string{}
	for _, failure := range result.Failures {
		failures[failure.Ref] = failure.Message
	}
	return failures, nil
}

var _ erp.StockWriter = inventoryStockWriter{}
