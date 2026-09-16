package ui_test

import (
	"context"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

type mockIdentityRepoTeamTest struct {
	identity.Repository
	users  map[string]*identity.User
	byID   map[int64]*identity.User
	nextID int64
}

func newMockIdentityRepoTeamTest() *mockIdentityRepoTeamTest {
	return &mockIdentityRepoTeamTest{
		users:  make(map[string]*identity.User),
		byID:   make(map[int64]*identity.User),
		nextID: 100,
	}
}

func (m *mockIdentityRepoTeamTest) CreateUser(ctx context.Context, u *identity.User) error {
	m.nextID++
	u.ID = m.nextID
	m.users[strings.ToLower(u.Email)] = u
	m.byID[u.ID] = u
	return nil
}

func (m *mockIdentityRepoTeamTest) CreateSecurity(ctx context.Context, s *identity.UserSecurity) error {
	return nil
}

func (m *mockIdentityRepoTeamTest) UpsertSecurity(ctx context.Context, s *identity.UserSecurity) error {
	return nil
}

func (m *mockIdentityRepoTeamTest) GetPermissionsForUser(ctx context.Context, userID int64, orgID int64) ([]string, error) {
	return []string{"user"}, nil
}

func (m *mockIdentityRepoTeamTest) CreateSession(ctx context.Context, s *identity.Session) error {
	return nil
}

func (m *mockIdentityRepoTeamTest) GetUserByEmail(ctx context.Context, email string) (*identity.User, error) {
	u, ok := m.users[strings.ToLower(email)]
	if !ok {
		return nil, nil
	}
	return u, nil
}

func (m *mockIdentityRepoTeamTest) GetUserByID(ctx context.Context, id int64) (*identity.User, error) {
	u, ok := m.byID[id]
	if !ok {
		return nil, nil
	}
	return u, nil
}

func (m *mockIdentityRepoTeamTest) UpdateUser(ctx context.Context, u *identity.User) error {
	m.users[strings.ToLower(u.Email)] = u
	m.byID[u.ID] = u
	return nil
}

type mockOrgRepoTeamTest struct {
	org.Repository
	members  map[int64]*org.Member
	branches []*org.Branch
	nextMID  int64
}

func newMockOrgRepoTeamTest() *mockOrgRepoTeamTest {
	return &mockOrgRepoTeamTest{
		members: make(map[int64]*org.Member),
		branches: []*org.Branch{
			{ID: 1, Name: i18n.New("مستودع القاهرة الرئيسي", "Main Warehouse")},
		},
		nextMID: 10,
	}
}

func (m *mockOrgRepoTeamTest) ListBranchesByOrg(ctx context.Context, orgID int64) ([]*org.Branch, error) {
	return m.branches, nil
}

func (m *mockOrgRepoTeamTest) ListMembersByOrg(ctx context.Context, orgID int64) ([]*org.Member, error) {
	var list []*org.Member
	for _, mem := range m.members {
		if mem.OrganizationID == orgID {
			list = append(list, mem)
		}
	}
	return list, nil
}

func (m *mockOrgRepoTeamTest) AddMember(ctx context.Context, mem *org.Member) error {
	for _, existing := range m.members {
		if existing.OrganizationID == mem.OrganizationID && existing.UserID == mem.UserID {
			existing.BranchID = mem.BranchID
			existing.RoleID = mem.RoleID
			existing.OrgRoleID = mem.OrgRoleID
			existing.RoleKey = mem.RoleKey
			existing.EmployeeCode = mem.EmployeeCode
			existing.JobTitle = mem.JobTitle
			existing.IsActive = mem.IsActive
			mem.ID = existing.ID
			return nil
		}
	}
	m.nextMID++
	mem.ID = m.nextMID
	m.members[mem.ID] = mem
	return nil
}

func (m *mockOrgRepoTeamTest) ListEmployees(ctx context.Context, orgID int64) ([]*org.EmployeeView, error) {
	var views []*org.EmployeeView
	for _, mem := range m.members {
		if mem.OrganizationID == orgID {
			views = append(views, &org.EmployeeView{
				Member:     mem,
				UserName:   "د. أحمد جمال",
				UserEmail:  "ahmed@supplier.com",
				UserPhone:  "01099887766",
				UserStatus: "active",
				RoleName:   "موظف مبيعات وتوريد",
				BranchName: "مستودع القاهرة الرئيسي",
			})
		}
	}
	return views, nil
}

func (m *mockOrgRepoTeamTest) ListEmployeesWithTotal(ctx context.Context, orgID int64, _, _ int) ([]*org.EmployeeView, int, error) {
	views, err := m.ListEmployees(ctx, orgID)
	return views, len(views), err
}

func (m *mockOrgRepoTeamTest) ListBranchesWithTotal(_ context.Context, _ org.BranchFilter, _, _ int) ([]*org.Branch, int, error) {
	return nil, 0, nil
}

func (m *mockOrgRepoTeamTest) AdminBranchStats(_ context.Context) (org.AdminBranchStatsResult, error) {
	return org.AdminBranchStatsResult{}, nil
}

func (m *mockOrgRepoTeamTest) ToggleMemberStatus(ctx context.Context, orgID, memberID int64) error {
	if mem, ok := m.members[memberID]; ok && mem.OrganizationID == orgID {
		mem.IsActive = !mem.IsActive
	}
	return nil
}

func (m *mockOrgRepoTeamTest) GetMemberByID(ctx context.Context, orgID, memberID int64) (*org.Member, error) {
	if mem, ok := m.members[memberID]; ok && mem.OrganizationID == orgID {
		return mem, nil
	}
	return nil, nil
}

func (m *mockOrgRepoTeamTest) RemoveMember(ctx context.Context, orgID, userID int64) error {
	for id, mem := range m.members {
		if mem.OrganizationID == orgID && mem.UserID == userID {
			delete(m.members, id)
		}
	}
	return nil
}

func (m *mockOrgRepoTeamTest) ListRoles(_ context.Context, orgID int64) ([]*org.Role, error) {
	return []*org.Role{
		{ID: 1, OrganizationID: orgID, Key: "org_owner", Name: i18n.New("مالك المنشأة", "Owner"), IsSystem: true, IsOwner: true},
		{ID: 2, OrganizationID: orgID, Key: "org_manager", Name: i18n.New("مدير", "Manager"), IsSystem: true},
		{ID: 3, OrganizationID: orgID, Key: "org_warehouse", Name: i18n.New("أمين مخزن", "Warehouse"), IsSystem: true},
		{ID: 4, OrganizationID: orgID, Key: "org_employee", Name: i18n.New("موظف مبيعات وتوريد", "Sales Rep"), IsSystem: true},
	}, nil
}

func (m *mockOrgRepoTeamTest) GetRole(_ context.Context, orgID, roleID int64) (*org.Role, error) {
	for _, r := range []*org.Role{
		{ID: 1, OrganizationID: orgID, Key: "org_owner", Name: i18n.New("مالك المنشأة", "Owner"), IsSystem: true, IsOwner: true},
		{ID: 2, OrganizationID: orgID, Key: "org_manager", Name: i18n.New("مدير", "Manager"), IsSystem: true},
		{ID: 3, OrganizationID: orgID, Key: "org_warehouse", Name: i18n.New("أمين مخزن", "Warehouse"), IsSystem: true},
		{ID: 4, OrganizationID: orgID, Key: "org_employee", Name: i18n.New("موظف مبيعات وتوريد", "Sales Rep"), IsSystem: true},
	} {
		if r.ID == roleID {
			return r, nil
		}
	}
	return nil, nil
}

func (m *mockOrgRepoTeamTest) AssignBranchManager(_ context.Context, orgID, branchID int64, managerUserID *int64) error {
	return nil
}

func (m *mockOrgRepoTeamTest) UpdateMember(_ context.Context, orgID, memberID int64, patch org.MemberPatch) error {
	if mem, ok := m.members[memberID]; ok && mem.OrganizationID == orgID {
		if patch.JobTitle != nil {
			mem.JobTitle = *patch.JobTitle
		}
		if patch.EmployeeCode != nil {
			mem.EmployeeCode = *patch.EmployeeCode
		}
		if patch.BranchID != nil {
			mem.BranchID = patch.BranchID
		}
		if patch.RoleKey != nil {
			mem.RoleKey = *patch.RoleKey
		}
		if patch.OrgRoleID != nil {
			mem.OrgRoleID = patch.OrgRoleID
			mem.RoleID = *patch.OrgRoleID
		}
		if patch.IsActive != nil {
			mem.IsActive = *patch.IsActive
		}
	}
	return nil
}

func (m *mockOrgRepoTeamTest) CountRoleMembers(_ context.Context, _ int64) (map[int64]int, error) {
	return map[int64]int{}, nil
}

func (m *mockOrgRepoTeamTest) AssignMemberRole(_ context.Context, orgID, memberID, roleID int64) error {
	if mem, ok := m.members[memberID]; ok && mem.OrganizationID == orgID {
		mem.RoleID = roleID
		mem.OrgRoleID = &roleID
		for _, r := range []*org.Role{
			{ID: 1, Key: "org_owner"},
			{ID: 2, Key: "org_manager"},
			{ID: 3, Key: "org_warehouse"},
			{ID: 4, Key: "org_employee"},
		} {
			if r.ID == roleID {
				mem.RoleKey = r.Key
				break
			}
		}
	}
	return nil
}
