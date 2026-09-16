package catalog

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// The shared catalogue, as the vendor import reads and extends it.
//
// A vendor's row has to be tied to a product every other vendor's row can also
// be tied to, or their prices are not comparable and the marketplace has
// nothing to compare. So the import reads the whole shared catalogue as a
// projection, matches against it in memory, and — where the file carries a
// product the catalogue has never seen — adds it as pending for an
// administrator to approve.

// MatchProduct is a shared-catalogue product reduced to the fields matching
// reads. Thirty thousand of these cost a few megabytes; thirty thousand full
// products would not fit the budget of a request.
type MatchProduct struct {
	ID            int64  `json:"id"`
	NameAR        string `json:"name_ar"`
	NameEN        string `json:"name_en"`
	SKU           string `json:"sku"`
	Barcode       string `json:"barcode"`
	Scientific    string `json:"scientific"`
	DosageForm    string `json:"dosage_form"`
	Concentration string `json:"concentration"`
	Unit          string `json:"unit"`
	Manufacturer  string `json:"manufacturer"`
	PublicPrice   string `json:"public_price"`
}

// ImportBackend is the persistence the vendor import needs from the catalogue.
type ImportBackend interface {
	VariantWriter
	ListMatchProducts(ctx context.Context) ([]MatchProduct, error)
	CreateImportProducts(ctx context.Context, orgID int64, prods []*Product) ([]int64, error)
	RetireVariantsExcept(ctx context.Context, orgID, warehouseID int64, keep []int64) ([]RetiredVariant, error)
	DefaultCatalogOrg(ctx context.Context) (int64, error)
}

func (s *Service) importBackend() (ImportBackend, error) {
	backend, ok := s.repo.(ImportBackend)
	if !ok {
		return nil, ErrBulkVariantsUnavailable
	}
	return backend, nil
}

// ListMatchProducts loads the shared catalogue for in-memory matching.
//
// It runs as the system rather than as the vendor: the shared catalogue belongs
// to the organisation that owns it, and a supplier must be able to match
// against products they do not own — that is the entire point of it being
// shared.
func (s *Service) ListMatchProducts(ctx context.Context) ([]MatchProduct, error) {
	backend, err := s.importBackend()
	if err != nil {
		return nil, err
	}
	return backend.ListMatchProducts(database.AsSystem(ctx))
}

// CreateImportProducts registers products a vendor's file carried that the
// shared catalogue does not have.
//
// The caller decides the status; anything unset is created live. This used to
// force `pending` on every row, on the reasoning that a supplier's spelling is
// not an authority on what the catalogue should call a product. The reasoning
// holds — but the consequence did not: matching, in the smart order and in the
// next vendor's import alike, resolves only to live rows, so every product a
// bulk import introduced was invisible to the very engine meant to find it. The
// review still matters and still happens; it no longer gates whether the product
// exists as far as the rest of the system is concerned.
func (s *Service) CreateImportProducts(ctx context.Context, prods []*Product) ([]int64, error) {
	backend, err := s.importBackend()
	if err != nil {
		return nil, err
	}
	if len(prods) == 0 {
		return nil, nil
	}
	sysCtx := database.AsSystem(ctx)
	orgID, err := backend.DefaultCatalogOrg(sysCtx)
	if err != nil {
		return nil, err
	}
	for _, p := range prods {
		p.OrganizationID = orgID
		if p.Status == "" {
			p.Status = StatusActive
		}
		if p.InstitutionalWorkIDs == nil {
			p.InstitutionalWorkIDs = []int64{}
		}
	}
	return backend.CreateImportProducts(sysCtx, orgID, prods)
}

// RetiredVariant is one variant an import took off sale, and the catalogue
// product it was an offer of. The product id is what lets the caller find the
// warehouse balance that has to go with it.
type RetiredVariant struct {
	ID        int64
	ProductID int64
}

// RetireVariantsExcept takes every variant of an organisation off sale except
// the ones listed, and reports which ones it took.
//
// This is what "this file is my whole catalogue now" means, and it is
// deliberately a deactivation rather than a delete: a variant that disappears
// takes its order history's foreign keys with it, and a supplier who uploads
// the wrong file has to be able to undo the mistake. To the pharmacies
// browsing, and on every one of the vendor's own screens, an inactive variant
// and a deleted one are the same thing — it is off sale, it is out of search,
// and its balance is zero.
func (s *Service) RetireVariantsExcept(ctx context.Context, orgID, warehouseID int64, keep []int64) ([]RetiredVariant, error) {
	backend, err := s.importBackend()
	if err != nil {
		return nil, err
	}
	return backend.RetireVariantsExcept(ctx, orgID, warehouseID, keep)
}

// CatalogOwnerOrg is the organisation that owns the shared catalogue.
func (s *Service) CatalogOwnerOrg(ctx context.Context) (int64, error) {
	backend, err := s.importBackend()
	if err != nil {
		return 0, err
	}
	return backend.DefaultCatalogOrg(database.AsSystem(ctx))
}
