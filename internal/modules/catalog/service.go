package catalog

import (
	"context"
	"errors"
	"fmt"
	"github.com/muhiya/dawa24-store/internal/shared/matchflow"
	"log/slog"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// Cache represents the key-value cache contract used for high-frequency catalog reads.
type Cache interface {
	GetJSON(ctx context.Context, key string, dst any) error
	SetJSON(ctx context.Context, key string, val any, ttl time.Duration) error
	Delete(ctx context.Context, keys ...string) error
}

// Service coordinates product catalog business logic.
type Service struct {
	repo     Repository
	log      *slog.Logger
	instGate InstitutionalGate
	cache    Cache
	// imports and enricher back the reviewed catalogue import. Both are
	// optional: without a store the wizard is unavailable, and without an
	// enricher the AI switch is simply not offered.
	imports ImportSessionStore
	mapper  AIMapper
	// adjudicator resolves the rows similarity could not settle. Optional in
	// exactly the same way: unset means the tier is skipped and the import
	// keeps its deterministic answer.
	adjudicator MatchAdjudicator
	// matchMemory is the shared decision cache. Nil means every question is
	// paid for again, which is slower and dearer but never wrong.
	matchMemory matchflow.Memory
	// progress tracks background preparation runs so the review screen can show
	// the admin what a long import is doing.
	progress *ProgressTracker
	// notifyProgress announces every tick to whoever is watching live, without
	// putting a database write behind each one. See persistedProgress.
	notifyProgress ProgressNotifier
	// sheets holds decoded workbooks for the length of a mapping session, so
	// correcting a column mapping does not re-read and re-decode the upload.
	sheets *sheetCache
}

// SetInstitutionalGate installs the institutional work filter gate.
func (s *Service) SetInstitutionalGate(gate InstitutionalGate) {
	s.instGate = gate
}

// SetCache installs the cache layer for high-speed taxonomy reads.
func (s *Service) SetCache(c Cache) {
	s.cache = c
}

// NewService creates a new catalog service.
func NewService(repo Repository, log *slog.Logger) *Service {
	return &Service{
		repo: repo,
		log:  log,
	}
}

// CreateProduct creates a new product under the tenant bound to the request context.
func (s *Service) CreateProduct(ctx context.Context, p *Product) (*Product, error) {
	if p.OrganizationID == 0 {
		orgID, ok := database.TenantFrom(ctx)
		if ok && orgID > 0 {
			p.OrganizationID = orgID
		}
	}

	if err := p.Validate(); err != nil {
		return nil, err
	}
	if err := s.assertBrandInCategory(ctx, p); err != nil {
		return nil, err
	}

	if p.Status == "" {
		p.Status = StatusActive
	}
	if p.InstitutionalWorkIDs == nil {
		p.InstitutionalWorkIDs = []int64{}
	}

	if err := s.repo.CreateProduct(ctx, p); err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "product created", "product_id", p.ID, "org_id", p.OrganizationID)
	return p, nil
}

// maxImportBatch bounds one upload. Ten thousand rows is roughly the largest
// real distributor catalogue and already takes several seconds inside a single
// transaction; beyond that the request would outlive its own timeout and hold
// write locks on catalog.products the whole time.
const maxImportBatch = 20000

// BulkImportProducts writes a parsed batch into the master catalogue.
//
// Rows are validated here, before the write, so a bad value is reported against
// its product rather than aborting a transaction halfway through. Products that
// fail validation are dropped from the batch and named in the returned issues;
// the write itself stays all-or-nothing.
func (s *Service) BulkImportProducts(
	ctx context.Context, prods []*Product, opts BulkWriteOptions,
) (BulkWriteResult, []RowIssue, error) {
	empty := BulkWriteResult{Matches: map[int]MatchReason{}}
	if len(prods) == 0 {
		return empty, nil, nil
	}
	if len(prods) > maxImportBatch {
		return empty, nil, apperr.Validation("catalog.import_too_large",
			fmt.Sprintf(i18n.TDefault("w4_mod.d_d_102"),
				len(prods), maxImportBatch), nil)
	}

	valid, issues, err := s.validateImportBatch(prods)
	if err != nil {
		return empty, issues, err
	}

	res, err := s.repo.BulkUpsertProducts(ctx, valid, opts)
	if err != nil {
		s.log.ErrorContext(ctx, "bulk import products failed",
			"count", len(valid), "failures", len(res.Failures), "error", err)
		return res, issues, err
	}

	s.log.InfoContext(ctx, "bulk import products completed",
		"inserted", res.Inserted, "updated", res.Updated,
		"brands_created", res.BrandsCreated, "categories_created", res.CategoriesCreated,
		"submitted", len(valid))
	return res, issues, nil
}

// validateImportBatch drops products the domain refuses and names them in the
// returned issues, so a bad value is reported against its row rather than
// aborting a transaction halfway through.
func (s *Service) validateImportBatch(prods []*Product) ([]*Product, []RowIssue, error) {
	valid := make([]*Product, 0, len(prods))
	var issues []RowIssue

	for i, p := range prods {
		if p == nil {
			continue
		}
		if err := p.Validate(); err != nil {
			issues = append(issues, RowIssue{
				Row:      i + 1,
				Value:    p.Name.Get(i18n.AR),
				Message:  s.validationMessage(err),
				Severity: SeverityError,
			})
			continue
		}
		valid = append(valid, p)
	}

	if len(valid) == 0 {
		return nil, issues, apperr.Validation("catalog.import_no_valid_rows",
			i18n.TDefault("w4_mod.w4str_115_115"), nil)
	}
	return valid, issues, nil
}

// validationMessage renders a domain validation failure for the import report,
// preferring the Arabic message the rule carries.
func (s *Service) validationMessage(err error) string {
	var appErr *apperr.Error
	if errors.As(err, &appErr) && appErr.Msg != "" {
		return appErr.Msg
	}
	return err.Error()
}

// GetProduct retrieves a product and its associated active variants.
func (s *Service) GetProduct(ctx context.Context, id int64) (*Product, []*ProductVariant, error) {
	p, err := s.repo.GetProductByID(ctx, id)
	if err != nil {
		return nil, nil, err
	}

	variants, err := s.repo.ListVariantsByProduct(ctx, id)
	if err != nil {
		return nil, nil, err
	}

	return p, variants, nil
}

// UpdateProduct updates product details.
func (s *Service) UpdateProduct(ctx context.Context, p *Product) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if err := s.assertBrandInCategory(ctx, p); err != nil {
		return err
	}
	if p.InstitutionalWorkIDs == nil {
		p.InstitutionalWorkIDs = []int64{}
	}
	return s.repo.UpdateProduct(ctx, p)
}

// assertBrandInCategory refuses a product whose manufacturer does not operate
// in its category. The product form filters the brand list client-side for
// convenience; this is the rule, so a crafted or stale form cannot save a
// cosmetics product under a medical-supplies-only manufacturer.
//
// A product with no category or no brand is left alone: both columns are
// nullable and plenty of legacy rows carry only one.
func (s *Service) assertBrandInCategory(ctx context.Context, p *Product) error {
	if p.CategoryID == nil || p.BrandID == nil || *p.CategoryID <= 0 || *p.BrandID <= 0 {
		return nil
	}
	ok, err := s.repo.BrandInCategory(ctx, *p.CategoryID, *p.BrandID)
	if err != nil {
		return err
	}
	if !ok {
		return apperr.Validation("catalog.brand_category_mismatch",
			"The selected manufacturer does not operate in the selected category.",
			map[string]string{"brand_id": i18n.TDefault("w4_mod.s_341_341")})
	}
	return nil
}

// DeleteProduct soft-deletes a product.
func (s *Service) DeleteProduct(ctx context.Context, id int64) error {
	return s.repo.DeleteProduct(ctx, id)
}

// Search runs full Arabic/English search across the product catalogue.
func (s *Service) Search(ctx context.Context, params SearchParams) ([]*Product, error) {
	if params.FirstWord == "" && params.Query != "" {
		params.FirstWord = FirstWordOf(params.Query)
	}
	if s.instGate != nil && len(params.AllowedWorkIDs) == 0 {
		if uid, err := authctx.UserID(ctx); err == nil && uid > 0 {
			works, err := s.instGate.AllowedWorkIDs(ctx, uid, params.FilterMode)
			if err == nil {
				params.AllowedWorkIDs = works
			}
		}
	}
	return s.repo.SearchProducts(ctx, params)
}

// Count returns the total count of products matching search filters.
func (s *Service) Count(ctx context.Context, params SearchParams) (int, error) {
	if params.FirstWord == "" && params.Query != "" {
		params.FirstWord = FirstWordOf(params.Query)
	}
	if s.instGate != nil && len(params.AllowedWorkIDs) == 0 {
		if uid, err := authctx.UserID(ctx); err == nil && uid > 0 {
			works, err := s.instGate.AllowedWorkIDs(ctx, uid, params.FilterMode)
			if err == nil {
				params.AllowedWorkIDs = works
			}
		}
	}
	return s.repo.CountProducts(ctx, params)
}

// SearchWithTotal searches products and returns total matching count for pagination.
func (s *Service) SearchWithTotal(ctx context.Context, params SearchParams) ([]*Product, int, error) {
	if params.FirstWord == "" && params.Query != "" {
		params.FirstWord = FirstWordOf(params.Query)
	}
	if s.instGate != nil && len(params.AllowedWorkIDs) == 0 {
		if uid, err := authctx.UserID(ctx); err == nil && uid > 0 {
			works, err := s.instGate.AllowedWorkIDs(ctx, uid, params.FilterMode)
			if err == nil {
				params.AllowedWorkIDs = works
			}
		}
	}
	prods, err := s.repo.SearchProducts(ctx, params)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.repo.CountProducts(ctx, params)
	if err != nil {
		return prods, len(prods), nil
	}
	return prods, total, nil
}

// FastSearch searches the denormalized read model (catalog.product_index) with deterministic fallback (Rule R3).
// If the indexed table returns 0 results or errors, it falls back to direct catalog.products table search.
func (s *Service) FastSearch(ctx context.Context, params SearchParams) ([]*ProductIndexItem, error) {
	if s.instGate != nil && len(params.AllowedWorkIDs) == 0 {
		if uid, err := authctx.UserID(ctx); err == nil && uid > 0 {
			works, err := s.instGate.AllowedWorkIDs(ctx, uid, params.FilterMode)
			if err == nil {
				params.AllowedWorkIDs = works
			}
		}
	}

	items, err := s.repo.SearchProductIndex(ctx, params)
	if err == nil && len(items) > 0 {
		return items, nil
	}

	// Deterministic fallback (Rule R3): Fallback to catalog.products
	products, fbErr := s.repo.SearchProducts(ctx, params)
	if fbErr != nil {
		if err != nil {
			return nil, err
		}
		return nil, fbErr
	}

	fallbackItems := make([]*ProductIndexItem, 0, len(products))
	for _, p := range products {
		nameAr := p.Name.Get("ar")
		nameEn := p.Name.Get("en")
		hasDisc := p.Discount.IsPositive()
		finalPrice := p.EffectivePrice()
		var discPct float64
		if p.Price.IsPositive() && hasDisc {
			discPct = float64(p.Discount.Minor()) / float64(p.Price.Minor()) * 100.0
		}

		fallbackItems = append(fallbackItems, &ProductIndexItem{
			UniqueRowID:          ComposeUniqueRowID(p.ID, nil, p.BranchID),
			ProductID:            p.ID,
			SKU:                  p.SKU,
			NameAR:               nameAr,
			NameEN:               nameEn,
			ScientificName:       p.ScientificName,
			Price:                p.Price,
			Discount:             p.Discount,
			StockQuantity:        0,
			CategoryID:           p.CategoryID,
			BrandID:              p.BrandID,
			HasDiscount:          hasDisc,
			DiscountPercentage:   discPct,
			PriceAfterDiscount:   finalPrice,
			OrganizationID:       p.OrganizationID,
			BranchID:             p.BranchID,
			Status:               string(p.Status),
			ProductType:          "parent",
			InstitutionalWorkIDs: p.InstitutionalWorkIDs,
			CreatedAt:            p.CreatedAt,
			UpdatedAt:            p.UpdatedAt,
		})
	}
	return fallbackItems, nil
}

// RebuildProductIndex runs a full sweep rebuild of the denormalized read model.
func (s *Service) RebuildProductIndex(ctx context.Context) (int64, error) {
	return s.repo.RebuildProductIndex(ctx)
}

// ListDosageForms returns distinct pharmaceutical dosage forms used in the catalog.
func (s *Service) ListDosageForms(ctx context.Context) ([]string, error) {
	return s.repo.ListDosageForms(ctx)
}

