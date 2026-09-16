package org

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func (m *mockOrgRepo) AddReviewWithRatings(_ context.Context, r *Review, ratings []ReviewRating) error {
	r.ID = m.nextID
	m.nextID++
	r.Ratings = ratings
	m.reviews[r.OrganizationID] = append(m.reviews[r.OrganizationID], r)
	return nil
}

func (m *mockOrgRepo) GetReviewCriteria(_ context.Context, _ string) ([]*ReviewCriterion, error) {
	return nil, nil
}

func (m *mockOrgRepo) ReplyToReview(_ context.Context, reviewID, orgID int64, response string, responderID int64) error {
	return nil
}

func (m *mockOrgRepo) CreateInstitutionalWork(_ context.Context, iw *InstitutionalWork) error {
	iw.ID = m.nextID
	m.nextID++
	return nil
}

func (m *mockOrgRepo) GetInstitutionalWorkByID(_ context.Context, id int64) (*InstitutionalWork, error) {
	return nil, nil
}

func (m *mockOrgRepo) UpdateInstitutionalWork(_ context.Context, _ *InstitutionalWork) error {
	return nil
}

func (m *mockOrgRepo) DeleteInstitutionalWork(_ context.Context, _ int64) error {
	return nil
}

func (m *mockOrgRepo) ToggleInstitutionalWorkStatus(_ context.Context, _ int64) error {
	return nil
}

func (m *mockOrgRepo) ListInstitutionalWorks(_ context.Context, _ bool) ([]*InstitutionalWork, error) {
	return nil, nil
}

func (m *mockOrgRepo) ListAllFlatInstitutionalWorks(_ context.Context, _ bool) ([]*InstitutionalWork, error) {
	return nil, nil
}

func (m *mockOrgRepo) CanConnectInstitutionalWorks(_ context.Context, _, _ int64) (bool, error) {
	return true, nil
}

func (m *mockOrgRepo) AssignBranchInstitutionalWorks(_ context.Context, _ int64, _ []int64) error {
	return nil
}

func (m *mockOrgRepo) GetBranchInstitutionalWorks(_ context.Context, _ int64) ([]*InstitutionalWork, error) {
	return nil, nil
}

func (m *mockOrgRepo) GetInstitutionalWorkBySlug(_ context.Context, _ string) (*InstitutionalWork, error) {
	return nil, nil
}

func (m *mockOrgRepo) AnyBranchHasInstitutionalWork(_ context.Context, _, _ []int64) (bool, error) {
	return false, nil
}

func (m *mockOrgRepo) ListBranchesWithoutInstitutionalWorks(_ context.Context) ([]*BranchWithoutWorks, error) {
	return nil, nil
}

func (m *mockOrgRepo) GetReachableBuyerWorksForBranch(_ context.Context, _ int64) ([]*InstitutionalWork, error) {
	return nil, nil
}

func (m *mockOrgRepo) AssignEmployeeInstitutionalWork(_ context.Context, _, _, _ int64) error {
	return nil
}

func (m *mockOrgRepo) RemoveEmployeeInstitutionalWork(_ context.Context, _, _, _ int64) error {
	return nil
}

func (m *mockOrgRepo) ListEmployeeInstitutionalWorks(_ context.Context, _ int64) ([]*EmployeeInstitutionalWork, error) {
	return nil, nil
}

func (m *mockOrgRepo) ListOrgEmployeeInstitutionalWorks(_ context.Context, _ int64) ([]*EmployeeInstitutionalWork, error) {
	return nil, nil
}

func (m *mockOrgRepo) GetUserInstitutionalWorkIDs(_ context.Context, _ int64) ([]int64, error) {
	return nil, nil
}

func (m *mockOrgRepo) GetConnectedInstitutionalWorkIDs(_ context.Context, _ []int64) ([]int64, error) {
	return nil, nil
}

func (m *mockOrgRepo) ToggleMemberStatus(_ context.Context, _, _ int64) error {
	return nil
}

func (m *mockOrgRepo) GetMemberByID(_ context.Context, _, _ int64) (*Member, error) {
	return nil, nil
}

func (m *mockOrgRepo) CreateUserOrganization(_ context.Context, uo *UserOrganization) error {
	uo.ID = 1
	return nil
}

func (m *mockOrgRepo) GetUserOrganizationByID(_ context.Context, id int64) (*UserOrganization, error) {
	return &UserOrganization{ID: id, OrganizationNumber: "NUM1001", Status: UserOrgStatusApproved}, nil
}

func (m *mockOrgRepo) UpdateUserOrganization(_ context.Context, _ int64, _ string, _ UserOrganizationStatus, _ string) error {
	return nil
}

func (m *mockOrgRepo) DeleteUserOrganization(_ context.Context, _ int64) error {
	return nil
}

func (m *mockOrgRepo) ListUserOrganizationsByUser(_ context.Context, _ int64) ([]*UserOrganization, error) {
	return []*UserOrganization{}, nil
}

func (m *mockOrgRepo) ListUserOrganizationsByVendor(_ context.Context, _ int64, _ string) ([]*UserOrganization, error) {
	return []*UserOrganization{}, nil
}

func (m *mockOrgRepo) ListUserOrganizationsByVendorWithTotal(_ context.Context, _ int64, _ string, _, _ int) ([]*UserOrganization, int, error) {
	return []*UserOrganization{}, 0, nil
}

func (m *mockOrgRepo) ListAllUserOrganizations(_ context.Context, _ string) ([]*UserOrganization, error) {
	return []*UserOrganization{}, nil
}

func (m *mockOrgRepo) ListAllUserOrganizationsWithTotal(_ context.Context, _ string, _, _ int) ([]*UserOrganization, int, error) {
	return []*UserOrganization{}, 0, nil
}

func TestOrgLifecycleAndBranches(t *testing.T) {

	ctx := context.Background()
	repo := newMockOrgRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(repo, logger)

	// 1. Register Organization
	minOrder, _ := money.Parse("500.00")
	regInput := RegisterOrgInput{
		LegalName:          "Al-Amal Medical Distribution LLC",
		TradeName:          i18n.New("مخزن الأمل", "Al-Amal Warehouse"),
		TaxNumber:          "TX-883322",
		CommercialRegister: "CR-992211",
		Type:               TypeVendor,
		CreditLimit:        minOrder,
		PaymentTermsDays:   30,
	}

	createdOrg, err := svc.RegisterOrganization(ctx, regInput)
	if err != nil {
		t.Fatalf("RegisterOrganization failed: %v", err)
	}
	if createdOrg.Status != StatusPending {
		t.Errorf("got status %s, want pending", createdOrg.Status)
	}

	// 2. Admin Approve Org
	if err := svc.ApproveOrganization(ctx, createdOrg.ID); err != nil {
		t.Fatalf("ApproveOrganization failed: %v", err)
	}
	gotOrg, err := svc.GetOrganization(ctx, createdOrg.ID)
	if err != nil {
		t.Fatalf("GetOrganization failed: %v", err)
	}
	if gotOrg.Status != StatusApproved {
		t.Errorf("got status %s, want approved", gotOrg.Status)
	}

	// Reject & Suspend
	_ = svc.RejectOrganization(ctx, createdOrg.ID)
	_ = svc.SuspendOrganization(ctx, createdOrg.ID)

	// 3. Branches
	branch := &Branch{
		OrganizationID: createdOrg.ID,
		Name:           i18n.New("الفرع الرئيسي", "Main Branch"),
		Code:           "BR-001",
		Address:        "123 Nile Corniche",
		IsMain:         true,
	}
	if err := svc.CreateBranch(ctx, branch); err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	branches, err := svc.ListBranches(ctx, createdOrg.ID)
	if err != nil || len(branches) != 1 {
		t.Fatalf("ListBranches failed: %v", err)
	}
	if err := svc.UpdateBranch(ctx, branch); err != nil {
		t.Fatalf("UpdateBranch failed: %v", err)
	}
	if err := svc.DeleteBranch(ctx, branch.ID, createdOrg.ID); err != nil {
		t.Fatalf("DeleteBranch failed: %v", err)
	}

	// 4. Members
	mem, err := svc.AddMember(ctx, createdOrg.ID, 100, 2)
	if err != nil {
		t.Fatalf("AddMember failed: %v", err)
	}
	if mem.UserID != 100 {
		t.Errorf("got user id %d, want 100", mem.UserID)
	}
	members, err := svc.ListMembers(ctx, createdOrg.ID)
	if err != nil || len(members) != 1 {
		t.Fatalf("ListMembers failed: %v", err)
	}
	if err := svc.UpdateMemberRole(ctx, createdOrg.ID, 100, "manager"); err != nil {
		t.Fatalf("UpdateMemberRole failed: %v", err)
	}
	if err := svc.RemoveMember(ctx, createdOrg.ID, 100); err != nil {
		t.Fatalf("RemoveMember failed: %v", err)
	}

	// 5. Follow & Unfollow
	following, err := svc.ToggleFollow(ctx, createdOrg.ID, 200)
	if err != nil || !following {
		t.Fatalf("ToggleFollow failed: %v", err)
	}

	// 6. Reviews
	rev, err := svc.AddReview(ctx, createdOrg.ID, 200, 5, "Fast delivery and genuine products")
	if err != nil {
		t.Fatalf("AddReview failed: %v", err)
	}
	if rev.Rating != 5 {
		t.Errorf("got rating %d, want 5", rev.Rating)
	}
	reviews, err := svc.ListReviews(ctx, createdOrg.ID, 10, 0)
	if err != nil || len(reviews) != 1 {
		t.Fatalf("ListReviews failed: %v", err)
	}

	// 7. Update & Delete Org
	createdOrg.TaxNumber = "TX-999999"
	if err := svc.UpdateOrganization(ctx, createdOrg); err != nil {
		t.Fatalf("UpdateOrganization failed: %v", err)
	}
	if err := svc.DeleteOrganization(ctx, createdOrg.ID); err != nil {
		t.Fatalf("DeleteOrganization failed: %v", err)
	}
}

func TestUserOrganizationFlow(t *testing.T) {
	ctx := context.Background()
	repo := newMockOrgRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(repo, logger)

	// 1. Customer creates a link to vendor organization
	custOrgID := int64(10)
	uo, err := svc.CreateUserOrgLink(ctx, 42, &custOrgID, 100, "NUM10001", UserOrgStatusPending)
	if err != nil {
		t.Fatalf("CreateUserOrgLink failed: %v", err)
	}
	if uo.OrganizationNumber != "NUM10001" {
		t.Errorf("got org number %s, want NUM10001", uo.OrganizationNumber)
	}
	if uo.Status != UserOrgStatusPending {
		t.Errorf("got status %s, want pending", uo.Status)
	}

	// 2. Vendor approves the link
	if err := svc.ApproveUserOrgLink(ctx, uo.ID); err != nil {
		t.Fatalf("ApproveUserOrgLink failed: %v", err)
	}

	// 3. Vendor rejects / edits
	if err := svc.RejectUserOrgLink(ctx, uo.ID, "Invalid customer code"); err != nil {
		t.Fatalf("RejectUserOrgLink failed: %v", err)
	}

	if err := svc.UpdateUserOrgLink(ctx, uo.ID, "NUM10002", "Updated code"); err != nil {
		t.Fatalf("UpdateUserOrgLink failed: %v", err)
	}

	// 4. Delete link
	if err := svc.DeleteUserOrgLink(ctx, uo.ID); err != nil {
		t.Fatalf("DeleteUserOrgLink failed: %v", err)
	}
}

func (m *mockOrgRepo) CountOrganizations(_ context.Context, _ *OrganizationType, _ *OrganizationStatus) (int, error) {
	return len(m.orgs), nil
}

func (m *mockOrgRepo) GetOrganizationsByIDs(_ context.Context, ids []int64) ([]*Organization, error) {
	var out []*Organization
	for _, id := range ids {
		if o, ok := m.orgs[id]; ok {
			out = append(out, o)
		}
	}
	return out, nil
}

func (m *mockOrgRepo) GetBranchesByIDs(_ context.Context, _ []int64) ([]*Branch, error) {
	return nil, nil
}
