package pages

import (
	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/ingest"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func resolveTargetID(variantID, productID int64) int64 {
	if variantID > 0 {
		return variantID
	}
	return productID
}

// SupplierVariantCard represents an enriched product variant supplied by a specific vendor.
// This is the core storefront item for the pharmacy catalog (Rebuild V2).
type SupplierVariantCard struct {
	VariantID       int64
	ProductID       int64
	ProductNameAr   string
	ProductNameEn   string
	ProductImage    string
	VariantName     string
	SKU             string
	DosageForm      string
	Manufacturer    string
	BrandID         *int64
	BrandName       string
	BrandLogo       string
	ScientificName  string
	PublicPrice     money.Amount // Suggested consumer retail price
	SupplierID      int64
	SupplierName    string
	SupplierLogo    string
	SupplierRating  float64
	IsVerified      bool
	BranchName      string
	CityName        string
	GovernorateName string
	DistanceKM      float64
	DistanceText    string
	Price           money.Amount // Net pharmacy price
	OriginalPrice   money.Amount // Original list price before offer
	DiscountPercent float64      // 15.5 = 15.5%
	AvailableStock  int
	MinOrderQty     int
	// MaxOrderQty is what this buyer's branch may actually take: the stock,
	// lowered to what is left of the supplier's per-branch quota. Zero means
	// no availability check ran, and the quantity box falls back to the stock.
	MaxOrderQty    int
	BatchNumber    string
	ExpiryDate     string
	IsCovered      bool
	CoverageReason string
	CanAddToCart   bool
	IsNegotiable   bool
	IsSponsored    bool
	SponsoredTier  int   // tier level from sponsorship package (5 Diamond, 4 Platinum, etc.)
	TieBreaker     int64 // random tie breaker for equal tier sponsorships
	IsFavorite     bool
}
type SupplierOffer struct {
	OfferID          int64
	VariantID        int64
	SupplierID       int64
	SupplierName     string
	SupplierRating   float64
	ReviewCount      int
	IsVerified       bool
	Price            money.Amount // final price (after offers) — the buy price
	OldPrice         money.Amount // list price before the offer
	DiscountAmount   money.Amount // saved amount off the list price
	DiscountBPS      int64        // effective percent in basis points (1500 = 15%)
	AvailableStock   int
	MinOrderQty      int
	MaxOrderQty      int
	BatchNumber      string
	ExpiryDate       string
	DeliveryEstimate string
	ColdChain        bool
	BranchName       string
	WarehouseName    string
	CityName         string
	GovernorateName  string
	DistanceKM       float64
	DistanceText     string
	IsCovered        bool
	CoverageReason   string // e.g. i18n.TDefault("w4m_ui.s_3_3")
	CanAddToCart     bool
	IsNegotiable     bool
	IsSponsored      bool // promoted via sponsorship package
}

// VariantDetailPageData contains full data for the dedicated Product Variant Details page.
type VariantDetailPageData struct {
	Variant        *catalog.ProductVariant
	Product        *catalog.Product
	SupplierOrg    *org.Organization
	SupplierBranch *org.Branch
	CurrentOffer   SupplierOffer
	OtherOffers    []SupplierOffer
	AllVariants    []*catalog.ProductVariant
	StockQty       int
	IsCovered      bool
	CoverageReason string
	Rating         float64
	ReviewCount    int
}

// ProductDetailViewData encapsulates complete B2B pharmaceutical presentation.
type ProductDetailViewData struct {
	Product        *catalog.Product
	Variants       []*catalog.ProductVariant
	SupplierOffers []SupplierOffer
	Rating         float64
	ReviewCount    int
	LowestPrice    money.Amount
}

// CatalogPageData encapsulates filter inputs, pagination metadata, and variant card results for the catalog page.
type CatalogPageData struct {
	Variants            []*SupplierVariantCard
	Categories          []*catalog.Category
	Brands              []*catalog.Brand
	Query               string
	CategoryID          *int64
	BrandID             *int64
	MinPrice            string
	MaxPrice            string
	DosageForm          string
	Sort                string
	InStock             bool
	HasDiscount         bool
	ViewMode            string // "grid" or "table"
	Page                int
	PageSize            int
	TotalItems          int
	TotalPages          int
	HasPrev             bool
	HasNext             bool
	PrevPage            int
	NextPage            int
	StartItem           int
	EndItem             int
	ActiveCategory      string
	ActiveBrand         string
	SponsoredProductIDs map[int64]bool // products promoted via sponsorship (shown with "Sponsored" tag, ranked first)
	CatalogAds          []*promo.Ad
	IsAdminStaff        bool
	RequiresBranch      bool
}

// CatalogFilterParams encapsulates filter inputs for the catalog page.
type CatalogFilterParams struct {
	Query       string
	CategoryID  *int64
	BrandID     *int64
	MinPrice    string
	MaxPrice    string
	DosageForm  string
	Sort        string
	InStock     bool
	HasDiscount bool
	ViewMode    string
	Page        int
	PageSize    int
}

// IngestWizardData contains comprehensive state for the multi-step ingest wizard.
type IngestWizardData struct {
	Step             int // 1 = Upload, 2 = Mapping, 3 = Review & Commit
	Session          *ingest.ImportSession
	Sessions         []*ingest.ImportSession
	Rows             []*ingest.ImportRow
	Warehouses       []*inventory.Warehouse
	MasterProducts   []*catalog.Product
	CurrentWarehouse *inventory.Warehouse
	AvailableHeaders []string
	NoticeType       string
	NoticeMessage    string
	ConfidenceFilter string
}

// ActiveBranchName resolves the current active branch name for customer shell or product displays.
func ActiveBranchName(buying authctx.BuyingBranch) string {
	if buying.Active != nil {
		for _, b := range buying.Branches {
			if b.ID == *buying.Active {
				return b.Name
			}
		}
	}
	if len(buying.Branches) > 0 {
		return buying.Branches[0].Name
	}
	return "الفرع المعتمد"
}

// OrderCeiling is the largest quantity a card's number box may offer.
//
// The stock when nothing has capped it, the quota remainder when something has.
// Both cards carry the same rule, so it is written once: a box that offered a
// number the server then refused would put the pharmacy in a loop with no way
// out but guessing.
func (c *SupplierVariantCard) OrderCeiling() int {
	if c == nil {
		return 1
	}
	return orderCeiling(c.MaxOrderQty, c.AvailableStock)
}

// OrderCeiling is the largest quantity this offer's number box may offer.
func (o *SupplierOffer) OrderCeiling() int {
	if o == nil {
		return 1
	}
	return orderCeiling(o.MaxOrderQty, o.AvailableStock)
}

func orderCeiling(maxQty, stock int) int {
	if maxQty > 0 {
		return maxQty
	}
	if stock > 0 {
		return stock
	}
	return 1
}
