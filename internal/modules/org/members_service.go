package org

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Membership reads and writes that address a member row directly.
//
// Split from service.go for the 400-line rule, along the seam the routes
// already draw: everything here is reached from a team screen and addresses one
// person's membership of one company.

// GetMember reads one membership of the caller's organization.
func (s *Service) GetMember(ctx context.Context, orgID, memberID int64) (*Member, error) {
	if orgID <= 0 || memberID <= 0 {
		return nil, apperr.Validation("member.invalid", "Valid organization and member IDs are required.", nil)
	}
	return s.repo.GetMember(ctx, orgID, memberID)
}

// UpdateMember writes only the fields the submitted form carried.
func (s *Service) UpdateMember(ctx context.Context, orgID, memberID int64, patch MemberPatch) error {
	if orgID <= 0 || memberID <= 0 {
		return apperr.Validation("member.invalid", "Valid organization and member IDs are required.", nil)
	}
	if err := s.repo.UpdateMember(ctx, orgID, memberID, patch); err != nil {
		return err
	}
	s.log.InfoContext(ctx, "member updated", "org_id", orgID, "member_id", memberID)
	return nil
}

// CountMembersByBranch returns how many members each branch holds.
func (s *Service) CountMembersByBranch(ctx context.Context, orgID int64) (map[int64]int, error) {
	if orgID <= 0 {
		return map[int64]int{}, nil
	}
	return s.repo.CountMembersByBranch(ctx, orgID)
}

// UserBelongsElsewhere reports whether a user is already a member of some other
// organization.
//
// Adding an employee by email reuses an existing account when one matches. That
// is right for someone who has signed up and is joining their pharmacy; it is
// wrong when the address belongs to a person already working for a different
// company, because AddMember would quietly give this company a membership of
// them. The caller refuses in that case rather than deciding for both parties.
func (s *Service) UserBelongsElsewhere(ctx context.Context, userID, orgID int64) (bool, error) {
	if userID <= 0 {
		return false, nil
	}
	ids, err := s.repo.MemberOrganizations(ctx, userID)
	if err != nil {
		return false, err
	}
	for _, id := range ids {
		if id != orgID {
			return true, nil
		}
	}
	return false, nil
}

// ListSocialMedia returns an organization's social media accounts.
func (s *Service) ListSocialMedia(ctx context.Context, orgID int64) ([]*SocialMedia, error) {
	return s.repo.ListSocialMediaByOrg(ctx, orgID)
}
