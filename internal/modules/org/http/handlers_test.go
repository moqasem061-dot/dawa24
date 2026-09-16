package http_test

import (
	"context"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/org"
)

type stubRepo struct {
	org.Repository
	t *testing.T
}

func (r stubRepo) fail(method string) {
	r.t.Helper()
	r.t.Fatalf("repository.%s was called; the request should have been rejected before reaching the repository", method)
}

func (r stubRepo) CreateOrganization(ctx context.Context, o *org.Organization) error {
	r.fail("CreateOrganization")
	return nil
}
func (r stubRepo) GetOrganizationByID(ctx context.Context, id int64) (*org.Organization, error) {
	r.fail("GetOrganizationByID")
	return nil, nil
}
func (r stubRepo) GetSupplierProfile(ctx context.Context, id int64) (*org.SupplierOrgProfile, error) {
	r.fail("GetSupplierProfile")
	return nil, nil
}
func (r stubRepo) UpdateSupplierProfile(ctx context.Context, p *org.SupplierOrgProfile) error {
	r.fail("UpdateSupplierProfile")
	return nil
}
func (r stubRepo) UpdateOrganizationStatus(ctx context.Context, id int64, status org.OrganizationStatus) error {
	r.fail("UpdateOrganizationStatus")
	return nil
}
func (r stubRepo) UpdateOrganization(ctx context.Context, o *org.Organization) error {
	r.fail("UpdateOrganization")
	return nil
}
func (r stubRepo) DeleteOrganization(ctx context.Context, id int64) error {
	r.fail("DeleteOrganization")
	return nil
}
func (r stubRepo) ListOrganizations(ctx context.Context, orgType *org.OrganizationType, status *org.OrganizationStatus, limit, offset int) ([]*org.Organization, error) {
	r.fail("ListOrganizations")
	return nil, nil
}
func (r stubRepo) ListOrganizationsWithTotal(ctx context.Context, _ string, orgType *org.OrganizationType, status *org.OrganizationStatus, limit, offset int) ([]*org.Organization, int, error) {
	r.fail("ListOrganizationsWithTotal")
	return nil, 0, nil
}
func (r stubRepo) AdminOrgStats(ctx context.Context) (org.AdminOrgStatsResult, error) {
	r.fail("AdminOrgStats")
	return org.AdminOrgStatsResult{}, nil
}
func (r stubRepo) CountBranchesByOrg(ctx context.Context) (map[int64]int, error) {
	r.fail("CountBranchesByOrg")
	return nil, nil
}
func (r stubRepo) GetOrganizationsByIDs(ctx context.Context, ids []int64) ([]*org.Organization, error) {
	r.fail("GetOrganizationsByIDs")
	return nil, nil
}

func (r stubRepo) CreateBranch(ctx context.Context, b *org.Branch) error {
	r.fail("CreateBranch")
	return nil
}
func (r stubRepo) GetBranchByID(ctx context.Context, id int64) (*org.Branch, error) {
	r.fail("GetBranchByID")
	return nil, nil
}
func (r stubRepo) UpdateBranch(ctx context.Context, b *org.Branch) error {
	r.fail("UpdateBranch")
	return nil
}
func (r stubRepo) DeleteBranch(ctx context.Context, id, orgID int64) error {
	r.fail("DeleteBranch")
	return nil
}
func (r stubRepo) ListBranchesByOrg(ctx context.Context, orgID int64) ([]*org.Branch, error) {
	r.fail("ListBranchesByOrg")
	return nil, nil
}
func (r stubRepo) ListBranchesWithTotal(ctx context.Context, filter org.BranchFilter, limit, offset int) ([]*org.Branch, int, error) {
	r.fail("ListBranchesWithTotal")
	return nil, 0, nil
}
func (r stubRepo) AdminBranchStats(ctx context.Context) (org.AdminBranchStatsResult, error) {
	r.fail("AdminBranchStats")
	return org.AdminBranchStatsResult{}, nil
}
func (r stubRepo) GetBranchesByIDs(ctx context.Context, ids []int64) ([]*org.Branch, error) {
	r.fail("GetBranchesByIDs")
	return nil, nil
}
func (r stubRepo) UnsetMainBranches(ctx context.Context, orgID int64) error {
	r.fail("UnsetMainBranches")
	return nil
}

func (r stubRepo) AddMember(ctx context.Context, m *org.Member) error {
	r.fail("AddMember")
	return nil
}
func (r stubRepo) UpdateMemberRole(ctx context.Context, orgID, userID int64, role string) error {
	r.fail("UpdateMemberRole")
	return nil
}
func (r stubRepo) ListMembersByOrg(ctx context.Context, orgID int64) ([]*org.Member, error) {
	r.fail("ListMembersByOrg")
	return nil, nil
}
func (r stubRepo) RemoveMember(ctx context.Context, orgID, userID int64) error {
	r.fail("RemoveMember")
	return nil
}

func (r stubRepo) AddReview(ctx context.Context, rev *org.Review) error {
	r.fail("AddReview")
	return nil
}
func (r stubRepo) ListReviewsByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*org.Review, error) {
	r.fail("ListReviewsByOrg")
	return nil, nil
}
func (r stubRepo) ToggleFollower(ctx context.Context, orgID, userID int64) (bool, error) {
	r.fail("ToggleFollower")
	return false, nil
}
func (r stubRepo) IsFollowing(ctx context.Context, orgID, userID int64) (bool, error) {
	r.fail("IsFollowing")
	return false, nil
}
func (r stubRepo) ListFollowedOrgs(ctx context.Context, userID int64) ([]*org.Organization, error) {
	r.fail("ListFollowedOrgs")
	return nil, nil
}
func (r stubRepo) CreatePolicy(ctx context.Context, p *org.Policy) error {
	r.fail("CreatePolicy")
	return nil
}
func (r stubRepo) ListPoliciesByOrg(ctx context.Context, orgID int64) ([]*org.Policy, error) {
	r.fail("ListPoliciesByOrg")
	return nil, nil
}

func (r stubRepo) ReviewOrganization(ctx context.Context, id int64, status org.OrganizationStatus, notes, rejectionReason string, adminID int64) error {
	r.fail("ReviewOrganization")
	return nil
}
func (r stubRepo) CreateRole(ctx context.Context, role *org.Role) error {
	r.fail("CreateRole")
	return nil
}
func (r stubRepo) GetRole(ctx context.Context, orgID, roleID int64) (*org.Role, error) {
	r.fail("GetRole")
	return nil, nil
}

func (r stubRepo) UpdateRole(ctx context.Context, orgID int64, role *org.Role) error {
	r.fail("UpdateRole")
	return nil
}

func (r stubRepo) DeleteRole(ctx context.Context, orgID, roleID int64) error {
	r.fail("DeleteRole")
	return nil
}

func (r stubRepo) ListRoles(ctx context.Context, orgID int64) ([]*org.Role, error) {
	r.fail("ListRoles")
	return nil, nil
}

func (r stubRepo) CountRoleMembers(ctx context.Context, orgID int64) (map[int64]int, error) {
	r.fail("CountRoleMembers")
	return nil, nil
}

func (r stubRepo) AssignMemberRole(ctx context.Context, orgID, memberID, roleID int64) error {
	r.fail("AssignMemberRole")
	return nil
}
func (r stubRepo) GetDeliveryBands(ctx context.Context, orgID int64) ([]*org.DeliveryBand, error) {
	r.fail("GetDeliveryBands")
	return nil, nil
}
func (r stubRepo) SaveDeliveryBands(ctx context.Context, orgID int64, bands []*org.DeliveryBand) error {
	r.fail("SaveDeliveryBands")
	return nil
}
func (r stubRepo) AddReviewWithRatings(ctx context.Context, rev *org.Review, ratings []org.ReviewRating) error {
	r.fail("AddReviewWithRatings")
	return nil
}
func (r stubRepo) GetReviewCriteria(ctx context.Context, contextType string) ([]*org.ReviewCriterion, error) {
	r.fail("GetReviewCriteria")
	return nil, nil
}
func (r stubRepo) ReplyToReview(ctx context.Context, reviewID, orgID int64, response string, responderID int64) error {
	r.fail("ReplyToReview")
	return nil
}
func (r stubRepo) AssignBranchManager(ctx context.Context, orgID, branchID int64, managerUserID *int64) error {
	r.fail("AssignBranchManager")
	return nil
}
func (r stubRepo) ListEmployees(ctx context.Context, orgID int64) ([]*org.EmployeeView, error) {
	r.fail("ListEmployees")
	return nil, nil
}
func (r stubRepo) ListEmployeesWithTotal(ctx context.Context, orgID int64, limit, offset int) ([]*org.EmployeeView, int, error) {
	r.fail("ListEmployeesWithTotal")
	return nil, 0, nil
}
func (r stubRepo) ListMembersHolding(ctx context.Context, orgID int64, permissionKey string) ([]*org.EmployeeView, error) {
	r.fail("ListMembersHolding")
	return nil, nil
}
func (r stubRepo) CreateInstitutionalWork(ctx context.Context, iw *org.InstitutionalWork) error {
	r.fail("CreateInstitutionalWork")
	return nil
}
func (r stubRepo) GetInstitutionalWorkByID(ctx context.Context, id int64) (*org.InstitutionalWork, error) {
	r.fail("GetInstitutionalWorkByID")
	return nil, nil
}
func (r stubRepo) UpdateInstitutionalWork(ctx context.Context, iw *org.InstitutionalWork) error {
	r.fail("UpdateInstitutionalWork")
	return nil
}
func (r stubRepo) DeleteInstitutionalWork(ctx context.Context, id int64) error {
	r.fail("DeleteInstitutionalWork")
	return nil
}
func (r stubRepo) ToggleInstitutionalWorkStatus(ctx context.Context, id int64) error {
	r.fail("ToggleInstitutionalWorkStatus")
	return nil
}
func (r stubRepo) ListInstitutionalWorks(ctx context.Context, onlyActive bool) ([]*org.InstitutionalWork, error) {
	r.fail("ListInstitutionalWorks")
	return nil, nil
}
func (r stubRepo) ListAllFlatInstitutionalWorks(ctx context.Context, onlyActive bool) ([]*org.InstitutionalWork, error) {
	r.fail("ListAllFlatInstitutionalWorks")
	return nil, nil
}
func (r stubRepo) CanConnectInstitutionalWorks(ctx context.Context, fromID, toID int64) (bool, error) {
	r.fail("CanConnectInstitutionalWorks")
	return true, nil
}
func (r stubRepo) AssignBranchInstitutionalWorks(ctx context.Context, branchID int64, workIDs []int64) error {
	r.fail("AssignBranchInstitutionalWorks")
	return nil
}
func (r stubRepo) GetBranchInstitutionalWorks(ctx context.Context, branchID int64) ([]*org.InstitutionalWork, error) {
	r.fail("GetBranchInstitutionalWorks")
	return nil, nil
}
func (r stubRepo) GetInstitutionalWorkBySlug(ctx context.Context, slug string) (*org.InstitutionalWork, error) {
	r.fail("GetInstitutionalWorkBySlug")
	return nil, nil
}
func (r stubRepo) AnyBranchHasInstitutionalWork(ctx context.Context, branchIDs []int64, workIDs []int64) (bool, error) {
	r.fail("AnyBranchHasInstitutionalWork")
	return false, nil
}
func (r stubRepo) ListBranchesWithoutInstitutionalWorks(ctx context.Context) ([]*org.BranchWithoutWorks, error) {
	r.fail("ListBranchesWithoutInstitutionalWorks")
	return nil, nil
}
func (r stubRepo) GetReachableBuyerWorksForBranch(ctx context.Context, branchID int64) ([]*org.InstitutionalWork, error) {
	r.fail("GetReachableBuyerWorksForBranch")
	return nil, nil
}
func (r stubRepo) AssignEmployeeInstitutionalWork(ctx context.Context, orgID, userID, workID int64) error {
	r.fail("AssignEmployeeInstitutionalWork")
	return nil
}
func (r stubRepo) RemoveEmployeeInstitutionalWork(ctx context.Context, orgID, userID, workID int64) error {
	r.fail("RemoveEmployeeInstitutionalWork")
	return nil
}
func (r stubRepo) ListEmployeeInstitutionalWorks(ctx context.Context, userID int64) ([]*org.EmployeeInstitutionalWork, error) {
	r.fail("ListEmployeeInstitutionalWorks")
	return nil, nil
}
func (r stubRepo) ListOrgEmployeeInstitutionalWorks(ctx context.Context, orgID int64) ([]*org.EmployeeInstitutionalWork, error) {
	r.fail("ListOrgEmployeeInstitutionalWorks")
	return nil, nil
}
func (r stubRepo) GetUserInstitutionalWorkIDs(ctx context.Context, userID int64) ([]int64, error) {
	r.fail("GetUserInstitutionalWorkIDs")
	return nil, nil
}
func (r stubRepo) GetConnectedInstitutionalWorkIDs(ctx context.Context, fromWorkIDs []int64) ([]int64, error) {
	r.fail("GetConnectedInstitutionalWorkIDs")
	return nil, nil
}
func (r stubRepo) ToggleMemberStatus(ctx context.Context, orgID, memberID int64) error {
	r.fail("ToggleMemberStatus")
	return nil
}
func (r stubRepo) GetMemberByID(ctx context.Context, orgID, memberID int64) (*org.Member, error) {
	r.fail("GetMemberByID")
	return nil, nil
}
func (r stubRepo) SavePolicies(ctx context.Context, orgID int64, policies []*org.Policy) error {
	r.fail("SavePolicies")
	return nil
}
func (r stubRepo) ListSocialMediaByOrg(ctx context.Context, orgID int64) ([]*org.SocialMedia, error) {
	r.fail("ListSocialMediaByOrg")
	return nil, nil
}
func (r stubRepo) SaveSocialMedia(ctx context.Context, orgID int64, links []*org.SocialMedia) error {
	r.fail("SaveSocialMedia")
	return nil
}

type happyRepo struct {
	org.Repository
}

func (happyRepo) GetSupplierProfile(ctx context.Context, id int64) (*org.SupplierOrgProfile, error) {
	return &org.SupplierOrgProfile{
		ID:     id,
		NameAr: "Happy Supplier",
		Type:   "supplier",
	}, nil
}
func (happyRepo) UpdateSupplierProfile(ctx context.Context, p *org.SupplierOrgProfile) error {
	return nil
}
func (happyRepo) SavePolicies(ctx context.Context, orgID int64, policies []*org.Policy) error {
	return nil
}
func (happyRepo) ListSocialMediaByOrg(ctx context.Context, orgID int64) ([]*org.SocialMedia, error) {
	return nil, nil
}
func (happyRepo) SaveSocialMedia(ctx context.Context, orgID int64, links []*org.SocialMedia) error {
	return nil
}

func (happyRepo) ToggleMemberStatus(ctx context.Context, orgID, memberID int64) error {
	return nil
}
func (happyRepo) GetMemberByID(ctx context.Context, orgID, memberID int64) (*org.Member, error) {
	return &org.Member{ID: memberID, OrganizationID: orgID}, nil
}
func (happyRepo) AssignBranchManager(ctx context.Context, orgID, branchID int64, managerUserID *int64) error {
	return nil
}
func (happyRepo) ListEmployees(ctx context.Context, orgID int64) ([]*org.EmployeeView, error) {
	return nil, nil
}
func (happyRepo) ListEmployeesWithTotal(ctx context.Context, orgID int64, limit, offset int) ([]*org.EmployeeView, int, error) {
	return nil, 0, nil
}
func (happyRepo) ListMembersHolding(ctx context.Context, orgID int64, permissionKey string) ([]*org.EmployeeView, error) {
	return nil, nil
}
func (happyRepo) CreateInstitutionalWork(ctx context.Context, iw *org.InstitutionalWork) error {
	iw.ID = 1
	return nil
}
func (happyRepo) GetInstitutionalWorkByID(ctx context.Context, id int64) (*org.InstitutionalWork, error) {
	return nil, nil
}
func (happyRepo) UpdateInstitutionalWork(ctx context.Context, iw *org.InstitutionalWork) error {
	return nil
}
func (happyRepo) DeleteInstitutionalWork(ctx context.Context, id int64) error {
	return nil
}
func (happyRepo) ToggleInstitutionalWorkStatus(ctx context.Context, id int64) error {
	return nil
}
func (happyRepo) ListInstitutionalWorks(ctx context.Context, onlyActive bool) ([]*org.InstitutionalWork, error) {
	return nil, nil
}
func (happyRepo) ListAllFlatInstitutionalWorks(ctx context.Context, onlyActive bool) ([]*org.InstitutionalWork, error) {
	return nil, nil
}
func (happyRepo) CanConnectInstitutionalWorks(ctx context.Context, fromID, toID int64) (bool, error) {
	return true, nil
}
func (happyRepo) AssignBranchInstitutionalWorks(ctx context.Context, branchID int64, workIDs []int64) error {
	return nil
}
