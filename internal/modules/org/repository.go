package org

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// Repository defines the storage contract for organization management.
type Repository interface {
	CreateOrganization(ctx context.Context, o *Organization) error
	GetOrganizationByID(ctx context.Context, id int64) (*Organization, error)
	GetSupplierProfile(ctx context.Context, id int64) (*SupplierOrgProfile, error)
	UpdateSupplierProfile(ctx context.Context, p *SupplierOrgProfile) error
	UpdateOrganizationStatus(ctx context.Context, id int64, status OrganizationStatus) error
	UpdateOrganizationAICredentials(ctx context.Context, id int64, aiUserID, aiVirtualKey string) error
	SetExtraDevices(ctx context.Context, orgID int64, extraDevices int, expiresAt *time.Time) error
	ReviewOrganization(ctx context.Context, id int64, status OrganizationStatus, notes, rejectionReason string, adminID int64) error
	UpdateOrganization(ctx context.Context, o *Organization) error
	DeleteOrganization(ctx context.Context, id int64) error
	ListOrganizations(ctx context.Context, orgType *OrganizationType, status *OrganizationStatus, limit, offset int) ([]*Organization, error)
	ListOrganizationsWithTotal(ctx context.Context, search string, orgType *OrganizationType, status *OrganizationStatus, limit, offset int) ([]*Organization, int, error)
	AdminOrgStats(ctx context.Context) (AdminOrgStatsResult, error)
	GetOrganizationsByIDs(ctx context.Context, ids []int64) ([]*Organization, error)
	CountOrganizations(ctx context.Context, orgType *OrganizationType, status *OrganizationStatus) (int, error)

	CreateBranch(ctx context.Context, b *Branch) error
	GetBranchByID(ctx context.Context, id int64) (*Branch, error)
	UpdateBranch(ctx context.Context, b *Branch) error
	DeleteBranch(ctx context.Context, id, orgID int64) error
	ListBranchesByOrg(ctx context.Context, orgID int64) ([]*Branch, error)
	ListBranchesWithTotal(ctx context.Context, filter BranchFilter, limit, offset int) ([]*Branch, int, error)
	AdminBranchStats(ctx context.Context) (AdminBranchStatsResult, error)
	CountBranchesByOrg(ctx context.Context) (map[int64]int, error)
	GetBranchesByIDs(ctx context.Context, ids []int64) ([]*Branch, error)
	UnsetMainBranches(ctx context.Context, orgID int64) error
	AssignBranchManager(ctx context.Context, orgID, branchID int64, managerUserID *int64) error

	AddMember(ctx context.Context, m *Member) error

	// Profile sections and the change requests that gate the identity one.
	ReadProfileSection(ctx context.Context, orgID int64, section ProfileSection) (ProfileFields, error)
	ApplyProfileSection(ctx context.Context, orgID int64, section ProfileSection, fields ProfileFields) error
	ApplyApprovedProfileChange(ctx context.Context, tx pgx.Tx, req *ProfileChangeRequest) error
	CreateProfileChangeRequest(ctx context.Context, req *ProfileChangeRequest) error
	PendingProfileChanges(ctx context.Context, orgID int64) (map[ProfileSection]*ProfileChangeRequest, error)
	GetProfileChangeRequest(ctx context.Context, id int64) (*ProfileChangeRequest, error)
	ListProfileChangeRequests(ctx context.Context, status string, limit, offset int) ([]*ProfileChangeRequest, int, error)
	ListProfileChangeRequestsWithFilter(ctx context.Context, filter ProfileChangeFilter, limit, offset int) ([]*ProfileChangeRequest, int, error)
	ProfileChangeRequestCounts(ctx context.Context) (ProfileChangeCounts, error)
	DecideProfileChangeRequest(ctx context.Context, id, reviewerID int64, approve bool, notes string, apply func(context.Context, pgx.Tx, *ProfileChangeRequest) error) (*ProfileChangeRequest, error)
	WithdrawProfileChangeRequest(ctx context.Context, orgID, id int64) error

	// Organization deletion requests
	CreateOrgDeletionRequest(ctx context.Context, req *OrganizationDeletionRequest) error
	GetPendingOrgDeletionRequest(ctx context.Context, orgID int64) (*OrganizationDeletionRequest, error)
	GetOrgDeletionRequest(ctx context.Context, id int64) (*OrganizationDeletionRequest, error)
	CancelOrgDeletionRequest(ctx context.Context, orgID, requestID int64) error
	ListOrgDeletionRequests(ctx context.Context, status string, limit, offset int) ([]*OrganizationDeletionRequest, int, error)
	ReviewOrgDeletionRequest(ctx context.Context, requestID, reviewerID int64, approve bool, adminNotes string) (*OrganizationDeletionRequest, error)
	GetMember(ctx context.Context, orgID, memberID int64) (*Member, error)
	UpdateMember(ctx context.Context, orgID, memberID int64, patch MemberPatch) error
	CountMembersByBranch(ctx context.Context, orgID int64) (map[int64]int, error)
	MemberOrganizations(ctx context.Context, userID int64) ([]int64, error)
	UpdateMemberRole(ctx context.Context, orgID, userID int64, role string) error
	ToggleMemberStatus(ctx context.Context, orgID, memberID int64) error
	GetMemberByID(ctx context.Context, orgID, memberID int64) (*Member, error)
	ListMembersByOrg(ctx context.Context, orgID int64) ([]*Member, error)
	ListEmployees(ctx context.Context, orgID int64) ([]*EmployeeView, error)
	ListEmployeesWithTotal(ctx context.Context, orgID int64, limit, offset int) ([]*EmployeeView, int, error)
	// ListMembersHolding returns the active members whose company role grants
	// one permission — "who in this company may do X".
	//
	// It answers by permission rather than by role key on purpose. A supplier
	// that renames مندوب توصيل, or builds "مندوب المنطقة الشرقية" from scratch
	// and grants it the delivery keys, has made a delivery representative; a
	// query for role_key = 'org_courier' would not find them, and the
	// assignment list would silently omit half the team.
	ListMembersHolding(ctx context.Context, orgID int64, permissionKey string) ([]*EmployeeView, error)
	RemoveMember(ctx context.Context, orgID, userID int64) error

	// Company roles. Every one of these takes the caller's organization id
	// and the implementation puts it in the WHERE clause — a role id alone is
	// never enough to reach a role, because a role id alone belongs to
	// whichever company happens to own it.
	CreateRole(ctx context.Context, role *Role) error
	GetRole(ctx context.Context, orgID, roleID int64) (*Role, error)
	UpdateRole(ctx context.Context, orgID int64, role *Role) error
	DeleteRole(ctx context.Context, orgID, roleID int64) error
	ListRoles(ctx context.Context, orgID int64) ([]*Role, error)
	CountRoleMembers(ctx context.Context, orgID int64) (map[int64]int, error)
	AssignMemberRole(ctx context.Context, orgID, memberID, roleID int64) error

	// Delivery Bands
	GetDeliveryBands(ctx context.Context, orgID int64) ([]*DeliveryBand, error)
	SaveDeliveryBands(ctx context.Context, orgID int64, bands []*DeliveryBand) error

	// Multi-criteria Reviews
	AddReview(ctx context.Context, r *Review) error
	AddReviewWithRatings(ctx context.Context, r *Review, ratings []ReviewRating) error
	ListReviewsByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*Review, error)
	ListReviewsForVendor(ctx context.Context, vendorOrgID int64, limit, offset int) ([]*Review, error)
	GetReviewByOrderAndVendor(ctx context.Context, orderID, vendorOrgID int64) (*Review, error)
	ListReviewsForOrder(ctx context.Context, orderID int64) ([]*Review, error)
	HasDeliveredOrderFromVendor(ctx context.Context, customerOrgID, vendorOrgID int64) (bool, error)
	GetReviewCriteria(ctx context.Context, contextType string) ([]*ReviewCriterion, error)
	ReplyToReview(ctx context.Context, reviewID, orgID int64, response string, responderID int64) error
	ListAdminReviewsWithTotal(ctx context.Context, filter AdminReviewFilter) ([]*Review, int, error)
	GetAdminReviewStats(ctx context.Context) (*AdminReviewStats, error)
	UpdateReviewStatus(ctx context.Context, reviewID int64, isApproved bool) error
	SoftDeleteReview(ctx context.Context, reviewID int64) error

	ToggleFollower(ctx context.Context, orgID, userID int64) (bool, error)
	IsFollowing(ctx context.Context, orgID, userID int64) (bool, error)
	ListFollowedOrgs(ctx context.Context, userID int64) ([]*Organization, error)

	CreatePolicy(ctx context.Context, p *Policy) error
	ListPoliciesByOrg(ctx context.Context, orgID int64) ([]*Policy, error)
	SavePolicies(ctx context.Context, orgID int64, policies []*Policy) error

	ListSocialMediaByOrg(ctx context.Context, orgID int64) ([]*SocialMedia, error)
	SaveSocialMedia(ctx context.Context, orgID int64, links []*SocialMedia) error

	// Institutional Works (الهيكل المؤسسي وأنواعه)
	CreateInstitutionalWork(ctx context.Context, iw *InstitutionalWork) error
	GetInstitutionalWorkByID(ctx context.Context, id int64) (*InstitutionalWork, error)
	GetInstitutionalWorkBySlug(ctx context.Context, slug string) (*InstitutionalWork, error)
	UpdateInstitutionalWork(ctx context.Context, iw *InstitutionalWork) error
	DeleteInstitutionalWork(ctx context.Context, id int64) error
	ToggleInstitutionalWorkStatus(ctx context.Context, id int64) error
	ListInstitutionalWorks(ctx context.Context, onlyActive bool) ([]*InstitutionalWork, error)
	ListAllFlatInstitutionalWorks(ctx context.Context, onlyActive bool) ([]*InstitutionalWork, error)
	CanConnectInstitutionalWorks(ctx context.Context, fromID, toID int64) (bool, error)
	AssignBranchInstitutionalWorks(ctx context.Context, branchID int64, workIDs []int64) error
	GetBranchInstitutionalWorks(ctx context.Context, branchID int64) ([]*InstitutionalWork, error)
	AnyBranchHasInstitutionalWork(ctx context.Context, branchIDs []int64, workIDs []int64) (bool, error)
	ListBranchesWithoutInstitutionalWorks(ctx context.Context) ([]*BranchWithoutWorks, error)
	GetReachableBuyerWorksForBranch(ctx context.Context, branchID int64) ([]*InstitutionalWork, error)

	// Employee Institutional Works (مجموعات العمل للمستخدمين والموظفين)
	AssignEmployeeInstitutionalWork(ctx context.Context, orgID, userID, workID int64) error
	RemoveEmployeeInstitutionalWork(ctx context.Context, orgID, userID, workID int64) error
	ListEmployeeInstitutionalWorks(ctx context.Context, userID int64) ([]*EmployeeInstitutionalWork, error)
	ListOrgEmployeeInstitutionalWorks(ctx context.Context, orgID int64) ([]*EmployeeInstitutionalWork, error)
	GetUserInstitutionalWorkIDs(ctx context.Context, userID int64) ([]int64, error)
	GetConnectedInstitutionalWorkIDs(ctx context.Context, fromWorkIDs []int64) ([]int64, error)

	// User Organizations (ربط مستخدمي الصيدليات بالمنظمات والموردين)
	CreateUserOrganization(ctx context.Context, uo *UserOrganization) error
	GetUserOrganizationByID(ctx context.Context, id int64) (*UserOrganization, error)
	UpdateUserOrganization(ctx context.Context, id int64, orgNumber string, status UserOrganizationStatus, notes string) error
	DeleteUserOrganization(ctx context.Context, id int64) error
	ListUserOrganizationsByUser(ctx context.Context, userID int64) ([]*UserOrganization, error)
	ListUserOrganizationsByVendor(ctx context.Context, vendorOrgID int64, statusFilter string) ([]*UserOrganization, error)
	ListUserOrganizationsByVendorWithTotal(ctx context.Context, vendorOrgID int64, statusFilter string, limit, offset int) ([]*UserOrganization, int, error)
	ListAllUserOrganizations(ctx context.Context, statusFilter string) ([]*UserOrganization, error)
	ListAllUserOrganizationsWithTotal(ctx context.Context, statusFilter string, limit, offset int) ([]*UserOrganization, int, error)
}
