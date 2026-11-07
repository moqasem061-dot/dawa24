package org

import (
	"time"

	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
	"github.com/moqasem061-dot/dawa24/internal/shared/money"
)

// EmployeeInstitutionalWork links a user to an institutional work group.
type EmployeeInstitutionalWork struct {
	ID                  int64     `json:"id"`
	OrganizationID      int64     `json:"organization_id"`
	UserID              int64     `json:"user_id"`
	InstitutionalWorkID int64     `json:"institutional_work_id"`
	WorkTitle           string    `json:"work_title,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// BranchWithoutWorks represents an active branch that has no resolvable institutional works.
type BranchWithoutWorks struct {
	BranchID      int64  `json:"branch_id"`
	BranchName    string `json:"branch_name"`
	OrgID         int64  `json:"org_id"`
	OrgName       string `json:"org_name"`
	OrgType       string `json:"org_type"`
	WarehouseType string `json:"warehouse_type"`
}

// InstitutionalFilterMode selects Laravel's two documented filter semantics.
type InstitutionalFilterMode int

const (
	// FilterSimple: intersect the user's works directly with the product's,
	// and additionally allow products with no institutional restriction (empty array).
	// Laravel: applyInstitutionalWorksFilter_Simple — customer dashboard.
	FilterSimple InstitutionalFilterMode = iota

	// FilterWithConnections: resolve connections first, then intersect.
	// Products with no restriction are NOT visible.
	// Laravel: applyInstitutionalWorksFilter_WithConnections — purchase-request pages.
	FilterWithConnections
)

// UserOrganizationStatus represents the approval state of a customer-to-vendor organization linkage.
type UserOrganizationStatus string

const (
	UserOrgStatusPending  UserOrganizationStatus = "pending"
	UserOrgStatusApproved UserOrganizationStatus = "approved"
	UserOrgStatusRejected UserOrganizationStatus = "rejected"
)

// UserOrganization links a customer user (pharmacy) with a vendor organization using their organization number.
type UserOrganization struct {
	ID                 int64                  `json:"id"`
	UserID             int64                  `json:"user_id"`
	UserName           string                 `json:"user_name,omitempty"`
	UserEmail          string                 `json:"user_email,omitempty"`
	CustomerOrgID      *int64                 `json:"customer_org_id,omitempty"`
	CustomerOrgName    string                 `json:"customer_org_name,omitempty"`
	VendorOrgID        int64                  `json:"vendor_org_id"`
	VendorOrgName      string                 `json:"vendor_org_name,omitempty"`
	VendorOrgType      string                 `json:"vendor_org_type,omitempty"`
	OrganizationNumber string                 `json:"organization_number"`
	Status             UserOrganizationStatus `json:"status"`
	Notes              string                 `json:"notes,omitempty"`
	CreatedAt          time.Time              `json:"created_at"`
	UpdatedAt          time.Time              `json:"updated_at"`
}

// ChangeRequestStatus defines the state of an organization change request.
type ChangeRequestStatus string

const (
	ChangeRequestPending   ChangeRequestStatus = "pending"
	ChangeRequestApproved  ChangeRequestStatus = "approved"
	ChangeRequestRejected  ChangeRequestStatus = "rejected"
	ChangeRequestCancelled ChangeRequestStatus = "cancelled"
)

// ProfileValues encapsulates commercial and profile snapshot fields for change requests.
type ProfileValues struct {
	NameAr             string       `json:"name_ar"`
	NameEn             string       `json:"name_en"`
	Type               string       `json:"type"` // "supplier", "company", "agency", "customer"
	MinOrderPrice      money.Amount `json:"min_order_price"`
	MaxOrderPrice      money.Amount `json:"max_order_price"`
	OrganizationNumber string       `json:"organization_number"`
	Email              string       `json:"email"`
	Phone              string       `json:"phone"`
	TaxNumber          string       `json:"tax_number"`
	Address            string       `json:"address"`
	DescriptionAr      string       `json:"description_ar"`
	DescriptionEn      string       `json:"description_en"`
	Image              string       `json:"image"`          // Logo URL
	CoverageImage      string       `json:"coverage_image"` // Cover URL
}

// OrganizationChangeRequest records a proposed modification to an organization's profile.
type OrganizationChangeRequest struct {
	ID              int64               `json:"id"`
	PublicID        string              `json:"public_id"`
	OrganizationID  int64               `json:"organization_id"`
	OrgName         string              `json:"org_name,omitempty"`
	OrgType         string              `json:"org_type,omitempty"`
	RequestedBy     *int64              `json:"requested_by,omitempty"`
	RequesterName   string              `json:"requester_name,omitempty"`
	RequesterEmail  string              `json:"requester_email,omitempty"`
	Status          ChangeRequestStatus `json:"status"`
	CurrentValues   ProfileValues       `json:"current_values"`
	ProposedValues  ProfileValues       `json:"proposed_values"`
	ReviewedBy      *int64              `json:"reviewed_by,omitempty"`
	ReviewerName    string              `json:"reviewer_name,omitempty"`
	ReviewedAt      *time.Time          `json:"reviewed_at,omitempty"`
	AdminNotes      string              `json:"admin_notes,omitempty"`
	RejectionReason string              `json:"rejection_reason,omitempty"`
	CreatedAt       time.Time           `json:"created_at"`
	UpdatedAt       time.Time           `json:"updated_at"`
}

// ChangedFieldItem represents one diff item between current and proposed values.
type ChangedFieldItem struct {
	FieldKey    string
	LabelAr     string
	LabelEn     string
	OldValue    string
	NewValue    string
	IsDifferent bool
}

// PricingType defines the commercial tier/billing classification for institutional works.
type PricingType string

const (
	PricingFree         PricingType = "free"
	PricingPaid         PricingType = "paid"
	PricingSubscription PricingType = "subscription"
	PricingPerProject   PricingType = "per_project"
	PricingHourly       PricingType = "hourly"
	PricingMonthly      PricingType = "monthly"
)

// InstitutionalWork represents an organizational structural node/type (الهيكل المؤسسي).
type InstitutionalWork struct {
	ID                     int64                `json:"id"`
	PublicID               string               `json:"public_id"`
	Title                  i18n.Text            `json:"title"`
	Description            i18n.Text            `json:"description"`
	Icon                   string               `json:"icon"`
	PricingType            PricingType          `json:"pricing_type"`
	IsActive               bool                 `json:"is_active"`
	ViewType               int                  `json:"view_type"`
	Slug                   string               `json:"slug"`
	ParentID               *int64               `json:"parent_id,omitempty"`
	ParentTitle            string               `json:"parent_title,omitempty"`
	Children               []*InstitutionalWork `json:"children,omitempty"`
	AllowedConnections     []int64              `json:"allowed_connections,omitempty"`
	AllowedConnectionNames []string             `json:"allowed_connection_names,omitempty"`
	BranchCount            int                  `json:"branch_count,omitempty"`
	Level                  int                  `json:"level,omitempty"`
	CreatedAt              time.Time            `json:"created_at"`
	UpdatedAt              time.Time            `json:"updated_at"`
}

// ParentIDVal returns the parent ID or 0 if nil.
func (iw *InstitutionalWork) ParentIDVal() int64 {
	if iw == nil || iw.ParentID == nil {
		return 0
	}
	return *iw.ParentID
}

// CanConnectTo reports whether this entity is permitted to connect to the target entity ID.
func (iw *InstitutionalWork) CanConnectTo(targetID int64) bool {
	if iw == nil || len(iw.AllowedConnections) == 0 {
		return false
	}
	for _, id := range iw.AllowedConnections {
		if id == targetID {
			return true
		}
	}
	return false
}

// BranchInstitutionalWork joins branches to institutional work types.
type BranchInstitutionalWork struct {
	ID                  int64     `json:"id"`
	BranchID            int64     `json:"branch_id"`
	InstitutionalWorkID int64     `json:"institutional_work_id"`
	CreatedAt           time.Time `json:"created_at"`
}
