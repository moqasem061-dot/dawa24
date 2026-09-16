package ingest

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// CancelImport discards an import without touching the catalogue.
func (s *Service) CancelImport(ctx context.Context, publicID string) error {
	session, err := s.LoadImport(ctx, publicID)
	if err != nil {
		return err
	}
	if session.Phase == PhaseProcessing {
		return apperr.Conflict("import.running",
			i18n.TDefault("w4_mod.w4str_201_201"))
	}
	return s.imports.Cancel(ctx, session.ID)
}

// RecentImports backs the history panel on the upload screen.
func (s *Service) RecentImports(ctx context.Context, orgID int64, limit int) ([]*Session, error) {
	if s.imports == nil {
		return nil, ErrImportStoreUnavailable
	}
	return s.imports.List(ctx, orgID, limit)
}

// ImportRows reads a page of the results table.
func (s *Service) ImportRows(
	ctx context.Context, publicID string, filter RowFilter,
) ([]*RowOutcome, int, error) {
	session, err := s.LoadImport(ctx, publicID)
	if err != nil {
		return nil, 0, err
	}
	return s.imports.Rows(ctx, session.ID, filter)
}

// ImportRowCounts tallies the results ledger by outcome.
func (s *Service) ImportRowCounts(ctx context.Context, publicID string) (map[string]int, error) {
	session, err := s.LoadImport(ctx, publicID)
	if err != nil {
		return nil, err
	}
	return s.imports.RowCounts(ctx, session.ID)
}

// Warehouses lists the vendor's warehouses for the settings screen.
func (s *Service) Warehouses(ctx context.Context) ([]*inventory.Warehouse, error) {
	if s.inventory == nil {
		return nil, apperr.Unavailable("inventory", nil)
	}
	return s.inventory.ListWarehouses(ctx)
}
