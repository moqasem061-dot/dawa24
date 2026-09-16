package org_test

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/modules/org"
)

type empWorkEntry struct {
	orgID   int64
	userID  int64
	workID  int64
	deleted bool
}

type institutionalMockRepo struct {
	org.Repository
	items       map[int64]*org.InstitutionalWork
	connections map[int64]map[int64]bool
	empWorks    []*empWorkEntry
	nextID      int64
}

func newInstMockRepo() *institutionalMockRepo {
	return &institutionalMockRepo{
		items:       make(map[int64]*org.InstitutionalWork),
		connections: make(map[int64]map[int64]bool),
		empWorks:    make([]*empWorkEntry, 0),
		nextID:      1,
	}
}

func (m *institutionalMockRepo) CreateInstitutionalWork(_ context.Context, iw *org.InstitutionalWork) error {
	iw.ID = m.nextID
	m.nextID++
	m.items[iw.ID] = iw

	m.connections[iw.ID] = make(map[int64]bool)
	for _, toID := range iw.AllowedConnections {
		m.connections[iw.ID][toID] = true
	}
	return nil
}

func (m *institutionalMockRepo) GetInstitutionalWorkByID(_ context.Context, id int64) (*org.InstitutionalWork, error) {
	iw, ok := m.items[id]
	if !ok {
		return nil, nil
	}
	return iw, nil
}

func (m *institutionalMockRepo) UpdateInstitutionalWork(_ context.Context, iw *org.InstitutionalWork) error {
	m.items[iw.ID] = iw
	m.connections[iw.ID] = make(map[int64]bool)
	for _, toID := range iw.AllowedConnections {
		m.connections[iw.ID][toID] = true
	}
	return nil
}

func (m *institutionalMockRepo) DeleteInstitutionalWork(_ context.Context, id int64) error {
	delete(m.items, id)
	delete(m.connections, id)
	return nil
}

func (m *institutionalMockRepo) ToggleInstitutionalWorkStatus(_ context.Context, id int64) error {
	if iw, ok := m.items[id]; ok {
		iw.IsActive = !iw.IsActive
	}
	return nil
}

func (m *institutionalMockRepo) ListInstitutionalWorks(_ context.Context, onlyActive bool) ([]*org.InstitutionalWork, error) {
	var list []*org.InstitutionalWork
	for _, iw := range m.items {
		if !onlyActive || iw.IsActive {
			list = append(list, iw)
		}
	}
	return list, nil
}

func (m *institutionalMockRepo) ListAllFlatInstitutionalWorks(_ context.Context, onlyActive bool) ([]*org.InstitutionalWork, error) {
	return m.ListInstitutionalWorks(context.Background(), onlyActive)
}

func (m *institutionalMockRepo) CanConnectInstitutionalWorks(_ context.Context, fromID, toID int64) (bool, error) {
	if targets, ok := m.connections[fromID]; ok {
		return targets[toID], nil
	}
	return false, nil
}

func (m *institutionalMockRepo) AssignBranchInstitutionalWorks(_ context.Context, _ int64, _ []int64) error {
	return nil
}

func (m *institutionalMockRepo) GetBranchInstitutionalWorks(_ context.Context, _ int64) ([]*org.InstitutionalWork, error) {
	return nil, nil
}

func (m *institutionalMockRepo) GetInstitutionalWorkBySlug(_ context.Context, slug string) (*org.InstitutionalWork, error) {
	for _, iw := range m.items {
		if iw.Slug == slug {
			return iw, nil
		}
	}
	return nil, nil
}

func (m *institutionalMockRepo) AnyBranchHasInstitutionalWork(_ context.Context, _ []int64, _ []int64) (bool, error) {
	return false, nil
}

func (m *institutionalMockRepo) ListBranchesWithoutInstitutionalWorks(_ context.Context) ([]*org.BranchWithoutWorks, error) {
	return nil, nil
}

func (m *institutionalMockRepo) GetReachableBuyerWorksForBranch(_ context.Context, _ int64) ([]*org.InstitutionalWork, error) {
	return nil, nil
}

// Unused organization methods for interface satisfaction
func (m *institutionalMockRepo) CreateOrganization(_ context.Context, _ *org.Organization) error {
	return nil
}
func (m *institutionalMockRepo) GetOrganizationByID(_ context.Context, _ int64) (*org.Organization, error) {
	return nil, nil
}
func (m *institutionalMockRepo) GetSupplierProfile(_ context.Context, _ int64) (*org.SupplierOrgProfile, error) {
	return nil, nil
}
func (m *institutionalMockRepo) UpdateSupplierProfile(_ context.Context, _ *org.SupplierOrgProfile) error {
	return nil
}
func (m *institutionalMockRepo) UpdateOrganizationStatus(_ context.Context, _ int64, _ org.OrganizationStatus) error {
	return nil
}
func (m *institutionalMockRepo) UpdateOrganizationAICredentials(_ context.Context, _ int64, _, _ string) error {
	return nil
}
func (m *institutionalMockRepo) ReviewOrganization(_ context.Context, _ int64, _ org.OrganizationStatus, _, _ string, _ int64) error {
	return nil
}
func (m *institutionalMockRepo) UpdateOrganization(_ context.Context, _ *org.Organization) error {
	return nil
}
func (m *institutionalMockRepo) DeleteOrganization(_ context.Context, _ int64) error { return nil }
func (m *institutionalMockRepo) ListOrganizations(_ context.Context, _ *org.OrganizationType, _ *org.OrganizationStatus, _, _ int) ([]*org.Organization, error) {
	return nil, nil
}
func (m *institutionalMockRepo) ListOrganizationsWithTotal(_ context.Context, _ string, _ *org.OrganizationType, _ *org.OrganizationStatus, _, _ int) ([]*org.Organization, int, error) {
	return nil, 0, nil
}
func (m *institutionalMockRepo) AdminOrgStats(_ context.Context) (org.AdminOrgStatsResult, error) {
	return org.AdminOrgStatsResult{}, nil
}
func (m *institutionalMockRepo) CountOrganizations(_ context.Context, _ *org.OrganizationType, _ *org.OrganizationStatus) (int, error) {
	return 0, nil
}
func (m *institutionalMockRepo) CreateBranch(_ context.Context, _ *org.Branch) error { return nil }
func (m *institutionalMockRepo) GetBranchByID(_ context.Context, _ int64) (*org.Branch, error) {
	return nil, nil
}
func (m *institutionalMockRepo) UpdateBranch(_ context.Context, _ *org.Branch) error { return nil }
func (m *institutionalMockRepo) DeleteBranch(_ context.Context, _, _ int64) error    { return nil }
func (m *institutionalMockRepo) ListBranchesByOrg(_ context.Context, _ int64) ([]*org.Branch, error) {
	return nil, nil
}
func (m *institutionalMockRepo) ListBranchesWithTotal(_ context.Context, _ org.BranchFilter, _, _ int) ([]*org.Branch, int, error) {
	return nil, 0, nil
}
func (m *institutionalMockRepo) AdminBranchStats(_ context.Context) (org.AdminBranchStatsResult, error) {
	return org.AdminBranchStatsResult{}, nil
}
func (m *institutionalMockRepo) CountBranchesByOrg(_ context.Context) (map[int64]int, error) {
	return nil, nil
}
func (m *institutionalMockRepo) GetBranchesByIDs(_ context.Context, _ []int64) ([]*org.Branch, error) {
	return nil, nil
}
func (m *institutionalMockRepo) GetOrganizationsByIDs(_ context.Context, _ []int64) ([]*org.Organization, error) {
	return nil, nil
}
func (m *institutionalMockRepo) UnsetMainBranches(_ context.Context, _ int64) error { return nil }
func (m *institutionalMockRepo) AssignBranchManager(_ context.Context, _, _ int64, _ *int64) error {
	return nil
}
func (m *institutionalMockRepo) AddMember(_ context.Context, _ *org.Member) error { return nil }
func (m *institutionalMockRepo) UpdateMemberRole(_ context.Context, _, _ int64, _ string) error {
	return nil
}
func (m *institutionalMockRepo) ListMembersByOrg(_ context.Context, _ int64) ([]*org.Member, error) {
	return nil, nil
}
func (m *institutionalMockRepo) ListEmployees(_ context.Context, _ int64) ([]*org.EmployeeView, error) {
	return nil, nil
}
func (m *institutionalMockRepo) ListEmployeesWithTotal(_ context.Context, _ int64, _, _ int) ([]*org.EmployeeView, int, error) {
	return nil, 0, nil
}
func (m *institutionalMockRepo) ListMembersHolding(_ context.Context, _ int64, _ string) ([]*org.EmployeeView, error) {
	return nil, nil
}
func (m *institutionalMockRepo) RemoveMember(_ context.Context, _, _ int64) error { return nil }
func (m *institutionalMockRepo) CreateRole(_ context.Context, _ *org.Role) error  { return nil }
func (m *institutionalMockRepo) GetRole(_ context.Context, _, _ int64) (*org.Role, error) {
	return nil, nil
}

func (m *institutionalMockRepo) UpdateRole(_ context.Context, _ int64, _ *org.Role) error { return nil }

func (m *institutionalMockRepo) DeleteRole(_ context.Context, _, _ int64) error { return nil }

func (m *institutionalMockRepo) ListRoles(_ context.Context, _ int64) ([]*org.Role, error) {
	return nil, nil
}

func (m *institutionalMockRepo) CountRoleMembers(_ context.Context, _ int64) (map[int64]int, error) {
	return map[int64]int{}, nil
}

func (m *institutionalMockRepo) AssignMemberRole(_ context.Context, _, _, _ int64) error { return nil }
func (m *institutionalMockRepo) GetDeliveryBands(_ context.Context, _ int64) ([]*org.DeliveryBand, error) {
	return nil, nil
}
func (m *institutionalMockRepo) SaveDeliveryBands(_ context.Context, _ int64, _ []*org.DeliveryBand) error {
	return nil
}
func (m *institutionalMockRepo) AddReview(_ context.Context, _ *org.Review) error { return nil }
func (m *institutionalMockRepo) AddReviewWithRatings(_ context.Context, _ *org.Review, _ []org.ReviewRating) error {
	return nil
}
func (m *institutionalMockRepo) ListReviewsByOrg(_ context.Context, _ int64, _, _ int) ([]*org.Review, error) {
	return nil, nil
}
func (m *institutionalMockRepo) GetReviewCriteria(_ context.Context, _ string) ([]*org.ReviewCriterion, error) {
	return nil, nil
}
func (m *institutionalMockRepo) ReplyToReview(_ context.Context, _, _ int64, _ string, _ int64) error {
	return nil
}
func (m *institutionalMockRepo) ListReviewsForVendor(_ context.Context, _ int64, _, _ int) ([]*org.Review, error) {
	return nil, nil
}
func (m *institutionalMockRepo) GetReviewByOrderAndVendor(_ context.Context, _, _ int64) (*org.Review, error) {
	return nil, nil
}
func (m *institutionalMockRepo) ListReviewsForOrder(_ context.Context, _ int64) ([]*org.Review, error) {
	return nil, nil
}
func (m *institutionalMockRepo) HasDeliveredOrderFromVendor(_ context.Context, _, _ int64) (bool, error) {
	return true, nil
}
func (m *institutionalMockRepo) ListAdminReviewsWithTotal(_ context.Context, _ org.AdminReviewFilter) ([]*org.Review, int, error) {
	return nil, 0, nil
}
func (m *institutionalMockRepo) GetAdminReviewStats(_ context.Context) (*org.AdminReviewStats, error) {
	return &org.AdminReviewStats{}, nil
}
func (m *institutionalMockRepo) UpdateReviewStatus(_ context.Context, _ int64, _ bool) error {
	return nil
}
func (m *institutionalMockRepo) SoftDeleteReview(_ context.Context, _ int64) error {
	return nil
}
func (m *institutionalMockRepo) ToggleFollower(_ context.Context, _, _ int64) (bool, error) {
	return false, nil
}
func (m *institutionalMockRepo) IsFollowing(_ context.Context, _, _ int64) (bool, error) {
	return false, nil
}
func (m *institutionalMockRepo) ListFollowedOrgs(_ context.Context, _ int64) ([]*org.Organization, error) {
	return nil, nil
}
func (m *institutionalMockRepo) CreatePolicy(_ context.Context, _ *org.Policy) error { return nil }
func (m *institutionalMockRepo) ListPoliciesByOrg(_ context.Context, _ int64) ([]*org.Policy, error) {
	return nil, nil
}
func (m *institutionalMockRepo) SavePolicies(_ context.Context, _ int64, _ []*org.Policy) error {
	return nil
}
func (m *institutionalMockRepo) ListSocialMediaByOrg(_ context.Context, _ int64) ([]*org.SocialMedia, error) {
	return nil, nil
}
func (m *institutionalMockRepo) SaveSocialMedia(_ context.Context, _ int64, _ []*org.SocialMedia) error {
	return nil
}

// Employee Institutional Works mock
func (m *institutionalMockRepo) AssignEmployeeInstitutionalWork(_ context.Context, orgID, userID, workID int64) error {
	m.empWorks = append(m.empWorks, &empWorkEntry{orgID: orgID, userID: userID, workID: workID, deleted: false})
	return nil
}

func (m *institutionalMockRepo) RemoveEmployeeInstitutionalWork(_ context.Context, orgID, userID, workID int64) error {
	for _, e := range m.empWorks {
		if e.userID == userID && e.workID == workID && (orgID == 0 || e.orgID == orgID) {
			e.deleted = true
		}
	}
	return nil
}

func (m *institutionalMockRepo) ListEmployeeInstitutionalWorks(_ context.Context, userID int64) ([]*org.EmployeeInstitutionalWork, error) {
	var list []*org.EmployeeInstitutionalWork
	for _, e := range m.empWorks {
		if e.userID == userID && !e.deleted {
			list = append(list, &org.EmployeeInstitutionalWork{
				OrganizationID:      e.orgID,
				UserID:              e.userID,
				InstitutionalWorkID: e.workID,
			})
		}
	}
	return list, nil
}

func (m *institutionalMockRepo) ListOrgEmployeeInstitutionalWorks(_ context.Context, orgID int64) ([]*org.EmployeeInstitutionalWork, error) {
	var list []*org.EmployeeInstitutionalWork
	for _, e := range m.empWorks {
		if e.orgID == orgID && !e.deleted {
			list = append(list, &org.EmployeeInstitutionalWork{
				OrganizationID:      e.orgID,
				UserID:              e.userID,
				InstitutionalWorkID: e.workID,
			})
		}
	}
	return list, nil
}

func (m *institutionalMockRepo) GetUserInstitutionalWorkIDs(_ context.Context, userID int64) ([]int64, error) {
	var ids []int64
	for _, e := range m.empWorks {
		if e.userID == userID && !e.deleted {
			ids = append(ids, e.workID)
		}
	}
	return ids, nil
}

func (m *institutionalMockRepo) GetConnectedInstitutionalWorkIDs(_ context.Context, fromWorkIDs []int64) ([]int64, error) {
	var res []int64
	seen := make(map[int64]bool)
	for _, fromID := range fromWorkIDs {
		if targets, ok := m.connections[fromID]; ok {
			for toID := range targets {
				if !seen[toID] {
					seen[toID] = true
					res = append(res, toID)
				}
			}
		}
	}
	return res, nil
}

func (m *institutionalMockRepo) ToggleMemberStatus(_ context.Context, _, _ int64) error { return nil }
func (m *institutionalMockRepo) GetMemberByID(_ context.Context, _, _ int64) (*org.Member, error) {
	return nil, nil
}

func (m *institutionalMockRepo) CreateUserOrganization(_ context.Context, uo *org.UserOrganization) error {
	uo.ID = 1
	return nil
}
func (m *institutionalMockRepo) GetUserOrganizationByID(_ context.Context, id int64) (*org.UserOrganization, error) {
	return &org.UserOrganization{ID: id, OrganizationNumber: "NUM1001", Status: org.UserOrgStatusApproved}, nil
}
func (m *institutionalMockRepo) UpdateUserOrganization(_ context.Context, _ int64, _ string, _ org.UserOrganizationStatus, _ string) error {
	return nil
}

func (m *institutionalMockRepo) DeleteUserOrganization(_ context.Context, _ int64) error { return nil }
func (m *institutionalMockRepo) ListUserOrganizationsByUser(_ context.Context, _ int64) ([]*org.UserOrganization, error) {
	return []*org.UserOrganization{}, nil
}
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
