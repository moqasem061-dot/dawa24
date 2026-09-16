package org

import (
	"context"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// RegisterOrgInput specifies parameters for registering a tenant organization.
type RegisterOrgInput struct {
	LegalName          string
	TradeName          i18n.Text
	TaxNumber          string
	CommercialRegister string
	Type               OrganizationType
	CreditLimit        money.Amount
	PaymentTermsDays   int
}

// RegisterOrganization registers a new tenant awaiting approval.
func (s *Service) RegisterOrganization(ctx context.Context, input RegisterOrgInput) (*Organization, error) {
	o := &Organization{
		LegalName:          input.LegalName,
		TradeName:          input.TradeName,
		TaxNumber:          input.TaxNumber,
		CommercialRegister: input.CommercialRegister,
		Type:               input.Type,
		Status:             StatusPending,
		CreditLimit:        input.CreditLimit,
		PaymentTermsDays:   input.PaymentTermsDays,
	}

	if err := o.Validate(); err != nil {
		return nil, err
	}

	if err := s.repo.CreateOrganization(ctx, o); err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "organization registered", "org_id", o.ID, "legal_name", o.LegalName, "type", o.Type)
	return o, nil
}

// ApproveOrganization approves an organization tenant.
func (s *Service) ApproveOrganization(ctx context.Context, id int64) error {
	forgetOrganization(ctx, id)
	if err := s.repo.UpdateOrganizationStatus(ctx, id, StatusApproved); err != nil {
		return err
	}
	s.log.InfoContext(ctx, "organization approved", "org_id", id)
	return nil
}

// RejectOrganization rejects an organization tenant.
func (s *Service) RejectOrganization(ctx context.Context, id int64) error {
	forgetOrganization(ctx, id)
	if err := s.repo.UpdateOrganizationStatus(ctx, id, StatusRejected); err != nil {
		return err
	}
	s.log.InfoContext(ctx, "organization rejected", "org_id", id)
	return nil
}

// SuspendOrganization suspends an active organization tenant.
func (s *Service) SuspendOrganization(ctx context.Context, id int64) error {
	forgetOrganization(ctx, id)
	if err := s.repo.UpdateOrganizationStatus(ctx, id, StatusSuspended); err != nil {
		return err
	}
	s.log.InfoContext(ctx, "organization suspended", "org_id", id)
	return nil
}

// ReviewOrganization handles full administrative review with notes, rejection reasons, and audit stamps.
func (s *Service) ReviewOrganization(ctx context.Context, id int64, status OrganizationStatus, notes, rejectionReason string, adminID int64) error {
	forgetOrganization(ctx, id)
	if err := s.repo.ReviewOrganization(ctx, id, status, notes, rejectionReason, adminID); err != nil {
		return err
	}
	s.log.InfoContext(ctx, "organization reviewed", "org_id", id, "status", status, "admin_id", adminID)
	return nil
}

// RequestOrganizationDeletion submits an organization deletion request by the owner.
func (s *Service) RequestOrganizationDeletion(ctx context.Context, orgID, requestedBy int64, reason string) (*OrganizationDeletionRequest, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, apperr.Validation("org.deletion.missing_reason", "يرجى كتابة سبب طلب حذف المنشأة.", nil)
	}

	req := &OrganizationDeletionRequest{
		OrganizationID: orgID,
		RequestedBy:    requestedBy,
		Reason:         reason,
	}

	if err := s.repo.CreateOrgDeletionRequest(ctx, req); err != nil {
		return nil, err
	}

	if s.log != nil {
		s.log.InfoContext(ctx, "organization deletion requested", "org_id", orgID, "requested_by", requestedBy, "request_id", req.ID)
	}
	return req, nil
}

// GetPendingOrganizationDeletion retrieves any active pending deletion request for an organization.
func (s *Service) GetPendingOrganizationDeletion(ctx context.Context, orgID int64) (*OrganizationDeletionRequest, error) {
	return s.repo.GetPendingOrgDeletionRequest(ctx, orgID)
}

// CancelOrganizationDeletion cancels a pending deletion request by the owner.
func (s *Service) CancelOrganizationDeletion(ctx context.Context, orgID, requestID int64) error {
	if err := s.repo.CancelOrgDeletionRequest(ctx, orgID, requestID); err != nil {
		return err
	}
	if s.log != nil {
		s.log.InfoContext(ctx, "organization deletion request cancelled", "org_id", orgID, "request_id", requestID)
	}
	return nil
}

// ListOrganizationDeletionRequests lists deletion requests for administrators.
func (s *Service) ListOrganizationDeletionRequests(ctx context.Context, status string, limit, offset int) ([]*OrganizationDeletionRequest, int, error) {
	return s.repo.ListOrgDeletionRequests(ctx, status, limit, offset)
}

// ReviewOrganizationDeletion decides an organization deletion request (Approve/Reject).
func (s *Service) ReviewOrganizationDeletion(ctx context.Context, requestID, reviewerID int64, approve bool, adminNotes string) (*OrganizationDeletionRequest, error) {
	res, err := s.repo.ReviewOrgDeletionRequest(ctx, requestID, reviewerID, approve, adminNotes)
	if err != nil {
		return nil, err
	}
	if s.log != nil {
		s.log.InfoContext(ctx, "organization deletion request decided", "request_id", requestID, "reviewer_id", reviewerID, "approved", approve)
	}
	return res, nil
}
