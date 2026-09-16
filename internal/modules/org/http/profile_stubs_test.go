package http_test

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
)

// The profile-section contract, stubbed. These tests are about other things;
// they only need the interface satisfied.
func (happyRepo) ReadProfileSection(context.Context, int64, org.ProfileSection) (org.ProfileFields, error) {
	return org.ProfileFields{}, nil
}
func (happyRepo) ApplyProfileSection(context.Context, int64, org.ProfileSection, org.ProfileFields) error {
	return nil
}
func (happyRepo) ApplyApprovedProfileChange(context.Context, pgx.Tx, *org.ProfileChangeRequest) error {
	return nil
}
func (happyRepo) CreateProfileChangeRequest(context.Context, *org.ProfileChangeRequest) error {
	return nil
}
func (happyRepo) PendingProfileChanges(context.Context, int64) (map[org.ProfileSection]*org.ProfileChangeRequest, error) {
	return map[org.ProfileSection]*org.ProfileChangeRequest{}, nil
}
func (happyRepo) GetProfileChangeRequest(context.Context, int64) (*org.ProfileChangeRequest, error) {
	return &org.ProfileChangeRequest{}, nil
}
func (happyRepo) ListProfileChangeRequests(context.Context, string, int, int) ([]*org.ProfileChangeRequest, int, error) {
	return nil, 0, nil
}
func (happyRepo) ListProfileChangeRequestsWithFilter(context.Context, org.ProfileChangeFilter, int, int) ([]*org.ProfileChangeRequest, int, error) {
	return nil, 0, nil
}
func (happyRepo) ProfileChangeRequestCounts(context.Context) (org.ProfileChangeCounts, error) {
	return org.ProfileChangeCounts{}, nil
}
func (happyRepo) DecideProfileChangeRequest(context.Context, int64, int64, bool, string, func(context.Context, pgx.Tx, *org.ProfileChangeRequest) error) (*org.ProfileChangeRequest, error) {
	return &org.ProfileChangeRequest{}, nil
}
func (happyRepo) WithdrawProfileChangeRequest(context.Context, int64, int64) error { return nil }

// The profile-section contract, stubbed. These tests are about other things;
// they only need the interface satisfied.
func (stubRepo) ReadProfileSection(context.Context, int64, org.ProfileSection) (org.ProfileFields, error) {
	return org.ProfileFields{}, nil
}
func (stubRepo) ApplyProfileSection(context.Context, int64, org.ProfileSection, org.ProfileFields) error {
	return nil
}
func (stubRepo) ApplyApprovedProfileChange(context.Context, pgx.Tx, *org.ProfileChangeRequest) error {
	return nil
}
func (stubRepo) CreateProfileChangeRequest(context.Context, *org.ProfileChangeRequest) error {
	return nil
}
func (stubRepo) PendingProfileChanges(context.Context, int64) (map[org.ProfileSection]*org.ProfileChangeRequest, error) {
	return map[org.ProfileSection]*org.ProfileChangeRequest{}, nil
}
func (stubRepo) GetProfileChangeRequest(context.Context, int64) (*org.ProfileChangeRequest, error) {
	return &org.ProfileChangeRequest{}, nil
}
func (stubRepo) ListProfileChangeRequests(context.Context, string, int, int) ([]*org.ProfileChangeRequest, int, error) {
	return nil, 0, nil
}
func (stubRepo) ListProfileChangeRequestsWithFilter(context.Context, org.ProfileChangeFilter, int, int) ([]*org.ProfileChangeRequest, int, error) {
	return nil, 0, nil
}
func (stubRepo) ProfileChangeRequestCounts(context.Context) (org.ProfileChangeCounts, error) {
	return org.ProfileChangeCounts{}, nil
}
func (stubRepo) DecideProfileChangeRequest(context.Context, int64, int64, bool, string, func(context.Context, pgx.Tx, *org.ProfileChangeRequest) error) (*org.ProfileChangeRequest, error) {
	return &org.ProfileChangeRequest{}, nil
}
func (stubRepo) WithdrawProfileChangeRequest(context.Context, int64, int64) error { return nil }
