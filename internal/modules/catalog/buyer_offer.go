package catalog

import (
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// BuyerOfferQuery is one page of the buying catalogue, from one buyer's point of view.
type BuyerOfferQuery struct {
	BuyerOrgID     int64   // excluded as a seller; 0 for a signed-out visitor
	BuyerBranchID  int64   // 0 when no branch is selected
	AllowedWorkIDs []int64 // institutional works the buyer branch may buy from
	Weekday        int
	Query          string
	CategoryID     *int64
	BrandID        *int64
	DosageForm     string
	MinPriceMinor  *int64
	MaxPriceMinor  *int64
	OnlyDiscounted bool
	OnlyInStock    bool
	Sort           string
	Limit, Offset  int
	SupplierOrgID  int64 // needed for scoping to one supplier on /suppliers/{id}

	// CoveredVendorOrgIDs restricts the page to suppliers that can actually
	// deliver to the buyer's branch today, and ApplyCoverage says the caller
	// resolved that set rather than leaving it out.
	//
	// The two fields exist because "no covering suppliers" and "coverage was
	// not asked about" are different answers and a nil slice cannot tell them
	// apart. A signed-out visitor is browsing, not buying, and must still see
	// the catalogue; a pharmacy whose branch no supplier reaches must see an
	// honest empty page rather than 1,695 offers it cannot order.
	//
	// This is in the query rather than applied to the returned rows on purpose.
	// Coverage was the last predicate left in Go, and leaving it there is what
	// made the count a lie: the buying catalogue reported 1,695 offers for a
	// Cairo branch and rendered two, because 1,553 of them came from a supplier
	// that does not deliver to Cairo and were dropped after the page had
	// already been cut.
	CoveredVendorOrgIDs    []int64
	CoveredVendorBranchIDs []int64
	// CoveredOrgWideVendorOrgIDs cover from a coverage row bound to no branch,
	// which reaches the supplier's branch-bound listings too (ServesPoint).
	CoveredOrgWideVendorOrgIDs []int64
	ApplyCoverage              bool
}

// BuyerOffer is one sellable supplier offer joined with product and vendor details.
type BuyerOffer struct {
	VariantID            int64        `json:"variant_id"`
	ProductID            int64        `json:"product_id"`
	VendorOrgID          int64        `json:"vendor_org_id"`
	VendorOrgName        string       `json:"vendor_org_name"`
	VendorBranchID       *int64       `json:"vendor_branch_id,omitempty"`
	VendorBranchName     string       `json:"vendor_branch_name,omitempty"`
	CityName             string       `json:"city_name,omitempty"`
	GovernorateName      string       `json:"governorate_name,omitempty"`
	VendorLatitude       *float64     `json:"vendor_latitude,omitempty"`
	VendorLongitude      *float64     `json:"vendor_longitude,omitempty"`
	ProductName          i18n.Text    `json:"product_name"`
	ProductImage         string       `json:"product_image,omitempty"`
	ProductSKU           string       `json:"product_sku,omitempty"`
	ProductBarcode       string       `json:"product_barcode,omitempty"`
	ScientificName       string       `json:"scientific_name,omitempty"`
	DosageForm           string       `json:"dosage_form,omitempty"`
	ManufacturingCompany string       `json:"manufacturing_company,omitempty"`
	BrandID              *int64       `json:"brand_id,omitempty"`
	BrandName            string       `json:"brand_name,omitempty"`
	BrandLogo            string       `json:"brand_logo,omitempty"`
	CategoryID           *int64       `json:"category_id,omitempty"`
	VariantName          i18n.Text    `json:"variant_name"`
	VariantSKU           string       `json:"variant_sku,omitempty"`
	VariantImage         string       `json:"variant_image,omitempty"`
	PublicPrice          money.Amount `json:"public_price"`
	Price                money.Amount `json:"price"`
	OldPrice             money.Amount `json:"old_price"`
	Discount             money.Amount `json:"discount"`
	DiscountPercent      int          `json:"discount_percent"`
	AvailableStock       int          `json:"available_stock"`
	MinOrderQty          int          `json:"min_order_qty"`
	QuotaLimit           int          `json:"quota_limit"`
	ExpiryDate           *time.Time   `json:"expiry_date,omitempty"`
	IsFeatured           bool         `json:"is_featured"`
	IsNegotiable         bool         `json:"is_negotiable"`
	IsSponsored          bool         `json:"is_sponsored"`
	SponsoredTier        int          `json:"sponsored_tier"`
	Status               string       `json:"status"`
}
