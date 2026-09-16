package org

import (
	"strconv"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// OrganizationType defines the classification of a tenant.
//
// Two types only (Rebuild V2 rule 1): customer (صيدلية) and vendor (مورّد).
// The legacy supplier/company/agency values became 'vendor' and the legacy
// pharmacy/chain_pharmacy/individual values became 'customer' (migration 060).
// is_chain marks a customer with several branches. Platform admin is staff,
// not a type.
type OrganizationType string

const (
	TypeVendor   OrganizationType = "vendor"
	TypeCustomer OrganizationType = "customer"
)

// OrganizationStatus defines the approval state of an organization.
type OrganizationStatus string

const (
	StatusPending   OrganizationStatus = "pending"
	StatusApproved  OrganizationStatus = "approved"
	StatusSuspended OrganizationStatus = "suspended"
	StatusRejected  OrganizationStatus = "rejected"
	StatusDeleted   OrganizationStatus = "deleted"
)

// Organization represents a company or pharmacy organization tenant.
type Organization struct {
	ID                 int64     `json:"id"`
	PublicID           string    `json:"public_id"`
	OrganizationNumber string    `json:"organization_number,omitempty"`
	LegalName          string    `json:"legal_name"`
	TradeName          i18n.Text `json:"trade_name"`
	// Name, Email, Phone and Address are on org.organizations and were not on
	// this type, so the registration review modal could not show them: it read
	// LegalName, which registration frequently leaves empty, and rendered a
	// blank field beside every label.
	//
	// They are populated by GetOrganizationByID. The list queries leave them
	// zero, which the screens handle by falling back rather than by assuming.
	Name               i18n.Text          `json:"name"`
	Email              string             `json:"email,omitempty"`
	Phone              string             `json:"phone,omitempty"`
	Address            string             `json:"address,omitempty"`
	TaxNumber          string             `json:"tax_number"`
	CommercialRegister string             `json:"commercial_register"`
	PharmacistLicense  string             `json:"pharmacist_license,omitempty"`
	VerificationNotes  string             `json:"verification_notes,omitempty"`
	RejectionReason    string             `json:"rejection_reason,omitempty"`
	OwnerID            int64              `json:"owner_id,omitempty"`
	Type               OrganizationType   `json:"type"`
	Status             OrganizationStatus `json:"status"`
	CreditLimit        money.Amount       `json:"credit_limit"`
	PaymentTermsDays   int                `json:"payment_terms_days"`
	MinOrderPrice      money.Amount       `json:"min_order_price"`
	MaxOrderPrice      money.Amount       `json:"max_order_price"`
	IsSponsored        bool               `json:"is_sponsored"`
	SponsoredStartAt   *time.Time         `json:"sponsored_start_at,omitempty"`
	SponsoredEndAt     *time.Time         `json:"sponsored_end_at,omitempty"`
	Rating             int                `json:"rating"`
	Rank               int                `json:"rank"`
	ApprovedAt         *time.Time         `json:"approved_at,omitempty"`
	ApprovedBy         *int64             `json:"approved_by,omitempty"`
	// The organisation's AI gateway credential. Never serialised: the JSON API
	// returned it to any signed-in user, and a key is spendable by whoever has it.
	AIVirtualKey string    `json:"-"`
	AIUserID     string    `json:"-"`
	// Extra temporary connected devices granted by platform administrators.
	ExtraDevices          int        `json:"extra_devices"`
	ExtraDevicesExpiresAt *time.Time `json:"extra_devices_expires_at,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

// ActiveExtraDevices returns the count of extra devices if currently active and not expired.
func (o *Organization) ActiveExtraDevices() int {
	if o == nil {
		return 0
	}
	if o.ExtraDevices > 0 && (o.ExtraDevicesExpiresAt == nil || o.ExtraDevicesExpiresAt.After(time.Now())) {
		return o.ExtraDevices
	}
	return 0
}

// AdminOrgStatsResult provides platform-wide organization metrics in a single aggregation.
type AdminOrgStatsResult struct {
	TotalOrgs       int
	TotalPharmacies int
	TotalVendors    int
	PendingCount    int
	ApprovedCount   int
}

// SupplierOrgProfile holds all fields required for managing a supplier organization's commercial identity and profile.
type SupplierOrgProfile struct {
	ID                 int64        `json:"id"`
	PublicID           string       `json:"public_id"`
	NameAr             string       `json:"name_ar"`
	NameEn             string       `json:"name_en"`
	Type               string       `json:"type"` // "supplier", "company", "agency"
	MinOrderPrice      money.Amount `json:"min_order_price"`
	MaxOrderPrice      money.Amount `json:"max_order_price"`
	OrganizationNumber string       `json:"organization_number"`
	Email              string       `json:"email"`
	Phone              string       `json:"phone"`
	TaxNumber          string       `json:"tax_number"`
	Address            string       `json:"address"`
	DescriptionAr      string       `json:"description_ar"`
	DescriptionEn      string       `json:"description_en"`
	Image              string       `json:"image"`          // Logo URL / path
	CoverageImage      string       `json:"coverage_image"` // Cover URL / path
	Rating             int          `json:"rating"`
	Rank               int          `json:"rank"`
	Status             string       `json:"status"`
	CreatedAt          time.Time    `json:"created_at"`
	UpdatedAt          time.Time    `json:"updated_at"`
}

// Branch represents a physical location or warehouse of an organization.
type Branch struct {
	ID                 int64     `json:"id"`
	PublicID           string    `json:"public_id"`
	OrganizationID     int64     `json:"organization_id"`
	Name               i18n.Text `json:"name"`
	Code               string    `json:"code"`
	Address            string    `json:"address"`
	CityID             *int64    `json:"city_id,omitempty"`
	Latitude           *float64  `json:"latitude,omitempty"`
	Longitude          *float64  `json:"longitude,omitempty"`
	GoogleMapsURL      string    `json:"google_maps_url,omitempty"`
	ManagerID          *int64    `json:"manager_id,omitempty"`
	ManagerName        string    `json:"manager_name,omitempty"`
	ManagerEmail       string    `json:"manager_email,omitempty"`
	ManagerPhone       string    `json:"manager_phone,omitempty"`
	WarehouseType      string    `json:"warehouse_type,omitempty"` // warehouse, fast_hub, pharmacy_branch, cold_depot
	HasColdStorage     bool      `json:"has_cold_storage"`
	CapacitySQM        float64   `json:"capacity_sqm"`
	OperatingHours     string    `json:"operating_hours,omitempty"`
	Status             string    `json:"status"` // active, inactive
	IsMain             bool      `json:"is_main"`
	Phone              string    `json:"phone,omitempty"`
	InstitutionalWorks []string  `json:"institutional_works,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// BranchFilter holds filtering parameters for branch listing.
type BranchFilter struct {
	SearchQuery    string
	OrganizationID int64
	Status         string
}

// AdminBranchStatsResult provides platform-wide branch and warehouse counts.
type AdminBranchStatsResult struct {
	TotalBranches    int
	ActiveBranches   int
	PharmacyBranches int
	VendorWarehouses int
}

// Role represents a per-organization role with configurable permissions.
type Role struct {
	ID             int64     `json:"id"`
	OrganizationID int64     `json:"organization_id"`
	Key            string    `json:"key"`
	Name           i18n.Text `json:"name"`
	Description    string    `json:"description"`
	IsSystem       bool      `json:"is_system"`
	// IsOwner marks the role that holds the whole dashboard. It is not
	// editable and not deletable: an owner who could edit it could remove
	// their own access to the company with no way to restore it.
	IsOwner     bool      `json:"is_owner"`
	Permissions []string  `json:"permissions"`
	CreatedAt   time.Time `json:"created_at"`
}

// CustomRole backwards compatibility alias for Role.
type CustomRole = Role

// Member represents a user's employment and membership in an organization.
type Member struct {
	ID             int64        `json:"id"`
	OrganizationID int64        `json:"organization_id"`
	UserID         int64        `json:"user_id"`
	BranchID       *int64       `json:"branch_id,omitempty"`
	RoleID         int64        `json:"role_id"`
	RoleKey        string       `json:"role_key"`
	OrgRoleID      *int64       `json:"org_role_id,omitempty"`
	EmployeeCode   string       `json:"employee_code,omitempty"`
	JobTitle       string       `json:"job_title,omitempty"`
	BaseSalary     money.Amount `json:"base_salary"`
	VariableSalary money.Amount `json:"variable_salary"`
	IsActive       bool         `json:"is_active"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
}

// MemberPatch names the membership fields one form submitted.
//
// A nil field means the form did not carry it and it must keep its stored
// value. The team screens used to edit through AddMemberDirect — an upsert
// built from a fresh Member — so every field the form omitted was written as
// its zero value and an edit to a job title also cleared the branch, the
// employee code and the role.
type MemberPatch struct {
	BranchID     *int64
	RoleKey      *string
	OrgRoleID    *int64
	EmployeeCode *string
	JobTitle     *string
	IsActive     *bool
}

// EmployeeView bundles member attributes with user profile and branch assignment.
type EmployeeView struct {
	Member     *Member
	UserName   string
	UserEmail  string
	UserPhone  string
	UserStatus string
	UserAvatar string
	RoleName   string
	BranchName string
	IsManager  bool
}

// Review represents a multi-criteria customer/supplier review.
type Review struct {
	ID              int64          `json:"id"`
	PublicID        string         `json:"public_id"`
	OrganizationID  int64          `json:"organization_id"`
	UserID          int64          `json:"user_id"`
	OrderID         *int64         `json:"order_id,omitempty"`
	ShipmentID      *int64         `json:"shipment_id,omitempty"`
	ReviewerOrgID   *int64         `json:"reviewer_org_id,omitempty"`
	ProductID       *int64         `json:"product_id,omitempty"`
	Title           string         `json:"title,omitempty"`
	Rating          int            `json:"rating"` // 1-5 overall score
	ScoreRep        int            `json:"score_rep,omitempty"`
	ScoreQuality    int            `json:"score_quality,omitempty"`
	ScoreSpeed      int            `json:"score_speed,omitempty"`
	ReviewText      string         `json:"review_text,omitempty"`
	Response        string         `json:"response,omitempty"`
	ResponseAt      *time.Time     `json:"response_at,omitempty"`
	RespondedBy     *int64         `json:"responded_by,omitempty"`
	IsVerified      bool           `json:"is_verified"`
	IsApproved      bool           `json:"is_approved"`
	IsPublic        bool           `json:"is_public"`
	Status          string         `json:"status"` // pending, approved, rejected
	HelpfulCount    int            `json:"helpful_count"`
	Context         string         `json:"context"` // supplier, pharmacy, product, delivery
	Ratings         []ReviewRating `json:"ratings,omitempty"`
	VendorOrgName   string         `json:"vendor_org_name,omitempty"`
	ReviewerOrgName string         `json:"reviewer_org_name,omitempty"`
	OrderNumber     string         `json:"order_number,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`

	UpdatedAt time.Time `json:"updated_at"`
}

// ReviewCriterion defines one evaluatable metric (e.g. delivery_speed, packaging).
type ReviewCriterion struct {
	Key       string    `json:"key"`
	Name      i18n.Text `json:"name"`
	Context   string    `json:"context"`
	Weight    float64   `json:"weight"`
	SortOrder int       `json:"sort_order"`
	IsActive  bool      `json:"is_active"`
}

// ReviewRating captures an individual score for one criterion.
type ReviewRating struct {
	ReviewID  int64  `json:"review_id"`
	Criterion string `json:"criterion"`
	Score     int    `json:"score"`
}

// DeliveryBand defines distance-based delivery fees.
type DeliveryBand struct {
	ID             int64        `json:"id"`
	OrganizationID int64        `json:"organization_id"`
	FromMeters     int          `json:"from_meters"`
	ToMeters       int          `json:"to_meters"`
	Fee            money.Amount `json:"fee"`
	IsActive       bool         `json:"is_active"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
}

// Follower represents a customer following an organization for updates.
type Follower struct {
	ID             int64     `json:"id"`
	OrganizationID int64     `json:"organization_id"`
	UserID         int64     `json:"user_id"`
	CreatedAt      time.Time `json:"created_at"`
}

// SocialMedia represents an organization's social media profile link.
type SocialMedia struct {
	ID             int64     `json:"id"`
	PublicID       string    `json:"public_id"`
	OrganizationID int64     `json:"organization_id"`
	Platform       string    `json:"platform"`
	URL            string    `json:"url"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Policy types supported by the organization module.
const (
	PolicyTypeTerms    = "terms"
	PolicyTypeReturns  = "returns"
	PolicyTypePrivacy  = "privacy"
	PolicyTypeShipping = "shipping"
	PolicyTypeWarranty = "warranty"
)

// Policy represents organizational terms, delivery rules, or refund policies.
type Policy struct {
	ID             int64     `json:"id"`
	PublicID       string    `json:"public_id"`
	OrganizationID int64     `json:"organization_id"`
	Title          string    `json:"title"`
	Content        string    `json:"content"`
	PolicyType     string    `json:"policy_type"`
	IsActive       bool      `json:"is_active"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Validate checks organization registration details.
func (o *Organization) Validate() error {
	if o.LegalName == "" {
		return apperr.Validation("org.legal_name_required", "Legal name is required.", nil)
	}
	if o.CommercialRegister == "" {
		return apperr.Validation("org.cr_required", "Commercial registration number is required.", nil)
	}
	if o.Type == "" {
		o.Type = TypeCustomer
	}
	if o.Status == "" {
		o.Status = StatusPending
	}
	return nil
}

// ValidateBranch ensures branch data is valid.
func (b *Branch) Validate() error {
	if b.OrganizationID <= 0 {
		return apperr.Validation("branch.org_required", "Organization ID is required.", nil)
	}
	if b.Code == "" {
		if b.ID > 0 {
			b.Code = "BR-" + strconv.FormatInt(b.ID, 10)
		} else {
			b.Code = "BR-" + strconv.FormatInt(time.Now().Unix(), 10)
		}
	}
	return nil
}
