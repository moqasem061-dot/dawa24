package ingest

import (
	"context"
	"fmt"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

// UpdateStagedRow modifies a staged row's editable attributes before commit.
func (s *Service) UpdateStagedRow(
	ctx context.Context, publicID string, rowID int64,
	displayName, customVariantName string, price, discount, costPrice, costDiscount *float64, quantity *int, isExcluded *bool,
) error {
	session, err := s.LoadImport(ctx, publicID)
	if err != nil {
		return err
	}
	if session.Phase != PhaseReview {
		return apperr.Conflict("import.not_in_review", i18n.TDefault("w4_mod.w4str_205_205"))
	}
	return s.imports.UpdateRow(ctx, session.ID, rowID, displayName, customVariantName, price, discount, costPrice, costDiscount, quantity, isExcluded)
}

// SetBatchQuantity applies a uniform quantity to all staged rows in the import session.
func (s *Service) SetBatchQuantity(ctx context.Context, publicID string, quantity int) error {
	session, err := s.LoadImport(ctx, publicID)
	if err != nil {
		return err
	}
	if session.Phase != PhaseReview {
		return apperr.Conflict("import.not_in_review", i18n.TDefault("w4_mod.w4str_206_206"))
	}
	return s.imports.SetBatchQuantity(ctx, session.ID, quantity)
}

// AssignStagedRowMatch manually binds a staged row to a master catalog product.
func (s *Service) AssignStagedRowMatch(
	ctx context.Context, publicID string, rowID, productID int64,
) error {
	session, err := s.LoadImport(ctx, publicID)
	if err != nil {
		return err
	}
	if session.Phase != PhaseReview {
		return apperr.Conflict("import.not_in_review", i18n.TDefault("w4_mod.w4str_193_193"))
	}
	if productID <= 0 {
		return s.imports.AssignRowMatch(ctx, session.ID, rowID, 0, "", "")
	}
	p, _, err := s.catalog.GetProduct(database.AsSystem(ctx), productID)
	if err != nil || p == nil {
		return apperr.NotFound("product")
	}
	name := p.Name.Get("ar")
	if name == "" {
		name = p.Name.Get("en")
	}
	return s.imports.AssignRowMatch(ctx, session.ID, rowID, productID, name, p.SKU)
}

// ToggleStagedRowExclude toggles row inclusion/exclusion.
func (s *Service) ToggleStagedRowExclude(ctx context.Context, publicID string, rowID int64) (bool, error) {
	session, err := s.LoadImport(ctx, publicID)
	if err != nil {
		return false, err
	}
	if session.Phase != PhaseReview {
		return false, apperr.Conflict("import.not_in_review", i18n.TDefault("w4_mod.w4str_207_207"))
	}
	return s.imports.ToggleRowExclude(ctx, session.ID, rowID)
}

// SearchMasterCatalog queries master products for the manual match search modal.
func (s *Service) SearchMasterCatalog(ctx context.Context, query string) ([]*catalog.Product, error) {
	if s.catalog == nil {
		return nil, ErrImportStoreUnavailable
	}
	return s.catalog.Search(database.AsSystem(ctx), catalog.SearchParams{
		Query: query,
		Limit: 25,
	})
}

// vocabulary loads the catalogue's own brand and category names, which let the
// analyser recognise a column by what its values are.
//
// A failure here is not fatal: without the vocabulary the header and the value
// shapes still decide every column, and refusing to analyse a file because the
// brand list would not load would be a poor trade.
func (s *Service) vocabulary(ctx context.Context, orgID int64) *productmatch.Vocabulary {
	if s.catalog == nil {
		return productmatch.NewVocabulary(nil, nil, nil, nil)
	}
	vocab, err := s.catalog.ImportVocabulary(ctx, orgID)
	if err != nil {
		s.log.WarnContext(ctx, "import vocabulary unavailable", "error", err)
		return productmatch.NewVocabulary(nil, nil, nil, nil)
	}
	brands := make([]string, 0, len(vocab.Brands))
	for _, b := range vocab.Brands {
		brands = append(brands, b.Name)
	}
	categories := make([]string, 0, len(vocab.Categories))
	for _, c := range vocab.Categories {
		categories = append(categories, c.Name)
	}

	var warehouses, branches []string
	if s.inventory != nil {
		if list, err := s.inventory.ListWarehouses(ctx); err == nil {
			for _, w := range list {
				warehouses = append(warehouses, w.Name)
				if w.Code != "" {
					warehouses = append(warehouses, w.Code)
				}
			}
		}
	}
	return productmatch.NewVocabulary(brands, categories, warehouses, branches)
}

// describeStore is used by the handlers to explain an unavailable feature.
func (s *Service) describeStore() error {
	if s.imports == nil {
		return fmt.Errorf("%w", ErrImportStoreUnavailable)
	}
	return nil
}
