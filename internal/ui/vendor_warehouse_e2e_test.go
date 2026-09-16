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
	"github.com/muhiya/dawa24-store/internal/ui"
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

func TestVendorWarehouseDetailPage_Overhaul_E2E(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := newMockInventoryRepoForWarehouseTest()
	invSvc := inventory.NewService(repo, logger)

	h := ui.NewUIHandler(
		nil, nil, nil, nil, invSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger,
	)

	actor := authctx.Actor{
		UserID:         10,
		OrganizationID: 100,
		OrgType:        "vendor",
		Permissions:    []string{"vendor.*"},
	}

	// Create a test warehouse
	wh := &inventory.Warehouse{
		ID:             1,
		OrganizationID: 100,
		Name:           "مخزن القاهرة الرئيسي",
		Code:           "WH-CAI-01",
		Address:        "المنطقة الصناعية - مدينة نصر",
		Phone:          "01012345678",
		IsActive:       true,
	}
	repo.warehouses[1] = wh

	// Add rich pharmaceutical stocks
	exp1 := time.Now().AddDate(1, 0, 0)  // 1 year in future
	exp2 := time.Now().AddDate(0, 2, 0)  // 2 months in future (expiring soon)
	exp3 := time.Now().AddDate(0, -1, 0) // 1 month in past (expired)

	repo.detailed = []*inventory.DetailedWarehouseStockView{
		{
			StockID:          101,
			WarehouseID:      1,
			OrganizationID:   100,
			ProductID:        1,
			ProductVariantID: 11,
			ProductName:      "بانادول إكسترا 24 قرص",
			VariantName:      "Panadol Extra 24 Tab",
			ScientificName:   "Paracetamol + Caffeine",
			DosageForm:       "أقراص",
			Manufacturer:     "GSK",
			SKU:              "PAN-EXT-24",
			Barcode:          "6221142001234",
			BatchNumber:      "BN-94812",
			ExpiryDate:       &exp1,
			PriceStr:         "48.50",
			PublicPriceStr:   "52.00",
			DiscountStr:      "15.00",
			Quantity:         350,
			MinThreshold:     50,
			Status:           "active",
		},
		{
			StockID:          102,
			WarehouseID:      1,
			OrganizationID:   100,
			ProductID:        2,
			ProductVariantID: 12,
			ProductName:      "أوجمنتين 1 جم 14 قرص",
			VariantName:      "Augmentin 1g 14 Tab",
			ScientificName:   "Amoxicillin + Clavulanate",
			DosageForm:       "أقراص",
			Manufacturer:     "GlaxoSmithKline",
			SKU:              "AUG-1G-14",
			Barcode:          "6221142005678",
			BatchNumber:      "BN-88219",
			ExpiryDate:       &exp2,
			PriceStr:         "132.00",
			PublicPriceStr:   "140.00",
			DiscountStr:      "10.00",
			Quantity:         15, // Low stock (<= 20)
			MinThreshold:     20,
			Status:           "active",
		},
		{
			StockID:          103,
			WarehouseID:      1,
			OrganizationID:   100,
			ProductID:        3,
			ProductVariantID: 13,
			ProductName:      "كونجستال 20 قرص",
			VariantName:      "Congestal 20 Tablets",
			ScientificName:   "Paracetamol + Pseudoephedrine",
			DosageForm:       "أقراص",
			Manufacturer:     "Eva Pharma",
			SKU:              "CONG-20",
			Barcode:          "6221142003322",
			BatchNumber:      "BN-10293",
			ExpiryDate:       &exp3,
			PriceStr:         "25.00",
			PublicPriceStr:   "28.00",
			DiscountStr:      "20.00",
			Quantity:         0, // Out of stock
			MinThreshold:     30,
			Status:           "active",
		},
	}

	repo.stocks = []*inventory.Stock{
		{ID: 101, WarehouseID: 1, OrganizationID: 100, ProductID: 1, ProductVariantID: 11, Quantity: 350, MinThreshold: 50},
		{ID: 102, WarehouseID: 1, OrganizationID: 100, ProductID: 2, ProductVariantID: 12, Quantity: 15, MinThreshold: 20},
		{ID: 103, WarehouseID: 1, OrganizationID: 100, ProductID: 3, ProductVariantID: 13, Quantity: 0, MinThreshold: 30},
	}

	// 1. Test full warehouse detail rendering
	req := httptest.NewRequest("GET", "/vendor/warehouses/1", nil)
	req = req.WithContext(authctx.WithActor(req.Context(), actor))
	rCtx := chi.NewRouteContext()
	rCtx.URLParams.Add("id", "1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rCtx))

	rec := httptest.NewRecorder()
	h.VendorWarehouseDetailPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "مخزن القاهرة الرئيسي") {
		t.Errorf("expected body to contain warehouse name")
	}
	if !strings.Contains(body, "بانادول إكسترا 24 قرص") {
		t.Errorf("expected body to contain Panadol product name")
	}
	if !strings.Contains(body, "سعر الجمهور") || !strings.Contains(body, "نسبة الخصم") || !strings.Contains(body, "سعر بعد الخصم") {
		t.Errorf("expected table to contain separated 'سعر الجمهور', 'نسبة الخصم', and 'سعر بعد الخصم' headers")
	}
	if strings.Contains(body, "line-through") {
		t.Errorf("public price should not contain dash/line-through")
	}
	if !strings.Contains(body, "52.00") || !strings.Contains(body, "%15.00") || !strings.Contains(body, "48.50") {
		t.Errorf("expected separated values: public price 52.00, discount %%15.00, and price after discount 48.50")
	}
	if !strings.Contains(body, "Paracetamol + Caffeine") {
		t.Errorf("expected body to contain scientific name")
	}
	if !strings.Contains(body, "350") {
		t.Errorf("expected body to contain actual quantity 350")
	}
	if !strings.Contains(body, "أوجمنتين 1 جم 14 قرص") {
		t.Errorf("expected body to contain Augmentin")
	}
	if !strings.Contains(body, "كونجستال 20 قرص") {
		t.Errorf("expected body to contain Congestal")
	}

	// 2. Test search filter (search for Augmentin)
	reqSearch := httptest.NewRequest("GET", "/vendor/warehouses/1?q=أوجمنتين", nil)
	reqSearch = reqSearch.WithContext(authctx.WithActor(reqSearch.Context(), actor))
	reqSearch = reqSearch.WithContext(context.WithValue(reqSearch.Context(), chi.RouteCtxKey, rCtx))

	recSearch := httptest.NewRecorder()
	h.VendorWarehouseDetailPage(recSearch, reqSearch)

	searchBody := recSearch.Body.String()
	if !strings.Contains(searchBody, "أوجمنتين 1 جم 14 قرص") {
		t.Errorf("expected search result to contain Augmentin")
	}
	if strings.Contains(searchBody, "بانادول إكسترا 24 قرص") {
		t.Errorf("search result should not contain Panadol when filtering for Augmentin")
	}

	// 3. Test stock adjustment
	form := url.Values{}
	form.Set("new_quantity", "500")
	form.Set("reason", "جرد دوري ومطابقة فعلية")

	reqAdjust := httptest.NewRequest("POST", "/vendor/warehouses/1/stocks/101/adjust", strings.NewReader(form.Encode()))
	reqAdjust.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqAdjust = reqAdjust.WithContext(authctx.WithActor(reqAdjust.Context(), actor))
	rCtxAdj := chi.NewRouteContext()
	rCtxAdj.URLParams.Add("id", "1")
	rCtxAdj.URLParams.Add("stockID", "101")
	reqAdjust = reqAdjust.WithContext(context.WithValue(reqAdjust.Context(), chi.RouteCtxKey, rCtxAdj))

	recAdjust := httptest.NewRecorder()
	h.VendorWarehouseStockAdjustSubmit(recAdjust, reqAdjust)

	if recAdjust.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect after adjust, got %d", recAdjust.Code)
	}

	// Check updated quantity
	for _, s := range repo.stocks {
		if s.ID == 101 && s.Quantity != 500 {
			t.Errorf("expected stock 101 quantity to be 500, got %d", s.Quantity)
		}
	}
}
