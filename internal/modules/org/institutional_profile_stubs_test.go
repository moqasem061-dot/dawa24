package org_test

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
)

// The profile-section contract, stubbed. These tests are about other things;
// they only need the interface satisfied.
func (m *institutionalMockRepo) ReadProfileSection(context.Context, int64, org.ProfileSection) (org.ProfileFields, error) {
	return org.ProfileFields{}, nil
}
func (m *institutionalMockRepo) ApplyProfileSection(context.Context, int64, org.ProfileSection, org.ProfileFields) error {
	return nil
}
func (m *institutionalMockRepo) ApplyApprovedProfileChange(context.Context, pgx.Tx, *org.ProfileChangeRequest) error {
	return nil
}
func (m *institutionalMockRepo) CreateProfileChangeRequest(context.Context, *org.ProfileChangeRequest) error {
	return nil
}
func (m *institutionalMockRepo) PendingProfileChanges(context.Context, int64) (map[org.ProfileSection]*org.ProfileChangeRequest, error) {
	return map[org.ProfileSection]*org.ProfileChangeRequest{}, nil
}
func (m *institutionalMockRepo) GetProfileChangeRequest(context.Context, int64) (*org.ProfileChangeRequest, error) {
	return &org.ProfileChangeRequest{}, nil
}
func (m *institutionalMockRepo) ListProfileChangeRequests(context.Context, string, int, int) ([]*org.ProfileChangeRequest, int, error) {
	return nil, 0, nil
}
func (m *institutionalMockRepo) ListProfileChangeRequestsWithFilter(context.Context, org.ProfileChangeFilter, int, int) ([]*org.ProfileChangeRequest, int, error) {
	return nil, 0, nil
}
func (m *institutionalMockRepo) ProfileChangeRequestCounts(context.Context) (org.ProfileChangeCounts, error) {
	return org.ProfileChangeCounts{}, nil
}
func (m *institutionalMockRepo) DecideProfileChangeRequest(context.Context, int64, int64, bool, string, func(context.Context, pgx.Tx, *org.ProfileChangeRequest) error) (*org.ProfileChangeRequest, error) {
	return &org.ProfileChangeRequest{}, nil
}
func (m *institutionalMockRepo) WithdrawProfileChangeRequest(context.Context, int64, int64) error {
	return nil
}
