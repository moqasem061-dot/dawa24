package org

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// The profile-section half of the mock repository.
//
// It keeps the stored fields per section and the requests in memory, which is
// enough to exercise the rule that matters: an identity change waits for a
// moderator and every other section applies at once.

func (m *mockOrgRepo) profileStore(orgID int64) map[ProfileSection]ProfileFields {
	if m.profiles == nil {
		m.profiles = map[int64]map[ProfileSection]ProfileFields{}
	}
	if m.profiles[orgID] == nil {
		m.profiles[orgID] = map[ProfileSection]ProfileFields{}
	}
	return m.profiles[orgID]
}

func (m *mockOrgRepo) ReadProfileSection(
	_ context.Context, orgID int64, section ProfileSection,
) (ProfileFields, error) {
	stored := m.profileStore(orgID)[section]
	if stored == nil {
		return ProfileFields{}, nil
	}
	out := ProfileFields{}
	for k, v := range stored {
		out[k] = v
	}
	return out, nil
}

func (m *mockOrgRepo) ApplyProfileSection(
	_ context.Context, orgID int64, section ProfileSection, fields ProfileFields,
) error {
	store := m.profileStore(orgID)
	current := store[section]
	if current == nil {
		current = ProfileFields{}
	}
	for k, v := range fields {
		current[k] = v
	}
	store[section] = current
	return nil
}

func (m *mockOrgRepo) ApplyApprovedProfileChange(
	ctx context.Context, _ pgx.Tx, req *ProfileChangeRequest,
) error {
	return m.ApplyProfileSection(ctx, req.OrganizationID, req.Section, req.Proposed)
}

func (m *mockOrgRepo) CreateProfileChangeRequest(_ context.Context, req *ProfileChangeRequest) error {
	for _, existing := range m.changeRequests {
		if existing.OrganizationID == req.OrganizationID &&
			existing.Section == req.Section &&
			existing.Status == ChangePending {
			return apperr.Conflict("org.profile.change_pending",
				"A change to this section is already awaiting review.")
		}
	}
	m.nextID++
	req.ID = m.nextID
	req.Status = ChangePending
	m.changeRequests = append(m.changeRequests, req)
	return nil
}

func (m *mockOrgRepo) PendingProfileChanges(
	_ context.Context, orgID int64,
) (map[ProfileSection]*ProfileChangeRequest, error) {
	out := map[ProfileSection]*ProfileChangeRequest{}
	for _, req := range m.changeRequests {
		if req.OrganizationID == orgID && req.Status == ChangePending {
			out[req.Section] = req
		}
	}
	return out, nil
}

func (m *mockOrgRepo) GetProfileChangeRequest(_ context.Context, id int64) (*ProfileChangeRequest, error) {
	for _, req := range m.changeRequests {
		if req.ID == id {
			return req, nil
		}
	}
	return nil, apperr.NotFound("profile_change_request")
}

func (m *mockOrgRepo) ListProfileChangeRequests(
	_ context.Context, status string, limit, offset int,
) ([]*ProfileChangeRequest, int, error) {
	var all []*ProfileChangeRequest
	for _, req := range m.changeRequests {
		if status == "" || string(req.Status) == status {
			all = append(all, req)
		}
	}
	if offset >= len(all) {
		return nil, len(all), nil
	}
	end := offset + limit
	if end > len(all) {
		end = len(all)
	}
	return all[offset:end], len(all), nil
}

func (m *mockOrgRepo) ListProfileChangeRequestsWithFilter(
	ctx context.Context, filter ProfileChangeFilter, limit, offset int,
) ([]*ProfileChangeRequest, int, error) {
	return m.ListProfileChangeRequests(ctx, filter.Status, limit, offset)
}

func (m *mockOrgRepo) ProfileChangeRequestCounts(
	_ context.Context,
) (ProfileChangeCounts, error) {
	var c ProfileChangeCounts
	for _, req := range m.changeRequests {
		switch req.Status {
		case ChangePending:
			c.Pending++
		case ChangeApproved:
			c.Approved++
		case ChangeRejected:
			c.Rejected++
		case ChangeWithdrawn:
			c.Withdrawn++
		}
		c.All++
	}
	return c, nil
}

func (m *mockOrgRepo) DecideProfileChangeRequest(
	ctx context.Context, id, reviewerID int64, approve bool, notes string,
	apply func(context.Context, pgx.Tx, *ProfileChangeRequest) error,
) (*ProfileChangeRequest, error) {
	for _, req := range m.changeRequests {
		if req.ID != id {
			continue
		}
		if req.Status != ChangePending {
			return nil, apperr.Conflict("org.profile.change_decided",
				"This request has already been decided.")
		}
		if approve {
			req.Status = ChangeApproved
			if apply != nil {
				if err := apply(ctx, nil, req); err != nil {
					return nil, err
				}
			}
		} else {
			req.Status = ChangeRejected
		}
		req.AdminNotes = notes
		req.ReviewedBy = &reviewerID
		return req, nil
	}
	return nil, apperr.NotFound("profile_change_request")
}

func (m *mockOrgRepo) WithdrawProfileChangeRequest(_ context.Context, orgID, id int64) error {
	for _, req := range m.changeRequests {
		if req.ID == id && req.OrganizationID == orgID && req.Status == ChangePending {
			req.Status = ChangeWithdrawn
			return nil
		}
	}
	return apperr.NotFound("profile_change_request")
}
