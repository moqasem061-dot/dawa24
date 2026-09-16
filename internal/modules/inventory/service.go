package inventory

import (
	"context"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"log/slog"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Service coordinates inventory management and stock ledger operations.
type Service struct {
	repo Repository
	log  *slog.Logger
}

// NewService creates a new inventory service.
func NewService(repo Repository, log *slog.Logger) *Service {
	return &Service{
		repo: repo,
		log:  log,
	}
}

// CreateWarehouse creates a storage warehouse for the current tenant.
func (s *Service) CreateWarehouse(ctx context.Context, w *Warehouse) (*Warehouse, error) {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return nil, database.ErrNoTenant
	}
	w.OrganizationID = orgID

	if w.Name == "" {
		return nil, apperr.Validation("warehouse.name_required", "Warehouse name is required.", nil)
	}
	w.IsActive = true

	if err := s.repo.CreateWarehouse(ctx, w); err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "warehouse created", "warehouse_id", w.ID, "org_id", w.OrganizationID)
	return w, nil
}

// ListWarehouses returns all active warehouses for the current tenant.
func (s *Service) ListWarehouses(ctx context.Context) ([]*Warehouse, error) {
	return s.repo.ListWarehouses(ctx)
}

// ListWarehousesWithTotal returns paginated active warehouses for the current tenant with total count.
func (s *Service) ListWarehousesWithTotal(ctx context.Context, limit, offset int) ([]*Warehouse, int, error) {
	return s.repo.ListWarehousesWithTotal(ctx, limit, offset)
}

// ClearWarehouseStocks deletes all stocks for a warehouse (for clear_and_add import mode).
func (s *Service) ClearWarehouseStocks(ctx context.Context, warehouseID int64) error {
	return s.repo.ClearWarehouseStocks(ctx, warehouseID)
}

// UpsertStock inserts or updates a stock record in a warehouse.
func (s *Service) UpsertStock(ctx context.Context, st *Stock) error {
	return s.repo.UpsertStock(ctx, st)
}

// AdjustStockInput carries parameters for a stock adjustment.
type AdjustStockInput struct {
	StockID       int64        `json:"stock_id"`
	Delta         int          `json:"delta"`
	Type          MovementType `json:"type"`
	Details       string       `json:"details,omitempty"`
	ReferenceType string       `json:"reference_type,omitempty"`
	ReferenceID   *int64       `json:"reference_id,omitempty"`
	UserID        *int64       `json:"user_id,omitempty"`
}

// AdjustStock updates stock atomically and adds an entry to the movement ledger.
func (s *Service) AdjustStock(ctx context.Context, input AdjustStockInput) (*Stock, error) {
	if input.StockID <= 0 {
		return nil, apperr.Validation("stock_id.required", "Stock ID is required.", nil)
	}
	if input.Delta == 0 {
		return nil, apperr.Validation("delta.invalid", "Delta quantity cannot be zero.", nil)
	}
	if input.Type == "" {
		input.Type = MovementAdjustment
	}

	movement := StockMovement{
		Type:          input.Type,
		QuantityDelta: input.Delta,
		Details:       input.Details,
		ReferenceType: input.ReferenceType,
		ReferenceID:   input.ReferenceID,
		UserID:        input.UserID,
	}

	stock, err := s.repo.AdjustStock(ctx, input.StockID, input.Delta, movement)
	if err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "stock adjusted", "stock_id", stock.ID, "delta", input.Delta, "balance", stock.Quantity)
	return stock, nil
}

// GetStockByID loads one stock row the caller's tenant owns.
func (s *Service) GetStockByID(ctx context.Context, id int64) (*Stock, error) {
	return s.repo.GetStockByID(ctx, id)
}

// TransferStock transfers stock between two warehouses belonging to the tenant.
func (s *Service) TransferStock(ctx context.Context, t *WarehouseTransfer) (*WarehouseTransfer, error) {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return nil, database.ErrNoTenant
	}
	t.OrganizationID = orgID

	if err := t.Validate(); err != nil {
		return nil, err
	}

	// 1. Check source stock
	fromStock, err := s.repo.GetStock(ctx, t.FromWarehouseID, t.ProductVariantID)
	if err != nil {
		return nil, err
	}
	if fromStock.Quantity < t.Quantity {
		return nil, apperr.Validation("transfer.insufficient_source", "Source warehouse has insufficient stock.", nil)
	}

	// 2. Deduct the source only. The destination is credited on receipt, not
	//    on dispatch — see transfer_state.go. Goods on a van belong to neither
	//    warehouse's sellable quantity, and crediting early would let the
	//    destination sell medicine it has not received.
	_, err = s.repo.AdjustStock(ctx, fromStock.ID, -t.Quantity, StockMovement{
		Type:          MovementTransfer,
		QuantityDelta: -t.Quantity,
		Details:       "Outbound transfer dispatched",
		UserID:        t.InitiatedBy,
	})
	if err != nil {
		return nil, err
	}

	t.Status = TransferInTransit
	if err := s.repo.CreateTransfer(ctx, t); err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "warehouse transfer dispatched",
		"transfer_id", t.ID, "quantity", t.Quantity,
		"from", t.FromWarehouseID, "to", t.ToWarehouseID)
	return t, nil
}

// ListStocksByWarehouse retrieves stock rows for a warehouse.
func (s *Service) ListStocksByWarehouse(ctx context.Context, warehouseID int64) ([]*Stock, error) {
	return s.repo.ListStocksByWarehouse(ctx, warehouseID)
}

// ListDetailedStocksByWarehouse retrieves detailed stock and variant rows for a warehouse.
func (s *Service) ListDetailedStocksByWarehouse(ctx context.Context, warehouseID int64) ([]*DetailedWarehouseStockView, error) {
	return s.repo.ListDetailedStocksByWarehouse(ctx, warehouseID)
}

// ListStocksByOrg retrieves all stock rows for an organization across all its warehouses.
func (s *Service) ListStocksByOrg(ctx context.Context, orgID int64) ([]*Stock, error) {
	if orgID <= 0 {
		return nil, nil
	}
	return s.repo.ListStocksByOrg(ctx, orgID)
}

// ListStocksByOrgWithTotal retrieves paginated stock rows for an organization matching filters with total count.
func (s *Service) ListStocksByOrgWithTotal(ctx context.Context, orgID, warehouseID int64, search string, limit, offset int) ([]*Stock, int, error) {
	if orgID <= 0 {
		return nil, 0, nil
	}
	return s.repo.ListStocksByOrgWithTotal(ctx, orgID, warehouseID, search, limit, offset)
}

// ListLowStocksByOrgWithTotal retrieves paginated stock rows at or below their reorder threshold for an organization matching filters with total count.
func (s *Service) ListLowStocksByOrgWithTotal(ctx context.Context, orgID, warehouseID int64, search string, limit, offset int) ([]*Stock, int, error) {
	if orgID <= 0 {
		return nil, 0, nil
	}
	return s.repo.ListLowStocksByOrgWithTotal(ctx, orgID, warehouseID, search, limit, offset)
}

// ListStockMovements retrieves the movement ledger for a stock row.
func (s *Service) ListStockMovements(ctx context.Context, stockID int64, limit int) ([]*StockMovement, error) {
	return s.repo.ListStockMovements(ctx, stockID, limit)
}

// AvailableQuantity totals a variant's sellable stock across warehouses.
func (s *Service) AvailableQuantity(ctx context.Context, variantID int64) (int, error) {
	if variantID <= 0 {
		return 0, nil
	}
	return s.repo.AvailableQuantity(ctx, variantID)
}

// AvailableQuantities totals many variants' sellable stock in one query,
// keyed by variant ID; missing keys mean zero availability.
func (s *Service) AvailableQuantities(ctx context.Context, variantIDs []int64) (map[int64]int, error) {
	filtered := make([]int64, 0, len(variantIDs))
	for _, id := range variantIDs {
		if id > 0 {
			filtered = append(filtered, id)
		}
	}
	return s.repo.AvailableQuantities(ctx, filtered)
}

// SetStock creates or updates the stock row for one variant in one warehouse.
// It is how an opening quantity gets recorded when a supplier publishes a
// variant — inventory.stocks is the only place stock lives, and its
// warehouse_id is NOT NULL, so the caller must have a warehouse.
func (s *Service) SetStock(ctx context.Context, st *Stock) error {
	if st == nil || st.WarehouseID <= 0 || st.ProductVariantID <= 0 {
		return apperr.Validation("inventory.stock_invalid",
			"Warehouse and product variant are required.",
			map[string]string{"warehouse_id": i18n.TDefault("w4_mod.s_397_397")})
	}
	if st.Quantity < 0 {
		return apperr.Validation("inventory.quantity_negative",
			"Quantity cannot be negative.",
			map[string]string{"quantity": i18n.TDefault("w4_mod.s_398_398")})
	}
	return s.repo.UpsertStock(ctx, st)
}
