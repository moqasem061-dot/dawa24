package promo

import (
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// Package promo handles vendor discount offers, sponsored promotion packages,
// advertising campaigns, and engagement analytics.
// DiscountType specifies how an offer discount is applied.
type DiscountType string

const (
	DiscountPercentage DiscountType = "percentage"
	DiscountFixed      DiscountType = "fixed"
)

// Offer represents a promotional discount campaign configured by a vendor.
type Offer struct {
	ID             int64        `json:"id"`
	PublicID       string       `json:"public_id"`
	OrganizationID int64        `json:"organization_id"`
	BranchID       *int64       `json:"branch_id,omitempty"` // publishing vendor branch (062)
	Title          i18n.Text    `json:"title"`
	Description    i18n.Text    `json:"description,omitempty"`
	DiscountType   DiscountType `json:"discount_type"`
	DiscountValue  money.Amount `json:"discount_value"`
	MinOrderAmount money.Amount `json:"min_order_amount"` // offer minimum order value (062)
	AdminStatus    string       `json:"admin_status"`     // pending | approved | rejected (062)
	AdminNotes     string       `json:"admin_notes,omitempty"`
	ApprovedAt     *time.Time   `json:"approved_at,omitempty"`
	ApprovedBy     int64        `json:"approved_by,omitempty"`
	RejectedAt     *time.Time   `json:"rejected_at,omitempty"`
	RejectedBy     int64        `json:"rejected_by,omitempty"`
	StartsAt       time.Time    `json:"starts_at"`
	ExpiresAt      time.Time    `json:"expires_at"`
	IsActive       bool         `json:"is_active"`
	ViewsCount     int64        `json:"views_count"`
	ClicksCount    int64        `json:"clicks_count"`
	ProductIDs     []int64      `json:"product_ids,omitempty"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
	DeletedAt      *time.Time   `json:"deleted_at,omitempty"`
}

// OfferProductWithOffer carries an offer line together with the offer it
// belongs to — the shape storefront lookups return.
type OfferProductWithOffer struct {
	Product *OfferProduct
	Offer   *Offer
}

// OfferProduct is one line of an offer: the product (or variant) being sold
// and the custom pricing the vendor set for it (062).

// OfferPackage represents a tier for sponsoring and promoting offers.
type OfferPackage struct {
	ID           int64        `json:"id"`
	PublicID     string       `json:"public_id"`
	Name         i18n.Text    `json:"name"`
	Description  i18n.Text    `json:"description,omitempty"`
	Price        money.Amount `json:"price"`
	DurationDays int          `json:"duration_days"`
	MaxOffers    int          `json:"max_offers"`
	Credits      int          `json:"credits"`    // number of sponsorships included per purchase
	TierLevel    int          `json:"tier_level"` // higher ranks above lower in sponsored ranking
	SortOrder    int          `json:"sort_order"`
	IsFeatured   bool         `json:"is_featured"`
	BadgeColor   string       `json:"badge_color,omitempty"`
	IsActive     bool         `json:"is_active"`
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
}

// RankedSponsorship carries the ranking signal for a sponsored product or offer.
// Higher TierLevel sorts above lower; ties are broken randomly by the caller.
type RankedSponsorship struct {
	ItemType       SponsorshipItemType
	ItemID         int64
	OrganizationID int64
	PackageID      int64
	TierLevel      int
	ExpiresAt      time.Time
}

// OfferSponsorship represents an active sponsored campaign for an offer or product.
type OfferSponsorship struct {
	ID                   int64               `json:"id"`
	PublicID             string              `json:"public_id"`
	OrganizationID       int64               `json:"organization_id"`
	OfferID              int64               `json:"offer_id"`
	PackageID            int64               `json:"package_id"`
	StartsAt             time.Time           `json:"starts_at"`
	ExpiresAt            time.Time           `json:"expires_at"`
	Status               string              `json:"status"`
	ItemType             SponsorshipItemType `json:"item_type"`
	ItemID               int64               `json:"item_id"`
	CreditsUsed          int                 `json:"credits_used"`
	AdminStatus          AdminStatus         `json:"admin_status"`
	AdminNotes           string              `json:"admin_notes,omitempty"`
	ReviewedBy           *int64              `json:"reviewed_by,omitempty"`
	ReviewedAt           *time.Time          `json:"reviewed_at,omitempty"`
	SponsorshipRequestID *int64              `json:"sponsorship_request_id,omitempty"`
	CreatedAt            time.Time           `json:"created_at"`
}

// Ad represents a display banner advertisement with bilingual content,
// media (image or video), a click target, admin approval, and engagement stats.
type Ad struct {
	ID              int64             `json:"id"`
	PublicID        string            `json:"public_id"`
	OrganizationID  *int64            `json:"organization_id,omitempty"`
	Title           string            `json:"title"`
	TitleAr         string            `json:"title_ar,omitempty"`
	TitleEn         string            `json:"title_en,omitempty"`
	AdTextAr        string            `json:"ad_text_ar,omitempty"`
	AdTextEn        string            `json:"ad_text_en,omitempty"`
	ImageURL        string            `json:"image_url"` // legacy, kept for backward compat
	MediaType       AdMediaType       `json:"media_type"`
	MediaURL        string            `json:"media_url"`
	ThumbnailURL    string            `json:"thumbnail_url,omitempty"`
	TargetURL       string            `json:"target_url"`
	ClickTargetType AdClickTarget     `json:"click_target_type"`
	ClickTargetID   *int64            `json:"click_target_id,omitempty"`
	Position        string            `json:"position"`
	IsActive        bool              `json:"is_active"`
	AdminStatus     AdminStatus       `json:"admin_status"`
	AdminNotes      string            `json:"admin_notes,omitempty"`
	ReviewedBy      *int64            `json:"reviewed_by,omitempty"`
	ReviewedAt      *time.Time        `json:"reviewed_at,omitempty"`
	AdPlanID        *int64            `json:"ad_plan_id,omitempty"`
	DurationDays    int               `json:"duration_days"`
	StartsAt        time.Time         `json:"starts_at"`
	ExpiresAt       time.Time         `json:"expires_at"`
	Impressions     int64             `json:"impressions"`
	Clicks          int64             `json:"clicks"`
	CTR             float64           `json:"ctr,omitempty"`
	SupplierName    string            `json:"supplier_name,omitempty"`
	SupplierLogo    string            `json:"supplier_logo,omitempty"`
	PublicPrice     string            `json:"public_price,omitempty"`
	DiscountPercent string            `json:"discount_percent,omitempty"`
	SupplyPrice     string            `json:"supply_price,omitempty"`
	PendingChanges  *AdPendingChanges `json:"pending_changes,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
}

// AdPendingChanges contains proposed updates to an ad awaiting admin approval.
type AdPendingChanges struct {
	TitleAr         string        `json:"title_ar,omitempty"`
	TitleEn         string        `json:"title_en,omitempty"`
	AdTextAr        string        `json:"ad_text_ar,omitempty"`
	AdTextEn        string        `json:"ad_text_en,omitempty"`
	MediaType       AdMediaType   `json:"media_type,omitempty"`
	MediaURL        string        `json:"media_url,omitempty"`
	ThumbnailURL    string        `json:"thumbnail_url,omitempty"`
	Position        string        `json:"position,omitempty"`
	TargetURL       string        `json:"target_url,omitempty"`
	ClickTargetType AdClickTarget `json:"click_target_type,omitempty"`
	ClickTargetID   *int64        `json:"click_target_id,omitempty"`
	SubmittedAt     time.Time     `json:"submitted_at"`
}

// SpecialOffer represents the organization's special offer with product & geographic coverage rules.
type SpecialOffer struct {
	ID                 int64                   `json:"id"`
	PublicID           string                  `json:"public_id"`
	OrganizationID     int64                   `json:"organization_id"`
	OrganizationName   string                  `json:"organization_name,omitempty"`
	BranchID           *int64                  `json:"branch_id,omitempty"`
	BranchName         string                  `json:"branch_name,omitempty"`
	// BranchUnavailable is set when the offer names a branch the supplier has
	// since deleted or deactivated. Buyers cannot see such an offer; the
	// supplier is told so it can pick another branch.
	BranchUnavailable bool `json:"branch_unavailable,omitempty"`
	Title              i18n.Text               `json:"title"`
	Description        i18n.Text               `json:"description,omitempty"`
	DiscountPercentage float64                 `json:"discount_percentage"`
	DiscountAmount     money.Amount            `json:"discount_amount"`
	MinOrderAmount     money.Amount            `json:"min_order_amount"`
	TotalPrice         money.Amount            `json:"total_price"`
	StartDate          *time.Time              `json:"start_date,omitempty"`
	EndDate            *time.Time              `json:"end_date,omitempty"`
	Status             string                  `json:"status"`       // active, inactive, expired, draft
	AdminStatus        string                  `json:"admin_status"` // pending, approved, rejected, changes_requested
	AdminNotes         string                  `json:"admin_notes,omitempty"`
	Image              string                  `json:"image,omitempty"`
	Products           []*SpecialOfferProduct  `json:"products,omitempty"`
	Locations          []*SpecialOfferLocation `json:"locations,omitempty"`
	CreatedAt          time.Time               `json:"created_at"`
	UpdatedAt          time.Time               `json:"updated_at"`
}

// SpecialOfferProduct represents a specific product/variant in a special offer.
type SpecialOfferProduct struct {
	ID                 int64        `json:"id"`
	OfferID            int64        `json:"offer_id"`
	ProductID          int64        `json:"product_id,omitempty"`
	VariantID          int64        `json:"variant_id"`
	VariantName        string       `json:"variant_name,omitempty"`
	OriginalPrice      money.Amount `json:"original_price"`
	CustomPrice        money.Amount `json:"custom_price"`
	DiscountPercentage float64      `json:"discount_percentage"`
	DiscountAmount     money.Amount `json:"discount_amount"`
	Quantity           int          `json:"quantity"`
	CreatedAt          time.Time    `json:"created_at"`
}

// SpecialOfferLocation represents geographic coverage rules for a special offer.
type SpecialOfferLocation struct {
	ID              int64     `json:"id"`
	OfferID         int64     `json:"offer_id"`
	CityID          *int64    `json:"city_id,omitempty"`
	CityName        string    `json:"city_name,omitempty"`
	GovernorateID   *int64    `json:"governorate_id,omitempty"`
	GovernorateName string    `json:"governorate_name,omitempty"`
	AddressAr       string    `json:"address_ar"`
	AddressEn       string    `json:"address_en"`
	Latitude        float64   `json:"latitude"`
	Longitude       float64   `json:"longitude"`
	Radius          int       `json:"radius"`      // in meters
	DayOfWeek       int       `json:"day_of_week"` // 1=Saturday ... 7=Friday
	TimeFrom        string    `json:"time_from,omitempty"`
	TimeTo          string    `json:"time_to,omitempty"`
	Status          string    `json:"status"`       // active, inactive
	AdminStatus     string    `json:"admin_status"` // pending, approved, rejected
	CreatedAt       time.Time `json:"created_at"`
}

// OfferLocationAdminRow represents a single geographic coverage record with its joined offer, supplier, and city metadata.
type OfferLocationAdminRow struct {
	ID              int64     `json:"id"`
	OfferID         int64     `json:"offer_id"`
	OfferTitle      string    `json:"offer_title"`
	OrganizationID  int64     `json:"organization_id"`
	SupplierName    string    `json:"supplier_name"`
	CityID          int64     `json:"city_id"`
	CityName        string    `json:"city_name"`
	GovernorateID   int64     `json:"governorate_id"`
	GovernorateName string    `json:"governorate_name"`
	AddressAr       string    `json:"address_ar"`
	AddressEn       string    `json:"address_en"`
	Latitude        float64   `json:"latitude"`
	Longitude       float64   `json:"longitude"`
	RadiusMeters    int       `json:"radius_meters"`
	DayOfWeek       int       `json:"day_of_week"` // 1=Saturday ... 7=Friday
	TimeFrom        string    `json:"time_from"`
	TimeTo          string    `json:"time_to"`
	Status          string    `json:"status"`       // active | inactive
	AdminStatus     string    `json:"admin_status"` // pending | approved | rejected
	CreatedAt       time.Time `json:"created_at"`
}

// OfferLocationsStats holds summary KPI metrics across the platform.
type OfferLocationsStats struct {
	TotalLocations      int `json:"total_locations"`
	ActiveLocations     int `json:"active_locations"`
	CoveredCities       int `json:"covered_cities"`
	CoveredGovernorates int `json:"covered_governorates"`
	ActiveOffers        int `json:"active_offers"`
}

// OfferLocationsFilter holds filter parameters for listing offer locations.
type OfferLocationsFilter struct {
	OfferID     int64
	Status      string
	AdminStatus string
	Governorate string
	Search      string
	Limit       int
	Offset      int
}

// HighlightSection represents a curated section on the homepage or storefront
// (066: one table serves both the platform and organization-owned rows).
type HighlightSection struct {
	ID             int64                  `json:"id"`
	PublicID       string                 `json:"public_id"`
	OwnerType      string                 `json:"owner_type"`                // platform | organization (066)
	OrganizationID *int64                 `json:"organization_id,omitempty"` // owner when organization (066)
	Title          i18n.Text              `json:"title"`
	Description    i18n.Text              `json:"description,omitempty"`
	SectionType    string                 `json:"section_type,omitempty"` // vision, goals, about, why_us, features, services, achievements, certifications, stats, special_info
	Color          string                 `json:"color,omitempty"`
	Slug           string                 `json:"slug"`
	DisplayOrder   int                    `json:"display_order"`
	IsActive       bool                   `json:"is_active"`
	ShowInHeader   bool                   `json:"show_in_header"`
	Items          []HighlightSectionItem `json:"items,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at,omitempty"`
}

// HighlightSectionItem represents an item within a highlight section.
type HighlightSectionItem struct {
	ID           int64  `json:"id"`
	SectionID    int64  `json:"section_id"`
	ProductID    *int64 `json:"product_id,omitempty"`
	OfferID      *int64 `json:"offer_id,omitempty"`
	DisplayOrder int    `json:"display_order"`
}
