package inventory_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

type mockInventoryRepo struct {
	warehouses map[int64]*inventory.Warehouse
	stocks     map[string]*inventory.Stock // key: "warehouseID:variantID"
	stocksByID map[int64]*inventory.Stock
	movements  map[int64][]*inventory.StockMovement
	transfers  map[int64]*inventory.WarehouseTransfer
	nextID     int64
}

func newMockInventoryRepo() *mockInventoryRepo {
	return &mockInventoryRepo{
		warehouses: map[int64]*inventory.Warehouse{},
		stocks:     map[string]*inventory.Stock{},
		stocksByID: map[int64]*inventory.Stock{},
		movements:  map[int64][]*inventory.StockMovement{},
		transfers:  map[int64]*inventory.WarehouseTransfer{},
		nextID:     1,
	}
}

func (m *mockInventoryRepo) CreateWarehouse(_ context.Context, w *inventory.Warehouse) error {
	w.ID = m.nextID
	m.nextID++
	m.warehouses[w.ID] = w
	return nil
}

func (m *mockInventoryRepo) GetWarehouseByID(_ context.Context, id int64) (*inventory.Warehouse, error) {
	w, ok := m.warehouses[id]
	if !ok {
		return nil, apperr.NotFound("warehouse")
	}
	return w, nil
}

func (m *mockInventoryRepo) ListWarehouses(_ context.Context) ([]*inventory.Warehouse, error) {
	var list []*inventory.Warehouse
	for _, w := range m.warehouses {
		list = append(list, w)
	}
	return list, nil
}

func (m *mockInventoryRepo) ListWarehousesWithTotal(_ context.Context, limit, offset int) ([]*inventory.Warehouse, int, error) {
	var list []*inventory.Warehouse
	for _, w := range m.warehouses {
		list = append(list, w)
	}
	return list, len(list), nil
}

func stockKey(warehouseID, variantID int64) string {
	return fmt.Sprintf("%d:%d", warehouseID, variantID)
}

func (m *mockInventoryRepo) GetStockByID(_ context.Context, id int64) (*inventory.Stock, error) {
	for _, s := range m.stocks {
		if s.ID == id {
			return s, nil
		}
	}
	return nil, apperr.NotFound("stock")
}

func (m *mockInventoryRepo) GetStock(_ context.Context, warehouseID, variantID int64) (*inventory.Stock, error) {
	s, ok := m.stocks[stockKey(warehouseID, variantID)]
	if !ok {
		return nil, apperr.NotFound("stock")
	}
	return s, nil
}

func (m *mockInventoryRepo) UpsertStock(_ context.Context, s *inventory.Stock) error {
	if s.ID == 0 {
		s.ID = m.nextID
		m.nextID++
	}
	s.CreatedAt = time.Now()
	s.UpdatedAt = time.Now()
	m.stocks[stockKey(s.WarehouseID, s.ProductVariantID)] = s
	m.stocksByID[s.ID] = s
	return nil
}

func (m *mockInventoryRepo) ClearWarehouseStocks(_ context.Context, warehouseID int64) error {
	for key, s := range m.stocks {
		if s.WarehouseID == warehouseID {
			delete(m.stocks, key)
			delete(m.stocksByID, s.ID)
		}
	}
	return nil
}

func (m *mockInventoryRepo) AdjustStock(_ context.Context, stockID int64, delta int, movement inventory.StockMovement) (*inventory.Stock, error) {
	s, ok := m.stocksByID[stockID]
	if !ok {
		return nil, apperr.NotFound("stock")
	}
	newQty := s.Quantity + delta
	if newQty < 0 {
		return nil, apperr.Validation("stock.insufficient", "Insufficient stock", nil)
	}
	s.Quantity = newQty
	s.UpdatedAt = time.Now()

	movement.ID = m.nextID
	m.nextID++
	movement.StockID = stockID
	movement.QuantityDelta = delta
	movement.BalanceAfter = newQty
	movement.CreatedAt = time.Now()
	m.movements[stockID] = append(m.movements[stockID], &movement)

	return s, nil
}

func (m *mockInventoryRepo) ListStocksByWarehouse(_ context.Context, warehouseID int64) ([]*inventory.Stock, error) {
	var list []*inventory.Stock
	for _, s := range m.stocksByID {
		if s.WarehouseID == warehouseID {
			list = append(list, s)
		}
	}
	return list, nil
}

func (m *mockInventoryRepo) ListStocksByOrg(_ context.Context, orgID int64) ([]*inventory.Stock, error) {
	var list []*inventory.Stock
	for _, s := range m.stocksByID {
		if s.OrganizationID == orgID {
			list = append(list, s)
		}
	}
	return list, nil
}

func (m *mockInventoryRepo) ListStocksByOrgWithTotal(ctx context.Context, orgID int64, _ int64, _ string, limit, offset int) ([]*inventory.Stock, int, error) {
	list, err := m.ListStocksByOrg(ctx, orgID)
	return list, len(list), err
}

func (m *mockInventoryRepo) ListLowStocksByOrgWithTotal(ctx context.Context, orgID int64, _ int64, _ string, limit, offset int) ([]*inventory.Stock, int, error) {
	list, err := m.ListStocksByOrg(ctx, orgID)
	var low []*inventory.Stock
	for _, s := range list {
		if s.Quantity <= s.MinThreshold {
			low = append(low, s)
		}
	}
	return low, len(low), err
}

func (m *mockInventoryRepo) ListStockMovements(_ context.Context, stockID int64, limit int) ([]*inventory.StockMovement, error) {
	return m.movements[stockID], nil
}

func (m *mockInventoryRepo) CreateTransfer(_ context.Context, t *inventory.WarehouseTransfer) error {
	t.ID = m.nextID
	m.nextID++
	t.CreatedAt = time.Now()
	m.transfers[t.ID] = t
	return nil
}

func (m *mockInventoryRepo) GetTransferByID(_ context.Context, id int64) (*inventory.WarehouseTransfer, error) {
	t, ok := m.transfers[id]
	if !ok {
		return nil, apperr.NotFound("warehouse_transfer")
	}
	// Return a copy, as a real repository does when it scans a row into a fresh
	// struct. Handing back the stored pointer would let a caller's in-memory
	// TransitionTo mutate the "database" before the compare-and-swap ran,
	// making the CAS impossible to test.
	copied := *t
	return &copied, nil
}

// UpdateTransferStatus mirrors the PostgreSQL compare-and-swap. The `from`
// check is not decoration: without it here, the tests would pass while the real
// implementation was the only thing preventing a double credit.
func (m *mockInventoryRepo) UpdateTransferStatus(_ context.Context, id int64, from, to inventory.TransferStatus) error {
	t, ok := m.transfers[id]
	if !ok {
		return apperr.NotFound("warehouse_transfer")
	}
	if t.Status != from {
		return apperr.Conflict("transfer.state_changed",
			"This transfer was already updated by someone else. Reload and try again.")
	}
	t.Status = to
	t.UpdatedAt = time.Now()
	return nil
}

// TestD4SiblingVariantsCoexist proves the legacy D4 bug fix:
// Multiple variants of the SAME product can coexist in the same warehouse.
func TestD4SiblingVariantsCoexist(t *testing.T) {
	ctx := database.WithTenant(context.Background(), 10)
	repo := newMockInventoryRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := inventory.NewService(repo, logger)

	warehouse, err := svc.CreateWarehouse(ctx, &inventory.Warehouse{
		Name: "Main Cairo Central Warehouse",
		Code: "WH-CAI-01",
	})
	if err != nil {
		t.Fatalf("CreateWarehouse failed: %v", err)
	}

	const productID int64 = 100
	const variant500mg int64 = 501
	const variant1000mg int64 = 502

	// Variant 1
	stock1 := &inventory.Stock{
		OrganizationID:   10,
		WarehouseID:      warehouse.ID,
		ProductID:        productID,
		ProductVariantID: variant500mg,
		Quantity:         150,
	}
	if err := repo.UpsertStock(ctx, stock1); err != nil {
		t.Fatalf("Failed to upsert variant 500mg: %v", err)
	}

	// Sibling Variant 2 in the same warehouse (which failed in legacy schema under UNIQUE(product_id, warehouse_id))
	stock2 := &inventory.Stock{
		OrganizationID:   10,
		WarehouseID:      warehouse.ID,
		ProductID:        productID,
		ProductVariantID: variant1000mg,
		Quantity:         75,
	}
	if err := repo.UpsertStock(ctx, stock2); err != nil {
		t.Fatalf("Failed to upsert variant 1000mg: %v", err)
	}

	// Verify both stocks exist independently
	retrieved1, err := repo.GetStock(ctx, warehouse.ID, variant500mg)
	if err != nil || retrieved1.Quantity != 150 {
		t.Errorf("Variant 500mg not found or wrong qty: %v", err)
	}

	retrieved2, err := repo.GetStock(ctx, warehouse.ID, variant1000mg)
	if err != nil || retrieved2.Quantity != 75 {
		t.Errorf("Variant 1000mg not found or wrong qty: %v", err)
	}
}

// TestInterWarehouseTransfer verifies transfer mechanics and double-entry movement ledger.
func TestInterWarehouseTransfer(t *testing.T) {
	ctx := database.WithTenant(context.Background(), 10)
	repo := newMockInventoryRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := inventory.NewService(repo, logger)

	wh1, _ := svc.CreateWarehouse(ctx, &inventory.Warehouse{Name: "Warehouse 1"})
	wh2, _ := svc.CreateWarehouse(ctx, &inventory.Warehouse{Name: "Warehouse 2"})

	const productID int64 = 100
	const variantID int64 = 501

	// Initial stock at WH1 = 100
	stockWH1 := &inventory.Stock{
		OrganizationID:   10,
		WarehouseID:      wh1.ID,
		ProductID:        productID,
		ProductVariantID: variantID,
		Quantity:         100,
	}
	_ = repo.UpsertStock(ctx, stockWH1)

	// Execute transfer of 30 units from WH1 to WH2
	transfer := &inventory.WarehouseTransfer{
		FromWarehouseID:  wh1.ID,
		ToWarehouseID:    wh2.ID,
		ProductID:        productID,
		ProductVariantID: variantID,
		Quantity:         30,
	}

	res, err := svc.TransferStock(ctx, transfer)
	if err != nil {
		t.Fatalf("TransferStock failed: %v", err)
	}
	// Transfers are two-phase: dispatch deducts the source and leaves the goods
	// in transit. The destination is credited by ReceiveTransfer, not here.
	// See inventory/transfer_state.go.
	if res.Status != inventory.TransferInTransit {
		t.Errorf("Transfer status = %s; want in_transit", res.Status)
	}

	if _, err := svc.ReceiveTransfer(ctx, res.ID); err != nil {
		t.Fatalf("ReceiveTransfer failed: %v", err)
	}

	// Check WH1 balance = 70
	s1, _ := repo.GetStock(ctx, wh1.ID, variantID)
	if s1.Quantity != 70 {
		t.Errorf("WH1 quantity = %d; want 70", s1.Quantity)
	}

	// Check WH2 balance = 30
	s2, _ := repo.GetStock(ctx, wh2.ID, variantID)
	if s2.Quantity != 30 {
		t.Errorf("WH2 quantity = %d; want 30", s2.Quantity)
	}

	// Check movement ledgers
	m1, _ := svc.ListStockMovements(ctx, s1.ID, 10)
	if len(m1) != 1 || m1[0].QuantityDelta != -30 {
		t.Errorf("WH1 movement delta = %v; want -30", m1)
	}

	m2, _ := svc.ListStockMovements(ctx, s2.ID, 10)
	if len(m2) != 1 || m2[0].QuantityDelta != 30 {
		t.Errorf("WH2 movement delta = %v; want +30", m2)
	}
}

// AvailableQuantity stub. Real stock totalling is covered by the repository
// integration tests; catalog.ProductVariant.StockQty is never populated, which
// is why this lookup exists at all.
func (m *mockInventoryRepo) AvailableQuantity(context.Context, int64) (int, error) {
	return 0, nil
}

func (m *mockInventoryRepo) AvailableQuantities(context.Context, []int64) (map[int64]int, error) {
	return map[int64]int{}, nil
}
