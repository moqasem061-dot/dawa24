package catalog

import (
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// ProductStatus defines lifecycle states for products and variants.
type ProductStatus string

const (
	StatusActive   ProductStatus = "active"
	StatusInactive ProductStatus = "inactive"
	// StatusPending exists only to read rows written before the catalogue had
	// no review queue. Nothing produces it any more: an administrator importing
	// a product is the act that approves it, and a vendor import cannot create
	// products at all. Treat it as active wherever it is encountered.
	StatusPending  ProductStatus = "pending"
	StatusRejected ProductStatus = "rejected"
)

// Category represents a hierarchical product category.
type Category struct {
	ID          int64      `json:"id"`
	PublicID    string     `json:"public_id"`
	ParentID    *int64     `json:"parent_id,omitempty"`
	Name        i18n.Text  `json:"name"`
	Description i18n.Text  `json:"description,omitempty"`
	Icon        string     `json:"icon,omitempty"`
	Image       string     `json:"image,omitempty"`
	Status      string     `json:"status"`
	SortOrder   int        `json:"sort_order"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty"`
}

// Brand represents a pharmaceutical brand or manufacturer.
type Brand struct {
	ID          int64      `json:"id"`
	PublicID    string     `json:"public_id"`
	Name        i18n.Text  `json:"name"`
	Description i18n.Text  `json:"description,omitempty"`
	Image       string     `json:"image,omitempty"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty"`
}

// BrandWithCount decorates a Brand with its live product count.
type BrandWithCount struct {
	Brand        *Brand
	ProductCount int
}

// CategoryWithCount decorates a Category with its live product count.
type CategoryWithCount struct {
	Category     *Category
	ProductCount int
}

// Product represents a tenant-owned master catalogue product.
type Product struct {
	ID                     int64         `json:"id"`
	PublicID               string        `json:"public_id"`
	OrganizationID         int64         `json:"organization_id"`
	CategoryID             *int64        `json:"category_id,omitempty"`
	BrandID                *int64        `json:"brand_id,omitempty"`
	BranchID               *int64        `json:"branch_id,omitempty"`
	Name                   i18n.Text     `json:"name"`
	Description            i18n.Text     `json:"description,omitempty"`
	SKU                    string        `json:"sku,omitempty"`
	Barcode                string        `json:"barcode,omitempty"`
	Price                  money.Amount  `json:"price"`
	Discount               money.Amount  `json:"discount"`
	OldPrice               money.Amount  `json:"old_price"`
	Image                  string        `json:"image,omitempty"`
	ImageLink              string        `json:"image_link,omitempty"`
	Status                 ProductStatus `json:"status"`
	SoldTimes              int64         `json:"sold_times"`
	IsFeatured             bool          `json:"is_featured"`
	DosageForm             string        `json:"dosage_form,omitempty"`
	ScientificName         string        `json:"scientific_name,omitempty"`
	Pharmacology           string        `json:"pharmacology,omitempty"`
	Active                 string        `json:"active,omitempty"`
	Concentration          string        `json:"concentration,omitempty"`
	Unit                   string        `json:"unit,omitempty"`
	ManufacturingCompanies string        `json:"manufacturing_companies,omitempty"`
	InstitutionalWorkIDs   []int64       `json:"institutional_work_ids,omitempty"`
	// SourceCategory is the category word an imported row carried, held until
	// the import resolves it to a CategoryID. It is never persisted: the
	// products table stores the resolved id, not the supplier's spelling.
	SourceCategory string     `json:"-"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	DeletedAt      *time.Time `json:"deleted_at,omitempty"`
}

// ProductVariant represents a distinct package, concentration, or SKU variation (was product_childerns).
type ProductVariant struct {
	ID                     int64         `json:"id"`
	PublicID               string        `json:"public_id"`
	OrganizationID         int64         `json:"organization_id"`
	ProductID              int64         `json:"product_id"`
	Name                   i18n.Text     `json:"name"`
	SKU                    string        `json:"sku,omitempty"`
	Barcode                string        `json:"barcode,omitempty"`
	Price                  money.Amount  `json:"price"`                    // سعر الجمهور
	CostPrice              *money.Amount `json:"cost_price,omitempty"`     // سعر التكلفة (اختياري - optional)
	CostDiscountPercentage float64       `json:"cost_discount_percentage"` // خصم التكلفة (%)
	Discount               money.Amount  `json:"discount"`                 // خصم الجمهور
	Unit                   string        `json:"unit,omitempty"`
	Image                  string        `json:"image,omitempty"`
	BatchNumber            string        `json:"batch_number,omitempty"`
	ExpiryDate             *time.Time    `json:"expiry_date,omitempty"`
	MinOrderQty            int           `json:"min_order_qty"`
	// QuotaLimit caps how much ONE buying branch may ever take of this
	// variant. nil means no quota. It is a total across orders, not a per-order
	// maximum: a branch that has taken the whole quota cannot come back, and
	// two branches of the same company each get the full allowance because the
	// allocation is about where the stock lands. The consumption itself lives
	// in commerce (summed from order lines); this is only the cap.
	QuotaLimit *int   `json:"quota_limit,omitempty"`
	BranchID   *int64 `json:"branch_id,omitempty"`
	// StockQty is NOT persisted on this table. catalog.product_variants has no
	// stock column — stock lives in inventory.stocks against a warehouse. This
	// field is a write-side input (a supplier's opening quantity) and a
	// read-side projection; anything that reads it without populating it from
	// inventory is reading a permanent zero. That is why the old cart's stock
	// guard, `if variant.StockQty > 0 && ...`, never fired.
	StockQty             int           `json:"stock_qty"`
	Status               ProductStatus `json:"status"`
	IsFeatured           bool          `json:"is_featured"`
	IsNegotiable         bool          `json:"is_negotiable"`
	VariantType          string        `json:"variant_type,omitempty"` // standard, offer
	OfferID              *int64        `json:"offer_id,omitempty"`
	InstitutionalWorkIDs []int64       `json:"institutional_work_ids,omitempty"`
	CreatedAt            time.Time     `json:"created_at"`
	UpdatedAt            time.Time     `json:"updated_at"`
	DeletedAt            *time.Time    `json:"deleted_at,omitempty"`
}

// HasCost reports whether this variant carries cost information (CostPrice or CostDiscountPercentage).
func (v *ProductVariant) HasCost() bool {
	if v == nil {
		return false
	}
	if v.CostDiscountPercentage > 0 && v.Price.IsPositive() {
		return true
	}
	return v.CostPrice != nil && v.CostPrice.IsPositive()
}

// HasCostPrice reports whether this variant carries cost information.
func (v *ProductVariant) HasCostPrice() bool {
	return v.HasCost()
}

// DiscountedCost computes the unit purchase cost:
// In Egyptian pharmaceutical trade, cost discount % is applied directly to Public Price (سعر الجمهور).
// Effective Cost = Public Price * (1 - CostDiscountPercentage / 100).
// If cost discount % is not given, CostPrice is used directly.
// If neither is present, returns money.Zero.
func (v *ProductVariant) DiscountedCost() money.Amount {
	if v == nil || !v.HasCost() {
		return money.Zero
	}
	if v.CostDiscountPercentage > 0 && v.Price.IsPositive() {
		discMinor := int64(float64(v.Price.Minor()) * (v.CostDiscountPercentage / 100.0))
		return money.FromMinor(v.Price.Minor() - discMinor)
	}
	if v.CostPrice != nil && v.CostPrice.IsPositive() {
		if v.CostDiscountPercentage > 0 {
			discMinor := int64(float64(v.CostPrice.Minor()) * (v.CostDiscountPercentage / 100.0))
			return money.FromMinor(v.CostPrice.Minor() - discMinor)
		}
		return *v.CostPrice
	}
	return money.Zero
}

// EffectiveSellingPrice computes the customer selling price after public discount.
func (v *ProductVariant) EffectiveSellingPrice() money.Amount {
	if v == nil {
		return money.Zero
	}
	if v.Discount.IsPositive() && v.Price.IsPositive() {
		// In catalog schema, Discount stores the discount percentage (e.g. 26.40 for 26.40%).
		// Minor units: 2640 minor = 26.40% = 2640 basis points.
		if v.Discount.Minor() > 0 && v.Discount.Minor() < 10000 {
			return v.Price.ApplyPercent(10000 - v.Discount.Minor())
		}
		if v.Discount.Minor() >= 10000 {
			return money.Zero
		}
	}
	return v.Price
}

// DiscountPercentageFloat returns the variant's plain discount percentage as float64 (e.g. 26.40).
func (v *ProductVariant) DiscountPercentageFloat() float64 {
	if v == nil || !v.Discount.IsPositive() {
		return 0.0
	}
	return float64(v.Discount.Minor()) / 100.0
}

// UnitSellingDiscountAmount computes the discount value conceded on the public price.
func (v *ProductVariant) UnitSellingDiscountAmount() money.Amount {
	if v == nil || !v.Price.IsPositive() {
		return money.Zero
	}
	selling := v.EffectiveSellingPrice()
	if v.Price.Minor() > selling.Minor() {
		return money.FromMinor(v.Price.Minor() - selling.Minor())
	}
	return money.Zero
}

// UnitTotalCost computes the unit cost strictly from purchase cost:
// Cost = Discounted Cost (سعر التكلفة بعد خصم التكلفة على سعر الجمهور).
// Selling discounts on public price are NEVER included in cost.
func (v *ProductVariant) UnitTotalCost() money.Amount {
	if v == nil || !v.HasCost() {
		return money.Zero
	}
	return v.DiscountedCost()
}

// UnitNetProfit computes the vendor's net profit per unit:
// Effective Selling Price - Discounted Cost.
// If no cost price exists, net profit is Zero.
func (v *ProductVariant) UnitNetProfit() money.Amount {
	if v == nil || !v.HasCost() {
		return money.Zero
	}
	selling := v.EffectiveSellingPrice()
	purch := v.DiscountedCost()
	return money.FromMinor(selling.Minor() - purch.Minor())
}

// ProfitMarginPercent computes the profit margin percentage over selling price.
// If no cost price exists or base is zero, returns 0.0%.
func (v *ProductVariant) ProfitMarginPercent() float64 {
	if v == nil || !v.HasCost() {
		return 0.0
	}
	base := v.EffectiveSellingPrice()
	if !base.IsPositive() {
		base = v.Price
	}
	if !base.IsPositive() {
		return 0.0
	}
	profit := v.UnitNetProfit()
	return (float64(profit.Minor()) / float64(base.Minor())) * 100.0
}

// EffectivePrice calculates the final customer price after discount.
func (p *Product) EffectivePrice() money.Amount {
	if p.Discount.IsPositive() && p.Discount.Minor() < p.Price.Minor() {
		eff, err := p.Price.Sub(p.Discount)
		if err == nil {
			return eff
		}
	}
	return p.Price
}

// PublicPrice returns the official retail public price (سعر الجمهور).
func (p *Product) PublicPrice() money.Amount {
	if p.OldPrice.IsPositive() {
		return p.OldPrice
	}
	return p.Price
}

// NetPrice returns the net pharmacy wholesale price (سعر الصيدلية بعد الخصم).
func (p *Product) NetPrice() money.Amount {
	return p.EffectivePrice()
}

// HasDiscount reports whether the product has an active discount.
func (p *Product) HasDiscount() bool {
	if p.Discount.IsPositive() {
		return true
	}
	if p.OldPrice.IsPositive() && p.OldPrice.Minor() > p.Price.Minor() {
		return true
	}
	return false
}

// DiscountPercent calculates the discount percentage relative to public price.
func (p *Product) DiscountPercent() int {
	pub := p.PublicPrice()
	if !pub.IsPositive() {
		return 0
	}
	net := p.NetPrice()
	diff := pub.Minor() - net.Minor()
	if diff <= 0 {
		return 0
	}
	return int((diff * 100) / pub.Minor())
}

// Validate ensures required product attributes are sound before persisting.
func (p *Product) Validate() error {
	if p.Name.IsEmpty() {
		return apperr.Validation("product.name_required", i18n.TDefault("w4_mod.w4str_78_78"), nil)
	}
	if p.Price.IsNegative() {
		return apperr.Validation("product.price_negative", i18n.TDefault("w4_mod.w4str_79_79"), nil)
	}
	return nil
}

// CustomerProductMapping defines a per-customer pricing/discount row and, for
// import rows (071), the customer's own name for the product. Vendor-set rows
// carry customer_org_id; import rows carry NULL and a raw_name.
type CustomerProductMapping struct {
	ID               int64         `json:"id"`
	OrganizationID   int64         `json:"organization_id"`
	CustomerOrgID    *int64        `json:"customer_org_id,omitempty"`
	ProductID        int64         `json:"product_id"`
	ProductVariantID *int64        `json:"product_variant_id,omitempty"`
	RawName          string        `json:"raw_name"`
	BranchID         *int64        `json:"branch_id,omitempty"`
	Price            money.Amount  `json:"price"`
	Discount         *money.Amount `json:"discount,omitempty"` // percent, 2dp
	Source           string        `json:"source"`             // excel | csv | link | manual
	Status           string        `json:"status"`             // pending | processed | rejected
	IsActive         bool          `json:"is_active"`
	CreatedAt        time.Time     `json:"created_at"`
	UpdatedAt        time.Time     `json:"updated_at"`
}

// SavingProduct represents a target-saving or tracked price product for a pharmacy or vendor.
type SavingProduct struct {
	ID             int64        `json:"id"`
	PublicID       string       `json:"public_id"`
	UserID         *int64       `json:"user_id,omitempty"`
	OrganizationID int64        `json:"organization_id"`
	ProductID      *int64       `json:"product_id,omitempty"`
	NameProduct    string       `json:"name_product"`
	SKU            string       `json:"sku,omitempty"`
	Quantity       float64      `json:"quantity"`
	Price          money.Amount `json:"price"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
	DeletedAt      *time.Time   `json:"deleted_at,omitempty"`
}

// SavingProductEnriched represents a saving product with its resolved parent catalog product and supplier counts.
type SavingProductEnriched struct {
	SavingProduct
	LinkedProductName  i18n.Text    `json:"linked_product_name"`
	LinkedProductSKU   string       `json:"linked_product_sku"`
	ProvidingOrgsCount int          `json:"providing_orgs_count"`
	TotalValue         money.Amount `json:"total_value"`
}

// ProductProviderInfo represents an organization/supplier selling a linked master product with their pricing and stock.
type ProductProviderInfo struct {
	OrgID              int64        `json:"org_id"`
	OrgName            i18n.Text    `json:"org_name"`
	VariantID          int64        `json:"variant_id"`
	VariantName        i18n.Text    `json:"variant_name"`
	SKU                string       `json:"sku"`
	Unit               string       `json:"unit"`
	Price              money.Amount `json:"price"`
	Discount           money.Amount `json:"discount"`
	PriceAfterDiscount money.Amount `json:"price_after_discount"`
	StockQuantity      int          `json:"stock_quantity"`
	Status             string       `json:"status"`
	BranchName         i18n.Text    `json:"branch_name"`
}

// SavingProductStats summarizes saving products metrics for a vendor.
type SavingProductStats struct {
	CountAll      int          `json:"count_all"`
	CountLinked   int          `json:"count_linked"`
	CountUnlinked int          `json:"count_unlinked"`
	FilteredCount int          `json:"filtered_count"`
	TotalQuantity float64      `json:"total_quantity"`
	TotalValue    money.Amount `json:"total_value"`
}

// CatalogMatchSource holds core metadata of a master product for high-speed matching.
type CatalogMatchSource struct {
	ID      int64  `json:"id"`
	SKU     string `json:"sku"`
	Barcode string `json:"barcode"`
	NameAr  string `json:"name_ar"`
	NameEn  string `json:"name_en"`
}
