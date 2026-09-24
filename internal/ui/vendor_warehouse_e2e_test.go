package ui_test

import (
	"context"

	"github.com/moqasem061-dot/dawa24/internal/modules/inventory"
)

type mockInventoryRepoForWarehouseTest struct {
	warehouses map[int64]*inventory.Warehouse
	stocks     []*inventory.Stock
	detailed   []*inventory.DetailedWarehouseStockView
}

func newMockInventoryRepoForWarehouseTest() *mockInventoryRepoForWarehouseTest {
	return &mockInventoryRepoForWarehouseTest{
		warehouses: make(map[int64]*inventory.Warehouse),
	}
}

func (m *mockInventoryRepoForWarehouseTest) CreateWarehouse(_ context.Context, w *inventory.Warehouse) error {
	w.ID = int64(len(m.warehouses) + 1)
	m.warehouses[w.ID] = w
	return nil
}

func (m *mockInventoryRepoForWarehouseTest) GetWarehouseByID(_ context.Context, id int64) (*inventory.Warehouse, error) {
	if w, ok := m.warehouses[id]; ok {
		return w, nil
	}
	return nil, nil
}

func (m *mockInventoryRepoForWarehouseTest) ListWarehouses(_ context.Context) ([]*inventory.Warehouse, error) {
	var list []*inventory.Warehouse
	for _, w := range m.warehouses {
		list = append(list, w)
	}
	return list, nil
}

func (m *mockInventoryRepoForWarehouseTest) ListWarehousesWithTotal(_ context.Context, limit, offset int) ([]*inventory.Warehouse, int, error) {
	var list []*inventory.Warehouse
	for _, w := range m.warehouses {
		list = append(list, w)
	}
	return list, len(list), nil
}

func (m *mockInventoryRepoForWarehouseTest) UpdateWarehouse(_ context.Context, w *inventory.Warehouse) error {
	m.warehouses[w.ID] = w
	return nil
}

func (m *mockInventoryRepoForWarehouseTest) SoftDeleteWarehouse(_ context.Context, id int64) error {
	delete(m.warehouses, id)
	return nil
}

func (m *mockInventoryRepoForWarehouseTest) CountStockInWarehouse(_ context.Context, warehouseID int64) (int, error) {
	return len(m.detailed), nil
}

func (m *mockInventoryRepoForWarehouseTest) GetStockByID(_ context.Context, id int64) (*inventory.Stock, error) {
	for _, s := range m.stocks {
		if s.ID == id {
			return s, nil
		}
	}
	return nil, nil
}

func (m *mockInventoryRepoForWarehouseTest) GetStock(_ context.Context, warehouseID, variantID int64) (*inventory.Stock, error) {
	for _, s := range m.stocks {
		if s.WarehouseID == warehouseID && s.ProductVariantID == variantID {
			return s, nil
		}
	}
	return nil, nil
}

func (m *mockInventoryRepoForWarehouseTest) UpsertStock(_ context.Context, s *inventory.Stock) error {
	m.stocks = append(m.stocks, s)
	return nil
}

func (m *mockInventoryRepoForWarehouseTest) ClearWarehouseStocks(_ context.Context, warehouseID int64) error {
	m.stocks = nil
	m.detailed = nil
	return nil
}

func (m *mockInventoryRepoForWarehouseTest) AdjustStock(_ context.Context, stockID int64, delta int, movement inventory.StockMovement) (*inventory.Stock, error) {
	for _, s := range m.stocks {
		if s.ID == stockID {
			s.Quantity += delta
			return s, nil
		}
	}
	for _, d := range m.detailed {
		if d.StockID == stockID {
			d.Quantity += delta
			st := &inventory.Stock{
				ID:               d.StockID,
				WarehouseID:      d.WarehouseID,
				OrganizationID:   d.OrganizationID,
				ProductID:        d.ProductID,
				ProductVariantID: d.ProductVariantID,
				Quantity:         d.Quantity,
				MinThreshold:     d.MinThreshold,
			}
			return st, nil
		}
	}
	return nil, nil
}

func (m *mockInventoryRepoForWarehouseTest) AvailableQuantity(_ context.Context, variantID int64) (int, error) {
	return 0, nil
}

func (m *mockInventoryRepoForWarehouseTest) AvailableQuantities(_ context.Context, variantIDs []int64) (map[int64]int, error) {
	return map[int64]int{}, nil
}

func (m *mockInventoryRepoForWarehouseTest) ListStocksByWarehouse(_ context.Context, warehouseID int64) ([]*inventory.Stock, error) {
	var list []*inventory.Stock
	for _, s := range m.stocks {
		if s.WarehouseID == warehouseID {
			list = append(list, s)
		}
	}
	return list, nil
}

func (m *mockInventoryRepoForWarehouseTest) ListDetailedStocksByWarehouse(_ context.Context, warehouseID int64) ([]*inventory.DetailedWarehouseStockView, error) {
	var list []*inventory.DetailedWarehouseStockView
	for _, d := range m.detailed {
		if d.WarehouseID == warehouseID {
			list = append(list, d)
		}
	}
	return list, nil
}

func (m *mockInventoryRepoForWarehouseTest) ListStocksByOrg(_ context.Context, orgID int64) ([]*inventory.Stock, error) {
	return m.stocks, nil
}

func (m *mockInventoryRepoForWarehouseTest) ListStocksByOrgWithTotal(_ context.Context, _ int64, _ int64, _ string, _ int, _ int) ([]*inventory.Stock, int, error) {
	return m.stocks, len(m.stocks), nil
}

func (m *mockInventoryRepoForWarehouseTest) ListLowStocksByOrgWithTotal(_ context.Context, _ int64, _ int64, _ string, _ int, _ int) ([]*inventory.Stock, int, error) {
	var low []*inventory.Stock
	for _, s := range m.stocks {
		if s.Quantity <= s.MinThreshold {
			low = append(low, s)
		}
	}
	return low, len(low), nil
}

func (m *mockInventoryRepoForWarehouseTest) ListStockMovements(_ context.Context, stockID int64, limit int) ([]*inventory.StockMovement, error) {
	return nil, nil
}

func (m *mockInventoryRepoForWarehouseTest) ListLowStock(_ context.Context, limit, offset int) ([]*inventory.Stock, error) {
	return nil, nil
}

func (m *mockInventoryRepoForWarehouseTest) ListLowStockWithTotal(_ context.Context, limit, offset int) ([]*inventory.Stock, int, error) {
	return nil, 0, nil
}

func (m *mockInventoryRepoForWarehouseTest) ListMovementsByOrg(_ context.Context, limit, offset int) ([]*inventory.StockMovement, error) {
	return nil, nil
}

func (m *mockInventoryRepoForWarehouseTest) CreateTransfer(_ context.Context, t *inventory.WarehouseTransfer) error {
	return nil
}

func (m *mockInventoryRepoForWarehouseTest) GetTransferByID(_ context.Context, id int64) (*inventory.WarehouseTransfer, error) {
	return nil, nil
}

func (m *mockInventoryRepoForWarehouseTest) UpdateTransferStatus(_ context.Context, id int64, from, to inventory.TransferStatus) error {
	return nil
}

func (m *mockInventoryRepoForWarehouseTest) ListTransfers(_ context.Context, status string, limit, offset int) ([]*inventory.WarehouseTransfer, error) {
	return nil, nil
}

func (m *mockInventoryRepoForWarehouseTest) ListTransfersWithTotal(_ context.Context, status string, limit, offset int) ([]*inventory.WarehouseTransfer, int, error) {
	return nil, 0, nil
}
