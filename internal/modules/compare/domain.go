package compare

import (
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// Plan represents a compare-engine subscription tier.
type Plan struct {
	ID            int64          `json:"id"`
	PublicID      string         `json:"public_id"`
	Name          i18n.Text      `json:"name"`
	Slug          string         `json:"slug"`
	Description   i18n.Text      `json:"description"`
	PriceMonthly  money.Amount   `json:"price_monthly"`
	PriceYearly   money.Amount   `json:"price_yearly"`
	PriceLifetime money.Amount   `json:"price_lifetime"`
	Currency      string         `json:"currency"`
	TrialDays     int            `json:"trial_days"`
	IsActive      bool           `json:"is_active"`
	IsPublic      bool           `json:"is_public"`
	IsRecommended bool           `json:"is_recommended"`
	SortOrder     int            `json:"sort_order"`
	Features      []*PlanFeature `json:"features,omitempty"`
	CreatedBy     *int64         `json:"created_by,omitempty"`
	UpdatedBy     *int64         `json:"updated_by,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     *time.Time     `json:"deleted_at,omitempty"`
}

// PlanFeature represents a capability or quota assigned to a plan.
type PlanFeature struct {
	ID          int64     `json:"id"`
	PlanID      int64     `json:"plan_id"`
	Key         string    `json:"key"`
	Name        i18n.Text `json:"name"`
	Description i18n.Text `json:"description"`
	Value       string    `json:"value"`
	ValueType   string    `json:"value_type"`
	IsActive    bool      `json:"is_active"`
	SortOrder   int       `json:"sort_order"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// PlanRequestStatus represents the review state of a plan request.
type PlanRequestStatus string

const (
	RequestPending   PlanRequestStatus = "pending"
	RequestApproved  PlanRequestStatus = "approved"
	RequestRejected  PlanRequestStatus = "rejected"
	RequestCancelled PlanRequestStatus = "cancelled"
)

// PlanRequest represents a tenant's request for plan enrollment or upgrade.
type PlanRequest struct {
	ID              int64             `json:"id"`
	PublicID        string            `json:"public_id"`
	PlanID          int64             `json:"plan_id"`
	OrganizationID  int64             `json:"organization_id"`
	UserID          int64             `json:"user_id"`
	Status          PlanRequestStatus `json:"status"`
	Notes           string            `json:"notes,omitempty"`
	RejectionReason string            `json:"rejection_reason,omitempty"`
	ReviewedBy      *int64            `json:"reviewed_by,omitempty"`
	ReviewedAt      *time.Time        `json:"reviewed_at,omitempty"`
	Plan            *Plan             `json:"plan,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
	DeletedAt       *time.Time        `json:"deleted_at,omitempty"`
}

// SubscriptionStatus represents the life cycle state of an active subscription.
type SubscriptionStatus string

const (
	SubPending   SubscriptionStatus = "pending"
	SubActive    SubscriptionStatus = "active"
	SubExpired   SubscriptionStatus = "expired"
	SubCancelled SubscriptionStatus = "cancelled"
	SubSuspended SubscriptionStatus = "suspended"
)

// Subscription represents an organization's or user's compare-engine subscription.
type Subscription struct {
	ID             int64              `json:"id"`
	PublicID       string             `json:"public_id"`
	PlanID         int64              `json:"plan_id"`
	OrganizationID *int64             `json:"organization_id,omitempty"`
	UserID         int64              `json:"user_id"`
	BillingPeriod  string             `json:"billing_period"`
	PaymentMethod  string             `json:"payment_method"`
	StartsAt       time.Time          `json:"starts_at"`
	EndsAt         *time.Time         `json:"ends_at,omitempty"`
	Status         SubscriptionStatus `json:"status"`
	Seats          int                `json:"seats"`
	Plan           *Plan              `json:"plan,omitempty"`
	CreatedAt      time.Time          `json:"created_at"`
	UpdatedAt      time.Time          `json:"updated_at"`
	DeletedAt      *time.Time         `json:"deleted_at,omitempty"`
}

// IsValid checks whether the subscription is active and not expired.
func (s *Subscription) IsValid() bool {
	if s == nil || s.Status != SubActive {
		return false
	}
	if s.EndsAt != nil && s.EndsAt.Before(time.Now().UTC()) {
		return false
	}
	return true
}

// SubscriptionUser assigns an individual user to a multi-seat organization subscription.
type SubscriptionUser struct {
	ID             int64     `json:"id"`
	SubscriptionID int64     `json:"subscription_id"`
	UserID         int64     `json:"user_id"`
	IsActive       bool      `json:"is_active"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// UserSession represents an active client session for concurrent device limit enforcement.
type UserSession struct {
	ID                 int64      `json:"id"`
	PublicID           string     `json:"public_id"`
	SubscriptionUserID *int64     `json:"subscription_user_id,omitempty"`
	UserID             int64      `json:"user_id"`
	SessionID          string     `json:"session_id"`
	DeviceUUID         string     `json:"device_uuid,omitempty"`
	IsActive           bool       `json:"is_active"`
	DeviceName         string     `json:"device_name,omitempty"`
	DeviceType         string     `json:"device_type,omitempty"`
	Platform           string     `json:"platform,omitempty"`
	PlatformVersion    string     `json:"platform_version,omitempty"`
	Browser            string     `json:"browser,omitempty"`
	BrowserVersion     string     `json:"browser_version,omitempty"`
	IPAddress          string     `json:"ip_address,omitempty"`
	UserAgent          string     `json:"user_agent,omitempty"`
	Country            string     `json:"country,omitempty"`
	City               string     `json:"city,omitempty"`
	LoggedInAt         time.Time  `json:"logged_in_at"`
	LastActivityAt     time.Time  `json:"last_activity_at"`
	LoggedOutAt        *time.Time `json:"logged_out_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// Entitlement answers "what may this user do in the compare tool right now?" (Plan V5 Phase 2 §2.1.4).
type Entitlement struct {
	Active            bool       `json:"active"`
	PlanSlug          string     `json:"plan_slug"`
	MaxActiveFiles    int        `json:"max_active_files"`
	MaxSessions       int        `json:"max_sessions"`
	AIMatchingEnabled bool       `json:"ai_matching_enabled"`
	ExpiresAt         *time.Time `json:"expires_at,omitempty"`
}

// FileVisibility controls whether a file's rows surface on the public market
// discounts page. Moderator temporary warehouses ignore it (they are always
// public); a vendor's compare-tool upload is private until the vendor opts in.
const (
	// A compare file is always private. خصومات السوق العامة is fed only by
	// temporary warehouses; there is no switch that publishes an upload there.
	VisibilityPrivate = "private"
)

// CompareFileStatus represents the processing state of an uploaded spreadsheet.
type CompareFileStatus string

const (
	FileUploaded CompareFileStatus = "uploaded"
	// FileProcessing is a file whose rows are being read right now.
	//
	// It exists so the screen that receives the redirect from an upload can
	// tell "not parsed yet" from "parsed and empty" — FileUploaded means the
	// latter. Without the distinction the column-mapping wizard opened on a
	// file whose columns had not been read, and offered the pharmacy a mapping
	// for a spreadsheet nothing had looked at.
	FileProcessing CompareFileStatus = "processing"
	FileMapping    CompareFileStatus = "mapping"
	FileReady      CompareFileStatus = "ready"
	FileFailed     CompareFileStatus = "failed"
	FileArchived   CompareFileStatus = "archived"
)

// MappingConfig defines column index mappings for spreadsheet parser.
type MappingConfig struct {
	NameCol     *int `json:"name_col,omitempty"`
	PriceCol    *int `json:"price_col,omitempty"`
	DiscountCol *int `json:"discount_col,omitempty"`
	CodeCol     *int `json:"code_col,omitempty"`
}

// ColumnDetection holds auto-detected column mappings with confidence scores.
type ColumnDetection struct {
	NameCol     *int                    `json:"name_col,omitempty"`
	PriceCol    *int                    `json:"price_col,omitempty"`
	DiscountCol *int                    `json:"discount_col,omitempty"`
	CodeCol     *int                    `json:"code_col,omitempty"`
	Confidence  float64                 `json:"confidence"`
	FieldScores map[TargetField]float64 `json:"field_scores,omitempty"`
}

// CompareFile represents an uploaded supplier price & discount spreadsheet.
type CompareFile struct {
	ID               int64             `json:"id"`
	PublicID         string            `json:"public_id"`
	OrganizationID   *int64            `json:"organization_id,omitempty"`
	UserID           int64             `json:"user_id"`
	SupplierName     string            `json:"supplier_name"`
	OriginalFilename string            `json:"original_filename"`
	StorageKey       string            `json:"storage_key"`
	MIMEType         string            `json:"mime_type"`
	SizeBytes        int64             `json:"size_bytes"`
	RowCount         int               `json:"row_count"`
	Status           CompareFileStatus `json:"status"`
	MappingConfig    MappingConfig     `json:"mapping_config"`
	IsTempWarehouse  bool              `json:"is_temp_warehouse"`
	Visibility       string            `json:"visibility"`
	ArchivedAt       *time.Time        `json:"archived_at,omitempty"`
	ArchiveReason    string            `json:"archive_reason,omitempty"`
	ErrorMessage     string            `json:"error_message,omitempty"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
	DeletedAt        *time.Time        `json:"deleted_at,omitempty"`
}

// TempWarehouseLifecycleResult holds counts of affected files and rows from a lifecycle pass.
type TempWarehouseLifecycleResult struct {
	ArchivedFilesCount int64 `json:"archived_files_count"`
	DeletedFilesCount  int64 `json:"deleted_files_count"`
	PurgedRowsCount    int64 `json:"purged_rows_count"`
	PurgedStorageFiles int64 `json:"purged_storage_files"`
}

// AdminTempWarehouseFilter narrows the Super Admin / "my uploads" temporary
// warehouse listing. A temporary warehouse is a compare.files row that is
// either a moderator upload (is_temp_warehouse) or any vendor compare-tool
// upload (organization_id IS NOT NULL).
type AdminTempWarehouseFilter struct {
	Search     string
	Status     *CompareFileStatus
	UploaderID *int64 // filter by compare.files.user_id
	Source     string // "", "moderator", "vendor"
	OwnerOnly  *int64 // when set, force user_id = *OwnerOnly (the "my uploads" page)
	// OwnerIn restricts the listing to a set of uploaders — the moderators
	// reporting to a main moderator, on the "مستودعات المشرفين تحت إدارتي"
	// screen.
	//
	// An EMPTY non-nil slice means "nobody", not "everybody". A main moderator
	// with no team must see an empty list, and a scope that degrades to "no
	// filter" when it holds no ids is the exact shape of an access-control
	// leak. buildAdminTempWarehouseWhere writes FALSE for it rather than
	// omitting the clause.
	OwnerIn   []int64
	SortBy    string // "supplier", "uploader", "type", "rows", "date", "status"
	SortOrder string // "asc", "desc"
}

// AdminTempWarehouse is a compare file enriched with the uploader and vendor
// labels the admin listing shows.
type AdminTempWarehouse struct {
	*CompareFile
	UploaderName string `json:"uploader_name"`
	OrgName      string `json:"org_name"`
}

// FileUploader is one distinct uploader for the admin listing filter dropdown.
type FileUploader struct {
	UserID int64  `json:"user_id"`
	Name   string `json:"name"`
}

// MatchMethod tracks how a row was linked to the canonical master catalog.
type MatchMethod string

const (
	MatchMethodNone         MatchMethod = "none"
	MatchMethodExact        MatchMethod = "exact"
	MatchMethodExactName    MatchMethod = "exact"
	MatchMethodSKU          MatchMethod = "sku"
	MatchMethodBarcode      MatchMethod = "sku"
	MatchMethodNormalized   MatchMethod = "normalized"
	MatchMethodPartial      MatchMethod = "normalized"
	MatchMethodFuzzy        MatchMethod = "fuzzy"
	MatchMethodAI           MatchMethod = "ai"
	MatchMethodManual       MatchMethod = "manual"
	MatchMethodSavedMapping MatchMethod = "manual"
	MatchMethodDirectID     MatchMethod = "exact"
	MatchMethodUnmatched    MatchMethod = "none"
)

// RowMatch records one row's match result for bulk persistence.
type RowMatch struct {
	RowID      int64
	ProductID  *int64
	Confidence float64
	Method     MatchMethod
}

// CompareFileRow represents an individual drug/product item extracted from a spreadsheet.
type CompareFileRow struct {
	ID                 int64          `json:"id"`
	FileID             int64          `json:"file_id"`
	OrganizationID     *int64         `json:"organization_id,omitempty"`
	RowNumber          int            `json:"row_number"`
	RawName            string         `json:"raw_name"`
	NormalizedName     string         `json:"normalized_name"`
	SKU                string         `json:"sku,omitempty"`
	Price              money.Amount   `json:"price"`
	Discount           float64        `json:"discount"`
	PriceAfterDiscount money.Amount   `json:"price_after_discount"`
	MatchedProductID   *int64         `json:"matched_product_id,omitempty"`
	MatchConfidence    float64        `json:"match_confidence"`
	MatchMethod        MatchMethod    `json:"match_method"`
	Meta               map[string]any `json:"meta,omitempty"`
	CreatedAt          time.Time      `json:"created_at"`
}

// CompareFileRowWithSupplier embeds CompareFileRow with its parent supplier name.
type CompareFileRowWithSupplier struct {
	CompareFileRow
	SupplierName string `json:"supplier_name"`
}

// CatalogStatus differentiates between Master Catalog products, Supplier listings, and Supplier gaps.
type CatalogStatus string

const (
	StatusCatalogAndSuppliers CatalogStatus = "catalog_and_suppliers" // Verified in catalog and actively listed by suppliers
	StatusCatalogOnly         CatalogStatus = "catalog_only"          // Exists in catalog but absent from supplier lists
	StatusSupplierCustom      CatalogStatus = "supplier_custom"       // Listed by supplier but not yet mapped to catalog
)

// MarketDiscountsFilter contains filter and pagination parameters for the public market discounts page.
// MarketDiscountsFilter is what a reader may narrow the board by.
//
// There is deliberately no organization field. The board shows the platform's
// own temporary warehouses and nothing else, that is fixed in the query's FROM
// clause, and a filter that could name an organization would be the beginning
// of a way to ask it for a supplier's private upload.
type MarketDiscountsFilter struct {
	Query       string   `json:"query"`
	Supplier    string   `json:"supplier"`
	MinPrice    *float64 `json:"min_price"`
	MaxPrice    *float64 `json:"max_price"`
	MinDiscount *float64 `json:"min_discount"`
	MaxDiscount *float64 `json:"max_discount"`
	SortBy      string   `json:"sort_by"` // "newest", "oldest", "discount_desc", "price_asc", "price_desc"
	Page        int      `json:"page"`
	Limit       int      `json:"limit"`
}

// HeadToHeadOutcome defines comparison relation for a single product between your supplier and competitor.
type HeadToHeadOutcome string

const (
	OutcomeYourBetter       HeadToHeadOutcome = "your_better"       // خصمك أفضل منهم
	OutcomeEqual            HeadToHeadOutcome = "equal"             // نفس الخصم
	OutcomeCompetitorBetter HeadToHeadOutcome = "competitor_better" // خصم أفضل منك
)

// HeadToHeadFilter represents filters applied to supplier vs supplier comparison.
type HeadToHeadFilter struct {
	SourceFileID int64              `json:"source_file_id"`
	TargetFileID int64              `json:"target_file_id"`
	Query        string             `json:"query"`
	MinPrice     *float64           `json:"min_price"`
	MaxPrice     *float64           `json:"max_price"`
	MinDiscount  *float64           `json:"min_discount"`
	MaxDiscount  *float64           `json:"max_discount"`
	Outcome      *HeadToHeadOutcome `json:"outcome"`
}

// HeadToHeadRow represents a product row formatted exactly as requested in the comparison UI.
type HeadToHeadRow struct {
	ProductID          *int64            `json:"product_id,omitempty"`
	ProductName        string            `json:"product_name"`
	SKU                string            `json:"sku,omitempty"`
	Price              money.Amount      `json:"price"`
	YourDiscount       float64           `json:"your_discount"`       // الخصم
	YourNetPrice       money.Amount      `json:"your_net_price"`      // الصافي لك
	CompetitorDiscount float64           `json:"competitor_discount"` // خصم المنافس
	CompetitorNetPrice money.Amount      `json:"competitor_net_price"`
	Outcome            HeadToHeadOutcome `json:"outcome"`
	BetterDiff         float64           `json:"better_diff"`     // فرق الخصم لـ خصمك أفضل منهم
	CompetitorDiff     float64           `json:"competitor_diff"` // فرق الخصم لـ خصم أفضل منك
	SavingsDifference  money.Amount      `json:"savings_difference"`
}
