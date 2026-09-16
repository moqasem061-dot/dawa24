package org

import (
	"time"

	"github.com/google/uuid"
)

// OrganizationDeletionRequestStatus represents the status of an organization deletion request.
type OrganizationDeletionRequestStatus string

const (
	OrgDeletionStatusPending   OrganizationDeletionRequestStatus = "pending"
	OrgDeletionStatusApproved  OrganizationDeletionRequestStatus = "approved"
	OrgDeletionStatusRejected  OrganizationDeletionRequestStatus = "rejected"
	OrgDeletionStatusCancelled OrganizationDeletionRequestStatus = "cancelled"
)

// OrganizationDeletionRequest is a formal request by an organization owner to delete the organization.
type OrganizationDeletionRequest struct {
	ID             int64                             `json:"id"`
	PublicID       uuid.UUID                         `json:"public_id"`
	OrganizationID int64                             `json:"organization_id"`
	RequestedBy    int64                             `json:"requested_by"`
	Reason         string                            `json:"reason"`
	Status         OrganizationDeletionRequestStatus `json:"status"`
	AdminNotes     string                            `json:"admin_notes"`
	ReviewedBy     *int64                            `json:"reviewed_by,omitempty"`
	ReviewedAt     *time.Time                        `json:"reviewed_at,omitempty"`
	CreatedAt      time.Time                         `json:"created_at"`
	UpdatedAt      time.Time                         `json:"updated_at"`

	// Enriched fields for Admin UI and Details
	OrgName        string `json:"org_name,omitempty"`
	OrgType        string `json:"org_type,omitempty"`
	OrgStatus      string `json:"org_status,omitempty"`
	CommercialReg  string `json:"commercial_register,omitempty"`
	TaxNumber      string `json:"tax_number,omitempty"`
	BranchesCount  int    `json:"branches_count,omitempty"`
	RequesterName  string `json:"requester_name,omitempty"`
	RequesterEmail string `json:"requester_email,omitempty"`
	RequesterPhone string `json:"requester_phone,omitempty"`
	ReviewerName   string `json:"reviewer_name,omitempty"`
}

// CanCancel reports whether this deletion request can be cancelled by the owner.
func (r *OrganizationDeletionRequest) CanCancel() bool {
	return r != nil && r.Status == OrgDeletionStatusPending
}
