package org

import (
	"context"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

type mockOrgRepo struct {
	Repository
	orgs           map[int64]*Organization
	branches       map[int64][]*Branch
	members        map[int64][]*Member
	reviews        map[int64][]*Review
	policies       map[int64][]*Policy
	followers      map[int64]map[int64]bool
	deliveryBands  map[int64][]*DeliveryBand
	roles          []*Role
	profiles       map[int64]map[ProfileSection]ProfileFields
	changeRequests []*ProfileChangeRequest
	nextID         int64
}

func newMockOrgRepo() *mockOrgRepo {
	return &mockOrgRepo{
		orgs:          map[int64]*Organization{},
		branches:      map[int64][]*Branch{},
		members:       map[int64][]*Member{},
		reviews:       map[int64][]*Review{},
		policies:      map[int64][]*Policy{},
		followers:     map[int64]map[int64]bool{},
		deliveryBands: map[int64][]*DeliveryBand{},
		nextID:        1,
	}
}

func (m *mockOrgRepo) CreateOrganization(_ context.Context, o *Organization) error {
	o.ID = m.nextID
	m.nextID++
	m.orgs[o.ID] = o
	return nil
}

func (m *mockOrgRepo) GetOrganizationByID(_ context.Context, id int64) (*Organization, error) {
	o, ok := m.orgs[id]
	if !ok {
		return nil, nil
	}
	return o, nil
}

func (m *mockOrgRepo) GetSupplierProfile(_ context.Context, id int64) (*SupplierOrgProfile, error) {
	o, ok := m.orgs[id]
	if !ok {
		return nil, nil
	}
	return &SupplierOrgProfile{
		ID:                 o.ID,
		NameAr:             o.LegalName,
		NameEn:             o.LegalName,
		Type:               string(o.Type),
		MinOrderPrice:      o.MinOrderPrice,
		MaxOrderPrice:      o.MaxOrderPrice,
		OrganizationNumber: o.OrganizationNumber,
		TaxNumber:          o.TaxNumber,
		Status:             string(o.Status),
	}, nil
}

func (m *mockOrgRepo) UpdateSupplierProfile(_ context.Context, p *SupplierOrgProfile) error {
	o, ok := m.orgs[p.ID]
	if ok {
		o.LegalName = p.NameAr
		o.Type = OrganizationType(p.Type)
		o.MinOrderPrice = p.MinOrderPrice
		o.MaxOrderPrice = p.MaxOrderPrice
		o.OrganizationNumber = p.OrganizationNumber
		o.TaxNumber = p.TaxNumber
	}
	return nil
}

func (m *mockOrgRepo) UpdateOrganizationStatus(_ context.Context, id int64, status OrganizationStatus) error {
	o, ok := m.orgs[id]
	if ok {
		o.Status = status
	}
	return nil
}

func (m *mockOrgRepo) UpdateOrganizationAICredentials(_ context.Context, id int64, aiUserID, aiVirtualKey string) error {
	o, ok := m.orgs[id]
	if ok {
		o.AIUserID = aiUserID
		o.AIVirtualKey = aiVirtualKey
	}
	return nil
}

func (m *mockOrgRepo) ReviewOrganization(_ context.Context, id int64, status OrganizationStatus, notes, rejectionReason string, adminID int64) error {
	o, ok := m.orgs[id]
	if ok {
		o.Status = status
		o.VerificationNotes = notes
		o.RejectionReason = rejectionReason
		o.ApprovedBy = &adminID
	}
	return nil
}

func (m *mockOrgRepo) UpdateOrganization(_ context.Context, o *Organization) error {
	m.orgs[o.ID] = o
	return nil
}

func (m *mockOrgRepo) DeleteOrganization(_ context.Context, id int64) error {
	if o, ok := m.orgs[id]; ok {
		o.Status = StatusSuspended
	}
	return nil
}

func (m *mockOrgRepo) UpdateBranch(_ context.Context, b *Branch) error {
	return nil
}

func (m *mockOrgRepo) DeleteBranch(_ context.Context, id, orgID int64) error {
	return nil
}

func (m *mockOrgRepo) ListOrganizations(_ context.Context, orgType *OrganizationType, status *OrganizationStatus, limit, offset int) ([]*Organization, error) {
	var list []*Organization
	for _, o := range m.orgs {
		if (orgType == nil || o.Type == *orgType) && (status == nil || o.Status == *status) {
			list = append(list, o)
		}
	}
	return list, nil
}

func (m *mockOrgRepo) ListOrganizationsWithTotal(ctx context.Context, _ string, orgType *OrganizationType, status *OrganizationStatus, limit, offset int) ([]*Organization, int, error) {
	list, err := m.ListOrganizations(ctx, orgType, status, limit, offset)
	return list, len(list), err
}

func (m *mockOrgRepo) AdminOrgStats(_ context.Context) (AdminOrgStatsResult, error) {
	return AdminOrgStatsResult{
		TotalOrgs:       len(m.orgs),
		TotalPharmacies: len(m.orgs),
		TotalVendors:    0,
		PendingCount:    0,
		ApprovedCount:   len(m.orgs),
	}, nil
}

func (m *mockOrgRepo) CountBranchesByOrg(_ context.Context) (map[int64]int, error) {
	counts := make(map[int64]int)
	for orgID, bs := range m.branches {
		counts[orgID] = len(bs)
	}
	return counts, nil
}

func (m *mockOrgRepo) CreateBranch(_ context.Context, b *Branch) error {
	b.ID = m.nextID
	m.nextID++
	m.branches[b.OrganizationID] = append(m.branches[b.OrganizationID], b)
	return nil
}

func (m *mockOrgRepo) GetBranchByID(_ context.Context, id int64) (*Branch, error) {
	for _, list := range m.branches {
		for _, b := range list {
			if b.ID == id {
				return b, nil
			}
		}
	}
	return nil, nil
}

func (m *mockOrgRepo) ListBranchesByOrg(_ context.Context, orgID int64) ([]*Branch, error) {
	return m.branches[orgID], nil
}

func (m *mockOrgRepo) ListBranchesWithTotal(_ context.Context, _ BranchFilter, _, _ int) ([]*Branch, int, error) {
	return nil, 0, nil
}

func (m *mockOrgRepo) AdminBranchStats(_ context.Context) (AdminBranchStatsResult, error) {
	return AdminBranchStatsResult{}, nil
}

func (m *mockOrgRepo) UnsetMainBranches(_ context.Context, orgID int64) error {
	for _, b := range m.branches[orgID] {
		b.IsMain = false
	}
	return nil
}

func (m *mockOrgRepo) AssignBranchManager(_ context.Context, orgID, branchID int64, managerUserID *int64) error {
	for _, b := range m.branches[orgID] {
		if b.ID == branchID {
			b.ManagerID = managerUserID
		}
	}
	return nil
}

func (m *mockOrgRepo) ListEmployees(_ context.Context, orgID int64) ([]*EmployeeView, error) {
	var list []*EmployeeView
	for _, mem := range m.members[orgID] {
		list = append(list, &EmployeeView{
			Member:    mem,
			UserName:  "Test User",
			UserEmail: "user@example.com",
		})
	}
	return list, nil
}

func (m *mockOrgRepo) ListEmployeesWithTotal(_ context.Context, orgID int64, _, _ int) ([]*EmployeeView, int, error) {
	var list []*EmployeeView
	for _, mem := range m.members[orgID] {
		list = append(list, &EmployeeView{
			Member:    mem,
			UserName:  "Test User",
			UserEmail: "user@example.com",
		})
	}
	return list, len(list), nil
}

func (m *mockOrgRepo) ListMembersHolding(_ context.Context, orgID int64, _ string) ([]*EmployeeView, error) {
	var list []*EmployeeView
	for _, mem := range m.members[orgID] {
		list = append(list, &EmployeeView{Member: mem, UserName: "Test Courier"})
	}
	return list, nil
}

func (m *mockOrgRepo) AddMember(_ context.Context, mem *Member) error {

	mem.ID = m.nextID
	m.nextID++
	m.members[mem.OrganizationID] = append(m.members[mem.OrganizationID], mem)
	return nil
}

func (m *mockOrgRepo) UpdateMemberRole(_ context.Context, orgID, userID int64, role string) error {
	return nil
}

func (m *mockOrgRepo) RemoveMember(_ context.Context, orgID, userID int64) error {
	var remaining []*Member
	for _, mem := range m.members[orgID] {
		if mem.UserID != userID {
			remaining = append(remaining, mem)
		}
	}
	m.members[orgID] = remaining
	return nil
}

func (m *mockOrgRepo) ListMembersByOrg(_ context.Context, orgID int64) ([]*Member, error) {
	return m.members[orgID], nil
}

func (m *mockOrgRepo) AddReview(_ context.Context, r *Review) error {
	r.ID = m.nextID
	m.nextID++
	m.reviews[r.OrganizationID] = append(m.reviews[r.OrganizationID], r)
	return nil
}

func (m *mockOrgRepo) ListReviewsByOrg(_ context.Context, orgID int64, limit, offset int) ([]*Review, error) {
	return m.reviews[orgID], nil
}

func (m *mockOrgRepo) ToggleFollower(_ context.Context, orgID, userID int64) (bool, error) {
	if m.followers[orgID] == nil {
		m.followers[orgID] = map[int64]bool{}
	}
	current := m.followers[orgID][userID]
	m.followers[orgID][userID] = !current
	return !current, nil
}

func (m *mockOrgRepo) IsFollowing(_ context.Context, orgID, userID int64) (bool, error) {
	if m.followers[orgID] == nil {
		return false, nil
	}
	return m.followers[orgID][userID], nil
}

func (m *mockOrgRepo) ListFollowedOrgs(_ context.Context, userID int64) ([]*Organization, error) {
	var list []*Organization
	for orgID, users := range m.followers {
		if users[userID] {
			if o := m.orgs[orgID]; o != nil {
				list = append(list, o)
			}
		}
	}
	return list, nil
}

func (m *mockOrgRepo) CreatePolicy(_ context.Context, p *Policy) error {
	p.ID = m.nextID
	m.nextID++
	m.policies[p.OrganizationID] = append(m.policies[p.OrganizationID], p)
	return nil
}

func (m *mockOrgRepo) ListPoliciesByOrg(_ context.Context, orgID int64) ([]*Policy, error) {
	return m.policies[orgID], nil
}

func (m *mockOrgRepo) SavePolicies(_ context.Context, orgID int64, policies []*Policy) error {
	m.policies[orgID] = policies
	return nil
}

func (m *mockOrgRepo) ListSocialMediaByOrg(_ context.Context, orgID int64) ([]*SocialMedia, error) {
	return nil, nil
}

func (m *mockOrgRepo) SaveSocialMedia(_ context.Context, orgID int64, links []*SocialMedia) error {
	return nil
}

func (m *mockOrgRepo) CreateRole(_ context.Context, role *Role) error {
	role.ID = 1
	return nil
}

func (m *mockOrgRepo) GetRole(_ context.Context, orgID, roleID int64) (*Role, error) {
	for _, r := range m.roles {
		if r.ID == roleID && r.OrganizationID == orgID {
			return r, nil
		}
	}
	return nil, apperr.NotFound("role")
}

func (m *mockOrgRepo) UpdateRole(_ context.Context, orgID int64, role *Role) error {
	for i, r := range m.roles {
		if r.ID == role.ID && r.OrganizationID == orgID {
			m.roles[i] = role
			return nil
		}
	}
	return apperr.NotFound("role")
}

func (m *mockOrgRepo) DeleteRole(_ context.Context, orgID, roleID int64) error {
	for i, r := range m.roles {
		if r.ID == roleID && r.OrganizationID == orgID {
			m.roles = append(m.roles[:i], m.roles[i+1:]...)
			return nil
		}
	}
	return apperr.NotFound("role")
}

func (m *mockOrgRepo) ListRoles(_ context.Context, orgID int64) ([]*Role, error) {
	var out []*Role
	for _, r := range m.roles {
		if r.OrganizationID == orgID {
			out = append(out, r)
		}
	}
	return out, nil
}

func (m *mockOrgRepo) CountRoleMembers(_ context.Context, _ int64) (map[int64]int, error) {
	return map[int64]int{}, nil
}

func (m *mockOrgRepo) AssignMemberRole(_ context.Context, _, _, _ int64) error { return nil }

func (m *mockOrgRepo) GetDeliveryBands(_ context.Context, orgID int64) ([]*DeliveryBand, error) {
	if m.deliveryBands != nil {
		return m.deliveryBands[orgID], nil
	}
	return nil, nil
}

func (m *mockOrgRepo) SaveDeliveryBands(_ context.Context, orgID int64, bands []*DeliveryBand) error {
	if m.deliveryBands == nil {
		m.deliveryBands = make(map[int64][]*DeliveryBand)
	}
	m.deliveryBands[orgID] = bands
	return nil
}

func (m *mockOrgRepo) CreateOrgDeletionRequest(_ context.Context, req *OrganizationDeletionRequest) error {
	req.ID = m.nextID
	m.nextID++
	req.Status = OrgDeletionStatusPending
	return nil
}

func (m *mockOrgRepo) GetPendingOrgDeletionRequest(_ context.Context, _ int64) (*OrganizationDeletionRequest, error) {
	return nil, nil
}

func (m *mockOrgRepo) GetOrgDeletionRequest(_ context.Context, _ int64) (*OrganizationDeletionRequest, error) {
	return nil, nil
}

func (m *mockOrgRepo) CancelOrgDeletionRequest(_ context.Context, _, _ int64) error {
	return nil
}

func (m *mockOrgRepo) ListOrgDeletionRequests(_ context.Context, _ string, _, _ int) ([]*OrganizationDeletionRequest, int, error) {
	return nil, 0, nil
}

func (m *mockOrgRepo) ReviewOrgDeletionRequest(_ context.Context, _, _ int64, _ bool, _ string) (*OrganizationDeletionRequest, error) {
	return nil, nil
}

func (m *mockOrgRepo) SetExtraDevices(_ context.Context, orgID int64, extraDevices int, expiresAt *time.Time) error {
	o, ok := m.orgs[orgID]
	if !ok {
		return apperr.NotFound("organization")
	}
	o.ExtraDevices = extraDevices
	o.ExtraDevicesExpiresAt = expiresAt
	return nil
}

func TestOrganization_ActiveExtraDevices(t *testing.T) {
	// 1. Zero extra devices
	o1 := &Organization{ExtraDevices: 0}
	if got := o1.ActiveExtraDevices(); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}

	// 2. Active extra devices with future expiry
	future := time.Now().Add(24 * time.Hour)
	o2 := &Organization{ExtraDevices: 5, ExtraDevicesExpiresAt: &future}
	if got := o2.ActiveExtraDevices(); got != 5 {
		t.Fatalf("expected 5, got %d", got)
	}

	// 3. Expired extra devices
	past := time.Now().Add(-1 * time.Hour)
	o3 := &Organization{ExtraDevices: 5, ExtraDevicesExpiresAt: &past}
	if got := o3.ActiveExtraDevices(); got != 0 {
		t.Fatalf("expected 0 for expired devices, got %d", got)
	}

	// 4. Active extra devices without explicit expiry
	o4 := &Organization{ExtraDevices: 3, ExtraDevicesExpiresAt: nil}
	if got := o4.ActiveExtraDevices(); got != 3 {
		t.Fatalf("expected 3, got %d", got)
	}

	// 5. Service SetExtraDevices sets and clears
	repo := newMockOrgRepo()
	svc := NewService(repo, nil)
	ctx := context.Background()
	_ = repo.CreateOrganization(ctx, &Organization{LegalName: "Test Pharmacy"})
	orgID := int64(1)

	if err := svc.SetExtraDevices(ctx, orgID, 4, &future); err != nil {
		t.Fatalf("SetExtraDevices: %v", err)
	}
	fetched, _ := svc.GetOrganization(ctx, orgID)
	if fetched.ActiveExtraDevices() != 4 {
		t.Fatalf("expected 4 active extra devices, got %d", fetched.ActiveExtraDevices())
	}

	// Clear extra devices
	if err := svc.SetExtraDevices(ctx, orgID, 0, nil); err != nil {
		t.Fatalf("SetExtraDevices reset: %v", err)
	}
	fetched2, _ := svc.GetOrganization(ctx, orgID)
	if fetched2.ActiveExtraDevices() != 0 {
		t.Fatalf("expected 0 active extra devices after reset, got %d", fetched2.ActiveExtraDevices())
	}
}
