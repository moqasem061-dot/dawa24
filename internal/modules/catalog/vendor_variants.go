package catalog

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// The vendor's own catalogue listing.
//
// Paged at the database rather than in the browser. The screen this replaces
// asked for five hundred rows and filtered them client-side, which meant a
// vendor with nine thousand variants could neither see nor search the other
// eight and a half thousand — and was shown a total that said five hundred.

// StockFilter narrows a listing by what is actually on the shelf.
type StockFilter string

const (
	// StockFilterAny applies no stock filter.
	StockFilterAny StockFilter = ""
	// StockFilterIn is everything with a balance above zero.
	StockFilterIn StockFilter = "in"
	// StockFilterLow is at or below the warehouse's re-order threshold.
	StockFilterLow StockFilter = "low"
	// StockFilterOut has run out.
	StockFilterOut StockFilter = "out"
)

// VendorVariantQuery is one page request against a vendor's catalogue.
type VendorVariantQuery struct {
	Query    string
	Status   string
	Stock    StockFilter
	Expiring bool
	Sort     string
	// PageNumber is one-based, as the pager shows it.
	PageNumber int
	PerPage    int
}

// PageSizes are the rows-per-page choices the listing offers. It mirrors
// pagination.RowsPerPageOptions so the vendor products table offers the same
// choices as every other dashboard table; it is duplicated rather than imported
// only because internal/shared/pagination must stay a dependency-free leaf and
// this package is free to depend on it either way — keep the two in step.
var PageSizes = []int{10, 25, 50, 100}

// DefaultPageSize is what the listing opens on: the platform-wide table default
// (pagination.TableRows), not the API's collection default.
const DefaultPageSize = 25

// Page resolves the query into a LIMIT and an OFFSET, clamped to the offered
// sizes so a hand-edited URL cannot ask for a hundred thousand rows.
func (q VendorVariantQuery) Page() (limit, offset int) {
	limit = q.PerPage
	if !validPageSize(limit) {
		limit = DefaultPageSize
	}
	page := q.PageNumber
	if page < 1 {
		page = 1
	}
	return limit, (page - 1) * limit
}

func validPageSize(size int) bool {
	for _, s := range PageSizes {
		if s == size {
			return true
		}
	}
	return false
}

// VendorVariantStats are the headline figures, counted across the whole
// catalogue rather than across the page on screen.
type VendorVariantStats struct {
	Total      int `json:"total"`
	Active     int `json:"active"`
	InStock    int `json:"in_stock"`
	LowStock   int `json:"low_stock"`
	OutOfStock int `json:"out_of_stock"`
	Expiring   int `json:"expiring"`
}

// VendorCatalogBackend is the persistence the vendor listing needs.
type VendorCatalogBackend interface {
	ListVendorVariants(ctx context.Context, orgID int64, params VendorVariantQuery) ([]*ProductVariant, int, error)
	VendorVariantStats(ctx context.Context, orgID int64) (VendorVariantStats, error)
	ListProductsByIDs(ctx context.Context, ids []int64) (map[int64]*Product, error)
}

func (s *Service) vendorBackend() (VendorCatalogBackend, error) {
	backend, ok := s.repo.(VendorCatalogBackend)
	if !ok {
		return nil, ErrBulkVariantsUnavailable
	}
	return backend, nil
}

// ListVendorVariants returns one page of a vendor's catalogue with its stock
// balances, and the total the pager needs.
func (s *Service) ListVendorVariants(
	ctx context.Context, orgID int64, params VendorVariantQuery,
) ([]*ProductVariant, int, error) {
	backend, err := s.vendorBackend()
	if err != nil {
		return nil, 0, err
	}
	return backend.ListVendorVariants(ctx, orgID, params)
}

// VendorVariantStats counts a vendor's whole catalogue.
func (s *Service) VendorVariantStats(ctx context.Context, orgID int64) (VendorVariantStats, error) {
	backend, err := s.vendorBackend()
	if err != nil {
		return VendorVariantStats{}, err
	}
	return backend.VendorVariantStats(ctx, orgID)
}

// ProductsByIDs loads the shared-catalogue products a page of variants points
// at, in one query.
//
// It reads as the system: a vendor's variant links to a product owned by the
// organisation that holds the shared catalogue, and they must be able to see
// the name of the thing they are selling.
func (s *Service) ProductsByIDs(ctx context.Context, ids []int64) (map[int64]*Product, error) {
	backend, err := s.vendorBackend()
	if err != nil {
		return nil, err
	}
	return backend.ListProductsByIDs(database.AsSystem(ctx), ids)
}
