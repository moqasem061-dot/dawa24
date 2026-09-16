package inventory

import (
	"context"
	"strings"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// UpdateWarehouse persists changes to an existing warehouse.
func (s *Service) UpdateWarehouse(ctx context.Context, id int64, input *Warehouse) (*Warehouse, error) {
	if _, ok := database.TenantFrom(ctx); !ok {
		return nil, database.ErrNoTenant
	}

	// Load first so that a warehouse belonging to another tenant is reported as
	// not found by row-level security, before any write is attempted.
	existing, err := s.repo.GetWarehouseByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(input.Name) == "" {
		return nil, apperr.Validation("warehouse.name_required",
			"Warehouse name is required.", map[string]string{"name": "required"})
	}

	// Only mutable fields are copied. Organisation and public id are identity,
	// not attributes, and letting a request body carry them would allow a
	// warehouse to be reassigned to another tenant.
	existing.Name = strings.TrimSpace(input.Name)
	existing.Code = strings.TrimSpace(input.Code)
	existing.Address = input.Address
	existing.Phone = input.Phone
	existing.Latitude = input.Latitude
	existing.Longitude = input.Longitude
	existing.IsActive = input.IsActive
	existing.BranchID = input.BranchID

	if err := s.repo.UpdateWarehouse(ctx, existing); err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "warehouse updated", "warehouse_id", existing.ID)
	return existing, nil
}

// DeleteWarehouse soft-deletes a warehouse that no longer holds stock.
//
// Deleting one that still holds goods is refused rather than cascaded. The
// stock rows would otherwise keep counting toward sellable inventory while
// belonging to a warehouse that no screen lists — inventory that exists in
// arithmetic but nowhere else.
func (s *Service) DeleteWarehouse(ctx context.Context, id int64) error {
	if _, ok := database.TenantFrom(ctx); !ok {
		return database.ErrNoTenant
	}

	if _, err := s.repo.GetWarehouseByID(ctx, id); err != nil {
		return err
	}

	count, err := s.repo.CountStockInWarehouse(ctx, id)
	if err != nil {
		return err
	}
	if count > 0 {
		return apperr.Conflict("warehouse.has_stock",
			"This warehouse still holds stock. Transfer or write off the remaining items before deleting it.")
	}

	if err := s.repo.SoftDeleteWarehouse(ctx, id); err != nil {
		return err
	}

	s.log.InfoContext(ctx, "warehouse deleted", "warehouse_id", id)
	return nil
}

// GetWarehouse returns a single warehouse.
func (s *Service) GetWarehouse(ctx context.Context, id int64) (*Warehouse, error) {
	if _, ok := database.TenantFrom(ctx); !ok {
		return nil, database.ErrNoTenant
	}
	return s.repo.GetWarehouseByID(ctx, id)
}

// ListLowStock returns stock at or below its reorder threshold, worst first.
func (s *Service) ListLowStock(ctx context.Context, limit, offset int) ([]*Stock, error) {
	if _, ok := database.TenantFrom(ctx); !ok {
		return nil, database.ErrNoTenant
	}
	return s.repo.ListLowStock(ctx, limit, offset)
}

// ListLowStockWithTotal returns paginated stock at or below its reorder threshold with total count.
func (s *Service) ListLowStockWithTotal(ctx context.Context, limit, offset int) ([]*Stock, int, error) {
	if _, ok := database.TenantFrom(ctx); !ok {
		return nil, 0, database.ErrNoTenant
	}
	return s.repo.ListLowStockWithTotal(ctx, limit, offset)
}

// ListOrgMovements returns the organisation-wide stock movement ledger.
func (s *Service) ListOrgMovements(ctx context.Context, limit, offset int) ([]*StockMovement, error) {
	if _, ok := database.TenantFrom(ctx); !ok {
		return nil, database.ErrNoTenant
	}
	return s.repo.ListMovementsByOrg(ctx, limit, offset)
}
