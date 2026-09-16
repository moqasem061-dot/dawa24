package ui_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/ui"
)

type mockTransferRepo struct {
	warehouses map[int64]*inventory.Warehouse
	stocks     map[int64]*inventory.Stock
	transfers  map[int64]*inventory.WarehouseTransfer
	movements  []*inventory.StockMovement
	nextID     int64
}

func newMockTransferRepo() *mockTransferRepo {
	return &mockTransferRepo{
		warehouses: make(map[int64]*inventory.Warehouse),
		stocks:     make(map[int64]*inventory.Stock),
		transfers:  make(map[int64]*inventory.WarehouseTransfer),
	}
}

func (m *mockTransferRepo) CreateWarehouse(_ context.Context, w *inventory.Warehouse) error {
	m.nextID++
	w.ID = m.nextID
	m.warehouses[w.ID] = w
	return nil
}
func (m *mockTransferRepo) UpdateWarehouse(_ context.Context, w *inventory.Warehouse) error {
	m.warehouses[w.ID] = w
	return nil
}
func (m *mockTransferRepo) GetWarehouseByID(_ context.Context, id int64) (*inventory.Warehouse, error) {
	return m.warehouses[id], nil
}
func (m *mockTransferRepo) ListWarehouses(_ context.Context) ([]*inventory.Warehouse, error) {
	var list []*inventory.Warehouse
	for _, w := range m.warehouses {
		list = append(list, w)
	}
	return list, nil
}
func (m *mockTransferRepo) ListWarehousesWithTotal(_ context.Context, limit, offset int) ([]*inventory.Warehouse, int, error) {
	list, _ := m.ListWarehouses(context.Background())
	return list, len(list), nil
}
func (m *mockTransferRepo) SoftDeleteWarehouse(_ context.Context, id int64) error {
	delete(m.warehouses, id)
	return nil
}
func (m *mockTransferRepo) CountStockInWarehouse(_ context.Context, warehouseID int64) (int, error) {
	return 0, nil
}
func (m *mockTransferRepo) ListMovementsByOrg(_ context.Context, limit, offset int) ([]*inventory.StockMovement, error) {
	return m.movements, nil
}
func (m *mockTransferRepo) GetStock(_ context.Context, warehouseID, variantID int64) (*inventory.Stock, error) {
	for _, s := range m.stocks {
		if s.WarehouseID == warehouseID && s.ProductVariantID == variantID && s.DeletedAt == nil {
			return s, nil
		}
	}
	return nil, nil
}
func (m *mockTransferRepo) GetStockByID(_ context.Context, id int64) (*inventory.Stock, error) {
	return m.stocks[id], nil
}
func (m *mockTransferRepo) UpsertStock(_ context.Context, s *inventory.Stock) error {
	if s.ID == 0 {
		m.nextID++
		s.ID = m.nextID
	}
	m.stocks[s.ID] = s
	return nil
}
func (m *mockTransferRepo) AdjustStock(_ context.Context, stockID int64, delta int, movement inventory.StockMovement) (*inventory.Stock, error) {
	s := m.stocks[stockID]
	s.Quantity += delta
	movement.StockID = stockID
	m.movements = append(m.movements, &movement)
	return s, nil
}
func (m *mockTransferRepo) ListStocksByWarehouse(_ context.Context, warehouseID int64) ([]*inventory.Stock, error) {
	var list []*inventory.Stock
	for _, s := range m.stocks {
		if s.WarehouseID == warehouseID && s.DeletedAt == nil {
			list = append(list, s)
		}
	}
	return list, nil
}
func (m *mockTransferRepo) ListDetailedStocksByWarehouse(_ context.Context, warehouseID int64) ([]*inventory.DetailedWarehouseStockView, error) {
	var list []*inventory.DetailedWarehouseStockView
	for _, s := range m.stocks {
		if s.WarehouseID == warehouseID && s.DeletedAt == nil {
			list = append(list, &inventory.DetailedWarehouseStockView{
				StockID:          s.ID,
				WarehouseID:      s.WarehouseID,
				ProductID:        s.ProductID,
				ProductVariantID: s.ProductVariantID,
				ProductName:      "دواء تجريبي",
				Quantity:         s.Quantity,
			})
		}
	}
	return list, nil
}
func (m *mockTransferRepo) ListStocksByOrg(_ context.Context, orgID int64) ([]*inventory.Stock, error) {
	var list []*inventory.Stock
	for _, s := range m.stocks {
		if s.OrganizationID == orgID && s.DeletedAt == nil {
			list = append(list, s)
		}
	}
	return list, nil
}
func (m *mockTransferRepo) ListStocksByOrgWithTotal(_ context.Context, orgID, whID int64, search string, limit, offset int) ([]*inventory.Stock, int, error) {
	stocks, _ := m.ListStocksByOrg(context.Background(), orgID)
	return stocks, len(stocks), nil
}
func (m *mockTransferRepo) ListLowStocksByOrgWithTotal(_ context.Context, orgID, whID int64, search string, limit, offset int) ([]*inventory.Stock, int, error) {
	stocks, _ := m.ListStocksByOrg(context.Background(), orgID)
	var low []*inventory.Stock
	for _, s := range stocks {
		if s.Quantity <= s.MinThreshold {
			low = append(low, s)
		}
	}
	return low, len(low), nil
}
func (m *mockTransferRepo) ListStockMovements(_ context.Context, stockID int64, limit int) ([]*inventory.StockMovement, error) {
	return m.movements, nil
}
func (m *mockTransferRepo) ListOrgMovements(_ context.Context, limit, offset int) ([]*inventory.StockMovement, error) {
	return m.movements, nil
}
func (m *mockTransferRepo) ListLowStock(_ context.Context, limit, offset int) ([]*inventory.Stock, error) {
	return nil, nil
}
func (m *mockTransferRepo) ListLowStockWithTotal(_ context.Context, limit, offset int) ([]*inventory.Stock, int, error) {
	return nil, 0, nil
}
func (m *mockTransferRepo) BulkWriteStocks(_ context.Context, stocks []*inventory.Stock) (int, error) {
	for _, s := range stocks {
		_ = m.UpsertStock(context.Background(), s)
	}
	return len(stocks), nil
}
func (m *mockTransferRepo) ClearWarehouseStocks(_ context.Context, whID int64) error {
	for _, s := range m.stocks {
		if s.WarehouseID == whID {
			now := time.Now()
			s.DeletedAt = &now
		}
	}
	return nil
}
func (m *mockTransferRepo) CreateTransfer(_ context.Context, t *inventory.WarehouseTransfer) error {
	m.nextID++
	t.ID = m.nextID
	t.CreatedAt = time.Now()
	t.UpdatedAt = time.Now()
	m.transfers[t.ID] = t
	return nil
}
func (m *mockTransferRepo) GetTransferByID(_ context.Context, id int64) (*inventory.WarehouseTransfer, error) {
	return m.transfers[id], nil
}
func (m *mockTransferRepo) UpdateTransferStatus(_ context.Context, id int64, from, to inventory.TransferStatus) error {
	if t, ok := m.transfers[id]; ok {
		t.Status = to
		t.UpdatedAt = time.Now()
	}
	return nil
}
func (m *mockTransferRepo) ListTransfers(_ context.Context, status string, limit, offset int) ([]*inventory.WarehouseTransfer, error) {
	var list []*inventory.WarehouseTransfer
	for _, t := range m.transfers {
		if status == "" || string(t.Status) == status {
			list = append(list, t)
		}
	}
	return list, nil
}
func (m *mockTransferRepo) ListTransfersWithTotal(_ context.Context, status string, limit, offset int) ([]*inventory.WarehouseTransfer, int, error) {
	list, _ := m.ListTransfers(context.Background(), status, limit, offset)
	return list, len(list), nil
}
func (m *mockTransferRepo) AvailableQuantity(_ context.Context, variantID int64) (int, error) {
	return 0, nil
}
func (m *mockTransferRepo) AvailableQuantities(_ context.Context, variantIDs []int64) (map[int64]int, error) {
	return map[int64]int{}, nil
}

func TestVendorStockTransferSubmit_EndToEnd(t *testing.T) {
	mockRepo := newMockTransferRepo()
	testLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	invSvc := inventory.NewService(mockRepo, testLogger)

	// Create 2 warehouses
	wh1 := &inventory.Warehouse{OrganizationID: 55, Name: "المخزن الرئيسي", Code: "WH-1", IsActive: true}
	wh2 := &inventory.Warehouse{OrganizationID: 55, Name: "مستودع الجيزة", Code: "WH-2", IsActive: true}
	_ = mockRepo.CreateWarehouse(context.Background(), wh1)
	_ = mockRepo.CreateWarehouse(context.Background(), wh2)

	// Stock in WH 1
	st1 := &inventory.Stock{
		OrganizationID:   55,
		WarehouseID:      wh1.ID,
		ProductID:        100,
		ProductVariantID: 200,
		Quantity:         50,
	}
	_ = mockRepo.UpsertStock(context.Background(), st1)

	h := ui.NewUIHandler(
		nil, nil, nil, nil, invSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, testLogger,
	)

	t.Run("Validation - Same Warehouse", func(t *testing.T) {
		form := url.Values{
			"from_warehouse_id": {"1"},
			"to_warehouse_id":   {"1"},
			"variant_id":        {"200"},
			"product_id":        {"100"},
			"quantity":          {"10"},
		}
		req := httptest.NewRequest(http.MethodPost, "/vendor/inventory/transfer", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		ctx := authctx.WithActor(req.Context(), authctx.Actor{UserID: 1, OrganizationID: 55})
		ctx = database.WithTenant(ctx, 55)
		req = req.WithContext(ctx)

		rec := httptest.NewRecorder()
		h.VendorStockTransferSubmit(rec, req)

		if rec.Code != http.StatusSeeOther {
			t.Errorf("expected 303 redirect, got %d", rec.Code)
		}
		loc := rec.Header().Get("Location")
		if !strings.Contains(loc, "notice=error") {
			t.Errorf("expected notice=error in redirect, got %s", loc)
		}
	})

	t.Run("Successful Transfer and Immediate Receipt", func(t *testing.T) {
		form := url.Values{
			"from_warehouse_id": {"1"},
			"to_warehouse_id":   {"2"},
			"variant_id":        {"200"},
			"product_id":        {"100"},
			"quantity":          {"15"},
			"immediate":         {"true"},
			"notes":             {"تحويل تجريبي لتغذية الفرع"},
		}
		req := httptest.NewRequest(http.MethodPost, "/vendor/inventory/transfer", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		ctx := authctx.WithActor(req.Context(), authctx.Actor{UserID: 1, OrganizationID: 55})
		ctx = database.WithTenant(ctx, 55)
		req = req.WithContext(ctx)

		rec := httptest.NewRecorder()
		h.VendorStockTransferSubmit(rec, req)

		if rec.Code != http.StatusSeeOther {
			t.Fatalf("expected 303 redirect, got %d", rec.Code)
		}
		loc := rec.Header().Get("Location")
		if !strings.Contains(loc, "notice=") && !strings.Contains(loc, "success=") {
			t.Fatalf("expected success in redirect, got %s", loc)
		}

		// Source should be deducted by 15: 50 - 15 = 35
		fromStock, _ := mockRepo.GetStock(context.Background(), 1, 200)
		if fromStock == nil || fromStock.Quantity != 35 {
			t.Fatalf("expected source stock 35, got %v", fromStock)
		}

		// Destination should be credited by 15
		toStock, _ := mockRepo.GetStock(context.Background(), 2, 200)
		if toStock == nil || toStock.Quantity != 15 {
			t.Fatalf("expected destination stock 15, got %v", toStock)
		}

		// Check transfer record
		transfers, err := mockRepo.ListTransfers(context.Background(), "", 10, 0)
		if err != nil || len(transfers) != 1 {
			t.Fatalf("expected 1 transfer recorded, got %d (err: %v)", len(transfers), err)
		}
		if transfers[0].Status != inventory.TransferCompleted {
			t.Errorf("expected transfer status completed, got %s", transfers[0].Status)
		}
		if transfers[0].Notes != "تحويل تجريبي لتغذية الفرع" {
			t.Errorf("expected transfer notes preserved, got %q", transfers[0].Notes)
		}
	})

	t.Run("VendorTransfersPage Render", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/vendor/transfers", nil)
		ctx := authctx.WithActor(req.Context(), authctx.Actor{UserID: 1, OrganizationID: 55})
		ctx = database.WithTenant(ctx, 55)
		req = req.WithContext(ctx)

		rec := httptest.NewRecorder()
		h.VendorTransfersPage(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "سجلات حركة المخازن والتحويلات") {
			t.Errorf("expected page title in HTML body")
		}
		if !strings.Contains(body, "المخزن الرئيسي") || !strings.Contains(body, "مستودع الجيزة") {
			t.Errorf("expected resolved warehouse names in transfer table")
		}
	})

	t.Run("VendorWarehouseDetailPage Operations Column Has Transfer Button", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/vendor/warehouses/1", nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "1")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		ctx := authctx.WithActor(req.Context(), authctx.Actor{UserID: 1, OrganizationID: 55})
		ctx = database.WithTenant(ctx, 55)
		req = req.WithContext(ctx)

		rec := httptest.NewRecorder()
		h.VendorWarehouseDetailPage(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "نقل المخزون") {
			t.Errorf("expected 'نقل المخزون' button in warehouse detail table actions")
		}
		if !strings.Contains(body, "transferModal") {
			t.Errorf("expected transferModal in warehouse detail page")
		}
	})
}
