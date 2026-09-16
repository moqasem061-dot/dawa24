package inventory

import (
	"context"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

var (
	errWarehouseNameRequired = apperr.Validation("warehouse.name_required",
		"Warehouse name is required.", map[string]string{"name": "required"})
	errWarehouseOwnerRequired = apperr.Validation("warehouse.owner_required",
		"An owning organization is required.", map[string]string{"organization_id": "required"})
)

// The platform's view of every organisation's warehouses.
//
// The screen listed them unpaginated and unfiltered, resolved the owning
// organisation through a five-hundred-row lookup map, and offered no way to act
// on one: an administrator could see a warehouse and do nothing about it.

// AdminWarehouseFilter narrows the cross-tenant warehouse listing.
type AdminWarehouseFilter struct {
	Query          string
	OrganizationID int64
	BranchID       int64
	// Active is nil for "either", so "show me the disabled ones" is expressible
	// and is not the same question as "show me everything".
	Active *bool

	Limit  int
	Offset int
}

// Normalize clamps the page window.
func (f *AdminWarehouseFilter) Normalize() {
	f.Query = strings.TrimSpace(f.Query)
	if f.Limit <= 0 {
		f.Limit = 25
	}
	if f.Limit > 200 {
		f.Limit = 200
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
}

// AdminWarehouseRow is one line of the cross-tenant warehouse listing.
type AdminWarehouseRow struct {
	Warehouse

	OrganizationName string `json:"organization_name"`
	OrganizationType string `json:"organization_type"`
	BranchName       string `json:"branch_name"`

	// ItemCount and TotalQuantity are what makes a warehouse legible: a name
	// alone does not say whether disabling it strands anything.
	ItemCount     int        `json:"item_count"`
	TotalQuantity int        `json:"total_quantity"`
	LastMovement  *time.Time `json:"last_movement,omitempty"`
}

// HoldsStock reports whether disabling this warehouse would strand goods.
func (r *AdminWarehouseRow) HoldsStock() bool {
	return r != nil && r.TotalQuantity > 0
}

// AdminWarehouseBackend is the cross-tenant listing's persistence.
type AdminWarehouseBackend interface {
	ListAdminWarehouseRows(ctx context.Context, f AdminWarehouseFilter) ([]*AdminWarehouseRow, int, error)
	AdminWarehouseOwners(ctx context.Context) ([]AdminWarehouseOwner, error)
	SetWarehouseActive(ctx context.Context, id int64, active bool) error
}

// AdminWarehouseOwner is one entry of the organisation filter.
type AdminWarehouseOwner struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Type  string `json:"type"`
	Count int    `json:"count"`
}

// ListAdminWarehouseRows returns one filtered page of every organisation's
// warehouses.
func (s *Service) ListAdminWarehouseRows(
	ctx context.Context, f AdminWarehouseFilter,
) ([]*AdminWarehouseRow, int, error) {
	backend, ok := s.repo.(AdminWarehouseBackend)
	if !ok {
		return nil, 0, nil
	}
	return backend.ListAdminWarehouseRows(ctx, f)
}

// AdminWarehouseOwners lists the organisations that own a warehouse.
func (s *Service) AdminWarehouseOwners(ctx context.Context) ([]AdminWarehouseOwner, error) {
	backend, ok := s.repo.(AdminWarehouseBackend)
	if !ok {
		return nil, nil
	}
	return backend.AdminWarehouseOwners(ctx)
}

// AdminSetWarehouseActive enables or disables one warehouse on behalf of the
// platform.
//
// It is deliberately separate from UpdateWarehouse, which requires a tenant in
// context and refuses to touch another organisation's row. An administrator has
// no tenant of their own here; the caller is responsible for having established
// that they are staff, and the write is recorded in the audit log by the
// repository.
func (s *Service) AdminSetWarehouseActive(ctx context.Context, id int64, active bool) error {
	backend, ok := s.repo.(AdminWarehouseBackend)
	if !ok {
		return nil
	}
	if err := backend.SetWarehouseActive(ctx, id, active); err != nil {
		return err
	}
	s.log.InfoContext(ctx, "warehouse activation changed by platform staff",
		"warehouse_id", id, "active", active)
	return nil
}

// AdminUpdateWarehouse saves an administrator's edit to any organisation's
// warehouse.
//
// Separate from UpdateWarehouse because that one requires a tenant in context
// and reads the row through row-level security, which is exactly right for the
// owner and refuses for staff. The owning organisation is never taken from the
// caller: it stays whatever the stored row says, so an edit cannot move a
// warehouse — and the stock inside it — into another tenant.
func (s *Service) AdminUpdateWarehouse(ctx context.Context, w *Warehouse) error {
	if w == nil || w.ID <= 0 {
		return nil
	}
	if strings.TrimSpace(w.Name) == "" {
		return errWarehouseNameRequired
	}
	w.Name = strings.TrimSpace(w.Name)
	if err := s.repo.UpdateWarehouse(ctx, w); err != nil {
		return err
	}
	s.log.InfoContext(ctx, "warehouse updated by platform staff", "warehouse_id", w.ID)
	return nil
}

// AdminCreateWarehouse creates a warehouse on behalf of an organisation.
func (s *Service) AdminCreateWarehouse(ctx context.Context, w *Warehouse) error {
	if w == nil || w.OrganizationID <= 0 {
		return errWarehouseOwnerRequired
	}
	if strings.TrimSpace(w.Name) == "" {
		return errWarehouseNameRequired
	}
	w.Name = strings.TrimSpace(w.Name)
	if err := s.repo.CreateWarehouse(ctx, w); err != nil {
		return err
	}
	s.log.InfoContext(ctx, "warehouse created by platform staff",
		"warehouse_id", w.ID, "org_id", w.OrganizationID)
	return nil
}
