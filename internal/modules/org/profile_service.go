package org

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Saving a company's profile, one section at a time.

// SaveProfileResult says what happened, so the page can tell the difference
// between "saved" and "sent for review" without inspecting the section again.
type SaveProfileResult struct {
	Applied bool
	Request *ProfileChangeRequest
}

// SaveProfileSection applies a section, or opens a change request for it.
//
// Which of the two happens is the section's own rule (ProfileSection.
// NeedsApproval), not the caller's: a handler that decided this would be a
// second place the policy lives.
func (s *Service) SaveProfileSection(ctx context.Context, in SaveProfileSection) (*SaveProfileResult, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}

	previous, err := s.repo.ReadProfileSection(ctx, in.OrganizationID, in.Section)
	if err != nil {
		return nil, err
	}

	if !in.Section.NeedsApproval() {
		if err := s.repo.ApplyProfileSection(ctx, in.OrganizationID, in.Section, in.Fields); err != nil {
			return nil, err
		}
		s.log.InfoContext(ctx, "organization profile section saved",
			"organization_id", in.OrganizationID, "section", in.Section)
		return &SaveProfileResult{Applied: true}, nil
	}

	// A request that changes nothing is not worth a moderator's attention, and
	// it would block the section behind a pending marker for no reason.
	req := &ProfileChangeRequest{
		OrganizationID: in.OrganizationID,
		RequestedBy:    in.UserID,
		Section:        in.Section,
		Proposed:       in.Fields,
		Previous:       previous,
	}
	if len(req.Changed()) == 0 {
		return &SaveProfileResult{Applied: true}, nil
	}

	if err := s.repo.CreateProfileChangeRequest(ctx, req); err != nil {
		return nil, err
	}
	s.log.InfoContext(ctx, "organization profile change requested",
		"organization_id", in.OrganizationID, "section", in.Section, "request_id", req.ID)
	return &SaveProfileResult{Request: req}, nil
}

// ReadProfileSection returns one section's stored values.
func (s *Service) ReadProfileSection(
	ctx context.Context, orgID int64, section ProfileSection,
) (ProfileFields, error) {
	if orgID <= 0 || !section.Valid() {
		return nil, apperr.Validation("org.profile.unknown_section", "Unknown profile section.", nil)
	}
	return s.repo.ReadProfileSection(ctx, orgID, section)
}

// PendingProfileChanges returns a company's open requests keyed by section.
func (s *Service) PendingProfileChanges(
	ctx context.Context, orgID int64,
) (map[ProfileSection]*ProfileChangeRequest, error) {
	if orgID <= 0 {
		return map[ProfileSection]*ProfileChangeRequest{}, nil
	}
	return s.repo.PendingProfileChanges(ctx, orgID)
}

// ListProfileChangeRequests returns one page of the review queue.
func (s *Service) ListProfileChangeRequests(
	ctx context.Context, status string, limit, offset int,
) ([]*ProfileChangeRequest, int, error) {
	if limit <= 0 {
		limit = 25
	}
	return s.repo.ListProfileChangeRequests(ctx, status, limit, offset)
}

// ListProfileChangeRequestsWithFilter returns one page of the review queue matching the filter.
func (s *Service) ListProfileChangeRequestsWithFilter(
	ctx context.Context, filter ProfileChangeFilter, limit, offset int,
) ([]*ProfileChangeRequest, int, error) {
	if limit <= 0 {
		limit = 25
	}
	return s.repo.ListProfileChangeRequestsWithFilter(ctx, filter, limit, offset)
}

// ProfileChangeRequestCounts returns queue counts grouped by status.
func (s *Service) ProfileChangeRequestCounts(ctx context.Context) (ProfileChangeCounts, error) {
	return s.repo.ProfileChangeRequestCounts(ctx)
}

// GetProfileChangeRequest reads one request.
func (s *Service) GetProfileChangeRequest(ctx context.Context, id int64) (*ProfileChangeRequest, error) {
	if id <= 0 {
		return nil, apperr.Validation("org.profile.invalid_request", "A valid request id is required.", nil)
	}
	return s.repo.GetProfileChangeRequest(ctx, id)
}

// DecideProfileChangeRequest approves or rejects one request.
//
// Approving applies the change in the same transaction that records the
// decision, so the two cannot disagree.
func (s *Service) DecideProfileChangeRequest(
	ctx context.Context, id, reviewerID int64, approve bool, notes string,
) (*ProfileChangeRequest, error) {
	if id <= 0 || reviewerID <= 0 {
		return nil, apperr.Validation("org.profile.invalid_request",
			"A valid request and reviewer are required.", nil)
	}
	if !approve && notes == "" {
		return nil, apperr.Validation("org.profile.rejection_needs_reason",
			"A rejection must say why.", nil)
	}

	decided, err := s.repo.DecideProfileChangeRequest(ctx, id, reviewerID, approve, notes,
		func(txCtx context.Context, tx pgx.Tx, req *ProfileChangeRequest) error {
			return s.repo.ApplyApprovedProfileChange(txCtx, tx, req)
		})
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	decided.ReviewedAt = &now
	s.log.InfoContext(ctx, "organization profile change decided",
		"request_id", id, "approved", approve, "reviewer_id", reviewerID)
	return decided, nil
}

// WithdrawProfileChangeRequest lets a company take back its own request.
func (s *Service) WithdrawProfileChangeRequest(ctx context.Context, orgID, id int64) error {
	if orgID <= 0 || id <= 0 {
		return apperr.Validation("org.profile.invalid_request", "A valid request id is required.", nil)
	}
	return s.repo.WithdrawProfileChangeRequest(ctx, orgID, id)
}
