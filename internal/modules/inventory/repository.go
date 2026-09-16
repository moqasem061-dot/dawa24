package inventory

import (
	"context"
)

// Repository defines storage operations for the inventory bounded context.
type Repository interface {
	CreateWarehouse(ctx context.Context, w *Warehouse) error
	GetWarehouseByID(ctx context.Context, id int64) (*Warehouse, error)
	ListWarehouses(ctx context.Context) ([]*Warehouse, error)
	ListWarehousesWithTotal(ctx context.Context, limit, offset int) ([]*Warehouse, int, error)
	UpdateWarehouse(ctx context.Context, w *Warehouse) error
	SoftDeleteWarehouse(ctx context.Context, id int64) error
	// CountStockInWarehouse backs the refusal to delete a warehouse that still
	// holds goods. Soft-deleting one would orphan its stock rows: they would
	// keep counting toward sellable inventory while belonging to a warehouse no
	// screen lists any more.
	CountStockInWarehouse(ctx context.Context, warehouseID int64) (int, error)

	GetStock(ctx context.Context, warehouseID, variantID int64) (*Stock, error)
	// GetStockByID loads one stock row. Under a tenant context it only finds
	// the tenant's own rows.
	GetStockByID(ctx context.Context, id int64) (*Stock, error)
	UpsertStock(ctx context.Context, s *Stock) error
	ClearWarehouseStocks(ctx context.Context, warehouseID int64) error
	AdjustStock(ctx context.Context, stockID int64, delta int, movement StockMovement) (*Stock, error)
	// AvailableQuantity totals a variant's sellable stock across warehouses.
	// catalog.ProductVariant.StockQty is never populated by any query; this is
	// the real source.
	AvailableQuantity(ctx context.Context, variantID int64) (int, error)
	// AvailableQuantities totals many variants' sellable stock in one query,
	// keyed by variant ID.
	AvailableQuantities(ctx context.Context, variantIDs []int64) (map[int64]int, error)
	ListStocksByWarehouse(ctx context.Context, warehouseID int64) ([]*Stock, error)
	ListDetailedStocksByWarehouse(ctx context.Context, warehouseID int64) ([]*DetailedWarehouseStockView, error)
	ListStocksByOrg(ctx context.Context, orgID int64) ([]*Stock, error)
	ListStocksByOrgWithTotal(ctx context.Context, orgID int64, warehouseID int64, search string, limit, offset int) ([]*Stock, int, error)
	ListLowStocksByOrgWithTotal(ctx context.Context, orgID int64, warehouseID int64, search string, limit, offset int) ([]*Stock, int, error)
	ListStockMovements(ctx context.Context, stockID int64, limit int) ([]*StockMovement, error)
	// ListLowStock returns rows at or below their reorder threshold, which is
	// what the vendor replenishment screen is built on.
	ListLowStock(ctx context.Context, limit, offset int) ([]*Stock, error)
	ListLowStockWithTotal(ctx context.Context, limit, offset int) ([]*Stock, int, error)
	// ListMovementsByOrg is the organisation-wide ledger, newest first.
	ListMovementsByOrg(ctx context.Context, limit, offset int) ([]*StockMovement, error)

	CreateTransfer(ctx context.Context, t *WarehouseTransfer) error
	GetTransferByID(ctx context.Context, id int64) (*WarehouseTransfer, error)
	// UpdateTransferStatus is a compare-and-swap: it only applies when the row
	// is still in the `from` state. Two concurrent receives would otherwise
	// both read `in_transit`, both pass the in-memory state check, and both
	// credit the destination — creating stock out of nothing. The loser of the
	// race gets zero rows affected and a conflict.
	UpdateTransferStatus(ctx context.Context, id int64, from, to TransferStatus) error
	ListTransfers(ctx context.Context, status string, limit, offset int) ([]*WarehouseTransfer, error)
	ListTransfersWithTotal(ctx context.Context, status string, limit, offset int) ([]*WarehouseTransfer, int, error)
}
