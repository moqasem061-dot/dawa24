package inventory_test

import (
	"context"
	"sort"

	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Lifecycle methods of mockInventoryRepo, kept separate from the original mock
// so inventory_test.go stays readable.

func (m *mockInventoryRepo) UpdateWarehouse(_ context.Context, w *inventory.Warehouse) error {
	if _, ok := m.warehouses[w.ID]; !ok {
		return apperr.NotFound("warehouse")
	}
	m.warehouses[w.ID] = w
	return nil
}

func (m *mockInventoryRepo) SoftDeleteWarehouse(_ context.Context, id int64) error {
	if _, ok := m.warehouses[id]; !ok {
		return apperr.NotFound("warehouse")
	}
	delete(m.warehouses, id)
	return nil
}

func (m *mockInventoryRepo) CountStockInWarehouse(_ context.Context, warehouseID int64) (int, error) {
	count := 0
	for _, s := range m.stocksByID {
		if s.WarehouseID == warehouseID && s.Quantity > 0 {
			count++
		}
	}
	return count, nil
}

func (m *mockInventoryRepo) ListLowStock(_ context.Context, limit, offset int) ([]*inventory.Stock, error) {
	var low []*inventory.Stock
	for _, s := range m.stocksByID {
		if s.Quantity <= s.MinThreshold {
			low = append(low, s)
		}
	}
	sort.Slice(low, func(i, j int) bool { return low[i].ID < low[j].ID })
	return pageSlice(low, limit, offset), nil
}

func (m *mockInventoryRepo) ListLowStockWithTotal(_ context.Context, limit, offset int) ([]*inventory.Stock, int, error) {
	var low []*inventory.Stock
	for _, s := range m.stocksByID {
		if s.Quantity <= s.MinThreshold {
			low = append(low, s)
		}
	}
	sort.Slice(low, func(i, j int) bool { return low[i].ID < low[j].ID })
	return pageSlice(low, limit, offset), len(low), nil
}

func (m *mockInventoryRepo) ListMovementsByOrg(_ context.Context, limit, offset int) ([]*inventory.StockMovement, error) {
	var all []*inventory.StockMovement
	for _, ms := range m.movements {
		all = append(all, ms...)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID > all[j].ID })
	return pageSlice(all, limit, offset), nil
}

func (m *mockInventoryRepo) ListTransfers(_ context.Context, status string, limit, offset int) ([]*inventory.WarehouseTransfer, error) {
	var list []*inventory.WarehouseTransfer
	for _, t := range m.transfers {
		if status == "" || string(t.Status) == status {
			list = append(list, t)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ID > list[j].ID })
	return pageSlice(list, limit, offset), nil
}

func (m *mockInventoryRepo) ListTransfersWithTotal(ctx context.Context, status string, limit, offset int) ([]*inventory.WarehouseTransfer, int, error) {
	var list []*inventory.WarehouseTransfer
	for _, t := range m.transfers {
		if status == "" || string(t.Status) == status {
			list = append(list, t)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ID > list[j].ID })
	return pageSlice(list, limit, offset), len(list), nil
}

func (m *mockInventoryRepo) ListDetailedStocksByWarehouse(_ context.Context, warehouseID int64) ([]*inventory.DetailedWarehouseStockView, error) {
	var list []*inventory.DetailedWarehouseStockView
	for _, s := range m.stocksByID {
		if s.WarehouseID == warehouseID {
			list = append(list, &inventory.DetailedWarehouseStockView{
				StockID:          s.ID,
				WarehouseID:      s.WarehouseID,
				OrganizationID:   s.OrganizationID,
				ProductID:        s.ProductID,
				ProductVariantID: s.ProductVariantID,
				Quantity:         s.Quantity,
				MinThreshold:     s.MinThreshold,
				Status:           "active",
			})
		}
	}
	return list, nil
}

func pageSlice[T any](s []T, limit, offset int) []T {
	if offset < 0 {
		offset = 0
	}
	if offset >= len(s) {
		return []T{}
	}
	end := offset + limit
	if limit <= 0 || end > len(s) {
		end = len(s)
	}
	return s[offset:end]
}
