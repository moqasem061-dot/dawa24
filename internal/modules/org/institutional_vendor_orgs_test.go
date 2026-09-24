package org_test

import (
	"context"

	"github.com/moqasem061-dot/dawa24/internal/modules/org"
)

func (m *institutionalMockRepo) ListUserOrganizationsByVendor(_ context.Context, _ int64, _ string) ([]*org.UserOrganization, error) {
	return []*org.UserOrganization{}, nil
}
func (m *institutionalMockRepo) ListUserOrganizationsByVendorWithTotal(_ context.Context, _ int64, _ string, _, _ int) ([]*org.UserOrganization, int, error) {
	return []*org.UserOrganization{}, 0, nil
}
func (m *institutionalMockRepo) ListAllUserOrganizations(_ context.Context, _ string) ([]*org.UserOrganization, error) {
	return []*org.UserOrganization{}, nil
}
func (m *institutionalMockRepo) ListAllUserOrganizationsWithTotal(_ context.Context, _ string, _, _ int) ([]*org.UserOrganization, int, error) {
	return []*org.UserOrganization{}, 0, nil
}
func (m *institutionalMockRepo) GetMember(context.Context, int64, int64) (*org.Member, error) {
	return &org.Member{ID: 1, OrganizationID: 1, UserID: 1, IsActive: true}, nil
}
func (m *institutionalMockRepo) UpdateMember(context.Context, int64, int64, org.MemberPatch) error {
	return nil
}
func (m *institutionalMockRepo) CountMembersByBranch(context.Context, int64) (map[int64]int, error) {
	return map[int64]int{}, nil
}
func (m *institutionalMockRepo) MemberOrganizations(context.Context, int64) ([]int64, error) {
	return nil, nil
}
