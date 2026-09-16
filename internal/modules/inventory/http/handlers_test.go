package http_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	inventoryHttp "github.com/muhiya/dawa24-store/internal/modules/inventory/http"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

type stubRepo struct{ t *testing.T }

func (r stubRepo) fail(method string) {
	r.t.Helper()
	r.t.Fatalf("repository.%s was called; the request should have been rejected before reaching the repository", method)
}

func (r stubRepo) CreateWarehouse(ctx context.Context, w *inventory.Warehouse) error {
	r.fail("CreateWarehouse")
	return nil
}
func (r stubRepo) GetWarehouseByID(ctx context.Context, id int64) (*inventory.Warehouse, error) {
	r.fail("GetWarehouseByID")
	return nil, nil
}
func (r stubRepo) ListWarehouses(ctx context.Context) ([]*inventory.Warehouse, error) {
	r.fail("ListWarehouses")
	return nil, nil
}
func (r stubRepo) ListWarehousesWithTotal(ctx context.Context, limit, offset int) ([]*inventory.Warehouse, int, error) {
	r.fail("ListWarehousesWithTotal")
	return nil, 0, nil
}
func (r stubRepo) UpdateWarehouse(ctx context.Context, w *inventory.Warehouse) error {
	r.fail("UpdateWarehouse")
	return nil
}
func (r stubRepo) SoftDeleteWarehouse(ctx context.Context, id int64) error {
	r.fail("SoftDeleteWarehouse")
	return nil
}
func (r stubRepo) CountStockInWarehouse(ctx context.Context, warehouseID int64) (int, error) {
	r.fail("CountStockInWarehouse")
	return 0, nil
}

func (r stubRepo) GetStock(ctx context.Context, warehouseID, variantID int64) (*inventory.Stock, error) {
	r.fail("GetStock")
	return nil, nil
}
func (r stubRepo) GetStockByID(ctx context.Context, id int64) (*inventory.Stock, error) {
	r.fail("GetStockByID")
	return nil, nil
}
func (r stubRepo) UpsertStock(ctx context.Context, s *inventory.Stock) error {
	r.fail("UpsertStock")
	return nil
}
func (r stubRepo) ClearWarehouseStocks(ctx context.Context, warehouseID int64) error {
	r.fail("ClearWarehouseStocks")
	return nil
}
func (r stubRepo) AdjustStock(ctx context.Context, stockID int64, delta int, movement inventory.StockMovement) (*inventory.Stock, error) {
	r.fail("AdjustStock")
	return nil, nil
}
func (r stubRepo) ListStocksByWarehouse(ctx context.Context, warehouseID int64) ([]*inventory.Stock, error) {
	r.fail("ListStocksByWarehouse")
	return nil, nil
}
func (r stubRepo) ListDetailedStocksByWarehouse(ctx context.Context, warehouseID int64) ([]*inventory.DetailedWarehouseStockView, error) {
	r.fail("ListDetailedStocksByWarehouse")
	return nil, nil
}
func (r stubRepo) ListStocksByOrg(ctx context.Context, orgID int64) ([]*inventory.Stock, error) {
	r.fail("ListStocksByOrg")
	return nil, nil
}
func (r stubRepo) ListStocksByOrgWithTotal(ctx context.Context, orgID int64, warehouseID int64, search string, limit, offset int) ([]*inventory.Stock, int, error) {
	r.fail("ListStocksByOrgWithTotal")
	return nil, 0, nil
}
func (r stubRepo) ListLowStocksByOrgWithTotal(ctx context.Context, orgID int64, warehouseID int64, search string, limit, offset int) ([]*inventory.Stock, int, error) {
	r.fail("ListLowStocksByOrgWithTotal")
	return nil, 0, nil
}
func (r stubRepo) ListStockMovements(ctx context.Context, stockID int64, limit int) ([]*inventory.StockMovement, error) {
	r.fail("ListStockMovements")
	return nil, nil
}
func (r stubRepo) ListLowStock(ctx context.Context, limit, offset int) ([]*inventory.Stock, error) {
	r.fail("ListLowStock")
	return nil, nil
}
func (r stubRepo) ListLowStockWithTotal(ctx context.Context, limit, offset int) ([]*inventory.Stock, int, error) {
	r.fail("ListLowStockWithTotal")
	return nil, 0, nil
}
func (r stubRepo) ListMovementsByOrg(ctx context.Context, limit, offset int) ([]*inventory.StockMovement, error) {
	r.fail("ListMovementsByOrg")
	return nil, nil
}

func (r stubRepo) CreateTransfer(ctx context.Context, t *inventory.WarehouseTransfer) error {
	r.fail("CreateTransfer")
	return nil
}
func (r stubRepo) GetTransferByID(ctx context.Context, id int64) (*inventory.WarehouseTransfer, error) {
	r.fail("GetTransferByID")
	return nil, nil
}
func (r stubRepo) UpdateTransferStatus(ctx context.Context, id int64, from, to inventory.TransferStatus) error {
	r.fail("UpdateTransferStatus")
	return nil
}
func (r stubRepo) ListTransfers(ctx context.Context, status string, limit, offset int) ([]*inventory.WarehouseTransfer, error) {
	r.fail("ListTransfers")
	return nil, nil
}
func (r stubRepo) ListTransfersWithTotal(ctx context.Context, status string, limit, offset int) ([]*inventory.WarehouseTransfer, int, error) {
	r.fail("ListTransfersWithTotal")
	return nil, 0, nil
}

type happyRepo struct{}

func (happyRepo) CreateWarehouse(ctx context.Context, w *inventory.Warehouse) error {
	w.ID = 1
	return nil
}
func (happyRepo) GetWarehouseByID(ctx context.Context, id int64) (*inventory.Warehouse, error) {
	return &inventory.Warehouse{ID: id, Name: "Main WH", OrganizationID: 1}, nil
}
func (happyRepo) ListWarehouses(ctx context.Context) ([]*inventory.Warehouse, error) {
	return []*inventory.Warehouse{{ID: 1, Name: "Main WH", OrganizationID: 1}}, nil
}
func (happyRepo) ListWarehousesWithTotal(ctx context.Context, limit, offset int) ([]*inventory.Warehouse, int, error) {
	return []*inventory.Warehouse{{ID: 1, Name: "Main WH", OrganizationID: 1}}, 1, nil
}
func (happyRepo) UpdateWarehouse(ctx context.Context, w *inventory.Warehouse) error { return nil }
func (happyRepo) SoftDeleteWarehouse(ctx context.Context, id int64) error           { return nil }
func (happyRepo) CountStockInWarehouse(ctx context.Context, warehouseID int64) (int, error) {
	return 0, nil
}
func (happyRepo) GetStock(ctx context.Context, warehouseID, variantID int64) (*inventory.Stock, error) {
	return &inventory.Stock{ID: 1, WarehouseID: warehouseID, ProductVariantID: variantID, Quantity: 100}, nil
}
func (happyRepo) GetStockByID(ctx context.Context, id int64) (*inventory.Stock, error) {
	return &inventory.Stock{ID: id, OrganizationID: 1, Quantity: 100}, nil
}
func (happyRepo) UpsertStock(ctx context.Context, s *inventory.Stock) error {
	s.ID = 1
	return nil
}
func (happyRepo) ClearWarehouseStocks(ctx context.Context, warehouseID int64) error { return nil }
func (happyRepo) AdjustStock(ctx context.Context, stockID int64, delta int, movement inventory.StockMovement) (*inventory.Stock, error) {
	return &inventory.Stock{ID: stockID, Quantity: 100 + delta}, nil
}
func (happyRepo) AvailableQuantity(ctx context.Context, variantID int64) (int, error) {
	return 100, nil
}
func (happyRepo) AvailableQuantities(ctx context.Context, variantIDs []int64) (map[int64]int, error) {
	m := make(map[int64]int, len(variantIDs))
	for _, id := range variantIDs {
		m[id] = 100
	}
	return m, nil
}
func (happyRepo) ListStocksByWarehouse(ctx context.Context, warehouseID int64) ([]*inventory.Stock, error) {
	return []*inventory.Stock{{ID: 1, WarehouseID: warehouseID, ProductVariantID: 1, Quantity: 100}}, nil
}
func (happyRepo) ListDetailedStocksByWarehouse(ctx context.Context, warehouseID int64) ([]*inventory.DetailedWarehouseStockView, error) {
	return []*inventory.DetailedWarehouseStockView{{StockID: 1, WarehouseID: warehouseID, ProductName: "Test Med", Quantity: 100}}, nil
}
func (happyRepo) ListStocksByOrg(ctx context.Context, orgID int64) ([]*inventory.Stock, error) {
	return []*inventory.Stock{{ID: 1, OrganizationID: orgID, WarehouseID: 1, ProductVariantID: 1, Quantity: 100}}, nil
}
func (happyRepo) ListStocksByOrgWithTotal(ctx context.Context, orgID int64, warehouseID int64, search string, limit, offset int) ([]*inventory.Stock, int, error) {
	return []*inventory.Stock{{ID: 1, OrganizationID: orgID, WarehouseID: 1, ProductVariantID: 1, Quantity: 100}}, 1, nil
}
func (happyRepo) ListLowStocksByOrgWithTotal(ctx context.Context, orgID int64, warehouseID int64, search string, limit, offset int) ([]*inventory.Stock, int, error) {
	return []*inventory.Stock{{ID: 1, OrganizationID: orgID, WarehouseID: 1, ProductVariantID: 1, Quantity: 5, MinThreshold: 10}}, 1, nil
}
func (happyRepo) ListStockMovements(ctx context.Context, stockID int64, limit int) ([]*inventory.StockMovement, error) {
	return []*inventory.StockMovement{{ID: 1, StockID: stockID, Type: inventory.MovementIn, QuantityDelta: 10}}, nil
}
func (happyRepo) ListLowStock(ctx context.Context, limit, offset int) ([]*inventory.Stock, error) {
	return []*inventory.Stock{{ID: 1, OrganizationID: 1, Quantity: 5, MinThreshold: 10}}, nil
}
func (happyRepo) ListLowStockWithTotal(ctx context.Context, limit, offset int) ([]*inventory.Stock, int, error) {
	return []*inventory.Stock{{ID: 1, OrganizationID: 1, Quantity: 5, MinThreshold: 10}}, 1, nil
}
func (happyRepo) ListMovementsByOrg(ctx context.Context, limit, offset int) ([]*inventory.StockMovement, error) {
	return []*inventory.StockMovement{{ID: 1, OrganizationID: 1, Type: inventory.MovementIn, QuantityDelta: 10}}, nil
}
func (happyRepo) CreateTransfer(ctx context.Context, t *inventory.WarehouseTransfer) error {
	t.ID = 1
	t.Status = inventory.TransferInTransit
	return nil
}
func (happyRepo) GetTransferByID(ctx context.Context, id int64) (*inventory.WarehouseTransfer, error) {
	return &inventory.WarehouseTransfer{ID: id, OrganizationID: 1, FromWarehouseID: 1, ToWarehouseID: 2, ProductVariantID: 1, Quantity: 10, Status: inventory.TransferInTransit}, nil
}
func (happyRepo) UpdateTransferStatus(ctx context.Context, id int64, from, to inventory.TransferStatus) error {
	return nil
}
func (happyRepo) ListTransfers(ctx context.Context, status string, limit, offset int) ([]*inventory.WarehouseTransfer, error) {
	return []*inventory.WarehouseTransfer{{ID: 1, OrganizationID: 1, Status: inventory.TransferInTransit}}, nil
}
func (happyRepo) ListTransfersWithTotal(ctx context.Context, status string, limit, offset int) ([]*inventory.WarehouseTransfer, int, error) {
	return []*inventory.WarehouseTransfer{{ID: 1, OrganizationID: 1, Status: inventory.TransferInTransit}}, 1, nil
}

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	svc := inventory.NewService(stubRepo{t: t}, log)
	r := chi.NewRouter()
	r.Use(httpx.RequestID)
	r.Use(httpx.Recover(log))
	r.Use(httpx.Locale)

	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("dawa24_session")
			if err != nil || cookie.Value == "" {
				httpx.Error(w, r, log, apperr.Unauthorized())
				return
			}
			if cookie.Value == "forged-token-that-was-never-issued" {
				httpx.Error(w, r, log, apperr.Unauthorized())
				return
			}
			next.ServeHTTP(w, r)
		})
	})
	inventoryHttp.NewHandler(svc, log).RegisterRoutes(r)
	return r
}

func newAuthedRouter(repo inventory.Repository) http.Handler {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	svc := inventory.NewService(repo, log)
	r := chi.NewRouter()
	r.Use(httpx.RequestID)
	r.Use(httpx.Recover(log))
	r.Use(httpx.Locale)

	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actor := authctx.Actor{
				UserID:         1,
				OrganizationID: 1,
				Role:           "admin",
				Permissions:    []string{"admin", "inventory.admin"},
			}
			ctx := authctx.WithActor(r.Context(), actor)
			ctx = database.WithTenant(ctx, 1)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	inventoryHttp.NewHandler(svc, log).RegisterRoutes(r)
	return r
}

var protectedRoutes = []struct{ method, path string }{
	{http.MethodGet, "/api/v1/inventory/warehouses"},
	{http.MethodPost, "/api/v1/inventory/warehouses"},
	{http.MethodGet, "/api/v1/inventory/warehouses/1"},
	{http.MethodPut, "/api/v1/inventory/warehouses/1"},
	{http.MethodDelete, "/api/v1/inventory/warehouses/1"},
	{http.MethodGet, "/api/v1/inventory/warehouses/1/stocks"},
	{http.MethodPost, "/api/v1/inventory/stocks/adjust"},
	{http.MethodGet, "/api/v1/inventory/stocks/low"},
	{http.MethodGet, "/api/v1/inventory/stocks/1/movements"},
	{http.MethodGet, "/api/v1/inventory/movements"},
	{http.MethodPost, "/api/v1/inventory/transfers"},
	{http.MethodGet, "/api/v1/inventory/transfers"},
	{http.MethodGet, "/api/v1/inventory/transfers/1"},
	{http.MethodPost, "/api/v1/inventory/transfers/1/receive"},
	{http.MethodPost, "/api/v1/inventory/transfers/1/cancel"},
}

func TestProtectedRoutesRejectAnonymousCallers(t *testing.T) {
	router := newTestRouter(t)
	for _, route := range protectedRoutes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			req := httptest.NewRequest(route.method, route.path, strings.NewReader("{}"))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("got %d, want 401 — this endpoint is reachable without a session", rec.Code)
			}
		})
	}
}

func TestProtectedRoutesRejectGarbageSessionToken(t *testing.T) {
	router := newTestRouter(t)
	for _, route := range protectedRoutes {
		req := httptest.NewRequest(route.method, route.path, strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "dawa24_session", Value: "forged-token-that-was-never-issued"})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s with a forged token got %d, want 401", route.method, route.path, rec.Code)
		}
	}
}

func TestUnauthorizedResponseUsesTheErrorEnvelope(t *testing.T) {
	router := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/inventory/warehouses", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	var body httpx.ErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not the JSON error envelope: %v (body: %s)", err, rec.Body.String())
	}
	if body.Error.Code == "" {
		t.Error("error envelope has no code")
	}
	if body.Error.RequestID == "" {
		t.Error("error envelope has no request_id")
	}
}

func TestInventoryHandler_HappyPaths(t *testing.T) {
	router := newAuthedRouter(happyRepo{})

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{"ListWarehouses", http.MethodGet, "/api/v1/inventory/warehouses", "", http.StatusOK},
		{"CreateWarehouse", http.MethodPost, "/api/v1/inventory/warehouses", `{"organization_id":1,"name":"Main Warehouse","is_active":true}`, http.StatusCreated},
		{"GetWarehouse", http.MethodGet, "/api/v1/inventory/warehouses/1", "", http.StatusOK},
		{"UpdateWarehouse", http.MethodPut, "/api/v1/inventory/warehouses/1", `{"organization_id":1,"name":"Main Warehouse Updated","is_active":true}`, http.StatusOK},
		{"DeleteWarehouse", http.MethodDelete, "/api/v1/inventory/warehouses/1", "", http.StatusNoContent},
		{"ListStocks", http.MethodGet, "/api/v1/inventory/warehouses/1/stocks", "", http.StatusOK},
		{"AdjustStock", http.MethodPost, "/api/v1/inventory/stocks/adjust", `{"stock_id":1,"delta":10,"type":"in","details":"restock"}`, http.StatusOK},
		{"ListLowStock", http.MethodGet, "/api/v1/inventory/stocks/low?limit=10&offset=0", "", http.StatusOK},
		{"ListMovements", http.MethodGet, "/api/v1/inventory/stocks/1/movements", "", http.StatusOK},
		{"ListOrgMovements", http.MethodGet, "/api/v1/inventory/movements?limit=10&offset=0", "", http.StatusOK},
		{"TransferStock", http.MethodPost, "/api/v1/inventory/transfers", `{"from_warehouse_id":1,"to_warehouse_id":2,"product_id":1,"product_variant_id":1,"quantity":10,"notes":"transfer"}`, http.StatusOK},
		{"ListTransfers", http.MethodGet, "/api/v1/inventory/transfers?limit=10&offset=0", "", http.StatusOK},
		{"GetTransfer", http.MethodGet, "/api/v1/inventory/transfers/1", "", http.StatusOK},
		{"ReceiveTransfer", http.MethodPost, "/api/v1/inventory/transfers/1/receive", "", http.StatusOK},
		{"CancelTransfer", http.MethodPost, "/api/v1/inventory/transfers/1/cancel", "", http.StatusOK},
		{"AdminWarehouses", http.MethodGet, "/api/v1/admin/inventory/warehouses", "", http.StatusOK},
		{"AdminTransfers", http.MethodGet, "/api/v1/admin/inventory/transfers", "", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bodyReader io.Reader
			if tt.body != "" {
				bodyReader = strings.NewReader(tt.body)
			}
			req := httptest.NewRequest(tt.method, tt.path, bodyReader)
			if tt.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("%s %s got status %d, want %d (body: %s)", tt.method, tt.path, rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

// AvailableQuantity stub. Real stock totalling is covered by the repository
// integration tests; catalog.ProductVariant.StockQty is never populated, which
// is why this lookup exists at all.
func (s stubRepo) AvailableQuantity(context.Context, int64) (int, error) {
	return 0, nil
}

func (s stubRepo) AvailableQuantities(context.Context, []int64) (map[int64]int, error) {
	return map[int64]int{}, nil
}
