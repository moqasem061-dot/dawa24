package catalog

import (
	"context"
	"strconv"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// SavingProductAdminView represents a saving product with associated user, org, and master product details.
type SavingProductAdminView struct {
	ID                int64        `json:"id"`
	PublicID          string       `json:"public_id"`
	UserID            *int64       `json:"user_id,omitempty"`
	UserName          string       `json:"user_name"`
	UserEmail         string       `json:"user_email"`
	UserPhone         string       `json:"user_phone"`
	OrganizationID    int64        `json:"organization_id"`
	OrganizationName  string       `json:"organization_name"`
	OrganizationType  string       `json:"organization_type"`
	ProductID         *int64       `json:"product_id,omitempty"`
	MasterProductName string       `json:"master_product_name"`
	MasterProductSKU  string       `json:"master_product_sku"`
	NameProduct       string       `json:"name_product"`
	SKU               string       `json:"sku,omitempty"`
	Quantity          float64      `json:"quantity"`
	Price             money.Amount `json:"price"`
	TotalValue        money.Amount `json:"total_value"`
	CreatedAt         time.Time    `json:"created_at"`
	UpdatedAt         time.Time    `json:"updated_at"`
}

// SavingProductAdminStats holds aggregate counts for platform administration.
type SavingProductAdminStats struct {
	TotalProducts      int          `json:"total_products"`
	FilteredCount      int          `json:"filtered_count"`
	TotalUsers         int          `json:"total_users"`
	TotalOrganizations int          `json:"total_organizations"`
	TotalQuantity      float64      `json:"total_quantity"`
	TotalValue         money.Amount `json:"total_value"`
	CountLinked        int          `json:"count_linked"`
	CountUnlinked      int          `json:"count_unlinked"`
}

// ProductAlert represents user notification triggers for price drops and restocks.
type ProductAlert struct {
	ID          int64        `json:"id"`
	PublicID    string       `json:"public_id"`
	UserID      int64        `json:"user_id"`
	ProductID   int64        `json:"product_id"`
	AlertType   string       `json:"alert_type"`
	TargetPrice money.Amount `json:"target_price"`
	IsTriggered bool         `json:"is_triggered"`
	TriggeredAt *time.Time   `json:"triggered_at,omitempty"`
	CreatedAt   time.Time    `json:"created_at"`
}

// InstitutionalGate resolves which institutional work ids a user may see products for.
// Implemented by the org module and injected at composition time in cmd/server/routes.go — modules must not import each other (ADR 0002).
type InstitutionalGate interface {
	AllowedWorkIDs(ctx context.Context, userID int64, mode int) ([]int64, error)
}

// InstitutionalGateFunc adapts a standard function to InstitutionalGate.
type InstitutionalGateFunc func(ctx context.Context, userID int64, mode int) ([]int64, error)

func (f InstitutionalGateFunc) AllowedWorkIDs(ctx context.Context, userID int64, mode int) ([]int64, error) {
	return f(ctx, userID, mode)
}

// ProductIndexItem represents a denormalized read-model record in catalog.product_index.
// Matches Laravel product_infos / product_search_index 25-column specification.
type ProductIndexItem struct {
	UniqueRowID          string       `json:"unique_row_id"`
	ProductID            int64        `json:"product_id"`
	VariantID            *int64       `json:"variant_id,omitempty"`
	SKU                  string       `json:"sku,omitempty"`
	NameAR               string       `json:"name_ar,omitempty"`
	NameEN               string       `json:"name_en,omitempty"`
	SearchText           string       `json:"search_text,omitempty"`
	SearchAR             string       `json:"search_ar,omitempty"`
	SearchEN             string       `json:"search_en,omitempty"`
	SearchSimple         string       `json:"search_simple,omitempty"`
	OrganizationName     string       `json:"organization_name,omitempty"`
	BranchCity           string       `json:"branch_city,omitempty"`
	ScientificName       string       `json:"scientific_name,omitempty"`
	Price                money.Amount `json:"price"`
	Discount             money.Amount `json:"discount"`
	StockQuantity        int          `json:"stock_quantity"`
	CategoryID           *int64       `json:"category_id,omitempty"`
	BrandID              *int64       `json:"brand_id,omitempty"`
	HasDiscount          bool         `json:"has_discount"`
	DiscountPercentage   float64      `json:"discount_percentage"`
	PriceAfterDiscount   money.Amount `json:"price_after_discount"`
	OrganizationID       int64        `json:"organization_id"`
	BranchID             *int64       `json:"branch_id,omitempty"`
	Status               string       `json:"status"`
	ProductType          string       `json:"product_type"` // parent, child
	InstitutionalWorkIDs []int64      `json:"institutional_work_ids"`
	CreatedAt            time.Time    `json:"created_at"`
	UpdatedAt            time.Time    `json:"updated_at"`
}

// ComposeUniqueRowID builds the canonical deterministic key for a read-model item.
func ComposeUniqueRowID(productID int64, variantID, branchID *int64) string {
	if variantID != nil && *variantID > 0 {
		if branchID != nil && *branchID > 0 {
			return "p_" + strconv.FormatInt(productID, 10) + "_v_" + strconv.FormatInt(*variantID, 10) + "_b_" + strconv.FormatInt(*branchID, 10)
		}
		return "p_" + strconv.FormatInt(productID, 10) + "_v_" + strconv.FormatInt(*variantID, 10)
	}
	if branchID != nil && *branchID > 0 {
		return "p_" + strconv.FormatInt(productID, 10) + "_b_" + strconv.FormatInt(*branchID, 10)
	}
	return "p_" + strconv.FormatInt(productID, 10)
}

// MatchDecisionView represents a record in the AI & matching decision memory cache.
type MatchDecisionView struct {
	ID                  int64      `json:"id"`
	OrganizationID      *int64     `json:"organization_id,omitempty"`
	OrganizationName    string     `json:"organization_name,omitempty"`
	UserID              *int64     `json:"user_id,omitempty"`
	UserName            string     `json:"user_name,omitempty"`
	DecisionKey         string     `json:"decision_key"`
	NormName            string     `json:"norm_name"`
	ChosenProductID     *int64     `json:"chosen_product_id,omitempty"`
	ChosenProductName   string     `json:"chosen_product_name,omitempty"`
	ChosenProductSKU    string     `json:"chosen_product_sku,omitempty"`
	Confidence          float64    `json:"confidence"`
	Reason              string     `json:"reason,omitempty"`
	PromptVersion       string     `json:"prompt_version"`
	HitCount            int64      `json:"hit_count"`
	Scope               string     `json:"scope"`
	Source              string     `json:"source"`
	PromotedBy          *int64     `json:"promoted_by,omitempty"`
	PromotedAt          *time.Time `json:"promoted_at,omitempty"`
	IsPlatformInherited bool       `json:"is_platform_inherited,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	LastUsedAt          time.Time  `json:"last_used_at"`
}

// DecisionMemoryFilter specifies rich filtering options for admin and org views.
type DecisionMemoryFilter struct {
	Search         string
	OrganizationID *int64
	UserID         *int64
	Scope          string // "org", "platform", or ""
	Source         string // "ai", "manual", "admin", "import", or ""
	MinConfidence  *float64
	MaxConfidence  *float64
	OnlyUnlinked   bool // chosen_product_id IS NULL
	CreatedFrom    *time.Time
	CreatedTo      *time.Time
	LastUsedFrom   *time.Time
	LastUsedTo     *time.Time
	MinHitCount    *int64
	PromptVersion  string
	Limit          int
	Offset         int
}

// DecisionMemoryPreference records per-organisation opt-out of platform-wide memory.
type DecisionMemoryPreference struct {
	OrganizationID    int64     `json:"organization_id"`
	UsePlatformMemory bool      `json:"use_platform_memory"`
	UpdatedBy         *int64    `json:"updated_by,omitempty"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// CustomerMappingView represents a customer/vendor saved product mapping record.
type CustomerMappingView struct {
	ID             int64     `json:"id"`
	OrganizationID int64     `json:"organization_id"`
	RawName        string    `json:"raw_name"`
	ProductID      int64     `json:"product_id"`
	ProductName    string    `json:"product_name"`
	ProductSKU     string    `json:"product_sku"`
	Source         string    `json:"source"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}
