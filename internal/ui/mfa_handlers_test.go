package ui

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

type testMockIdentityRepo struct {
	users      map[int64]*identity.User
	usersByEm  map[string]*identity.User
	security   map[int64]*identity.UserSecurity
	mfa        map[int64]*identity.UserMFA
	nextUserID int64
}

func newTestMockIdentityRepo() *testMockIdentityRepo {
	return &testMockIdentityRepo{
		users:      make(map[int64]*identity.User),
		usersByEm:  make(map[string]*identity.User),
		security:   make(map[int64]*identity.UserSecurity),
		mfa:        make(map[int64]*identity.UserMFA),
		nextUserID: 1,
	}
}

func (m *testMockIdentityRepo) CreateUser(ctx context.Context, u *identity.User) error {
	u.ID = m.nextUserID
	m.nextUserID++
	u.Status = identity.StatusActive
	m.users[u.ID] = u
	m.usersByEm[u.Email] = u
	return nil
}

func (m *testMockIdentityRepo) RegisterOrganization(ctx context.Context, u *identity.User, org identity.RegisterOrgInput) (*identity.RegisterOrgResult, error) {
	_ = m.CreateUser(ctx, u)
	return &identity.RegisterOrgResult{
		OrganizationID:     10,
		OrganizationType:   "customer",
		OrganizationStatus: "approved",
	}, nil
}

func (m *testMockIdentityRepo) GetUserByID(ctx context.Context, id int64) (*identity.User, error) {
	u, ok := m.users[id]
	if !ok {
		return nil, apperr.NotFound("user")
	}
	return u, nil
}

func (m *testMockIdentityRepo) GetUserByEmail(ctx context.Context, email string) (*identity.User, error) {
	u, ok := m.usersByEm[email]
	if !ok {
		return nil, apperr.NotFound("user")
	}
	return u, nil
}

func (m *testMockIdentityRepo) UpdateUser(ctx context.Context, u *identity.User) error {
	m.users[u.ID] = u
	m.usersByEm[u.Email] = u
	return nil
}

func (m *testMockIdentityRepo) GetSecurity(ctx context.Context, userID int64) (*identity.UserSecurity, error) {
	s, ok := m.security[userID]
	if !ok {
		s = &identity.UserSecurity{UserID: userID}
		m.security[userID] = s
	}
	return s, nil
}

func (m *testMockIdentityRepo) UpsertSecurity(ctx context.Context, s *identity.UserSecurity) error {
	m.security[s.UserID] = s
	return nil
}

func (m *testMockIdentityRepo) GetMFA(ctx context.Context, userID int64) (*identity.UserMFA, error) {
	val, ok := m.mfa[userID]
	if !ok {
		val = &identity.UserMFA{UserID: userID}
	}
	return val, nil
}

func (m *testMockIdentityRepo) UpsertMFA(ctx context.Context, mfa *identity.UserMFA) error {
	m.mfa[mfa.UserID] = mfa
	return nil
}

func (m *testMockIdentityRepo) GetPermissionsForUser(ctx context.Context, userID, orgID int64) ([]string, error) {
	return []string{"pharmacy.dashboard.view", "vendor.dashboard.view", "vendor.session.view"}, nil
}

func (m *testMockIdentityRepo) GetRolesForUser(ctx context.Context, userID int64) ([]string, error) {
	return []string{"user"}, nil
}

func (m *testMockIdentityRepo) UserBelongsToOrg(ctx context.Context, userID, orgID int64) (bool, error) {
	return true, nil
}

func (m *testMockIdentityRepo) CreateAddress(ctx context.Context, addr *identity.UserAddress) error {
	return nil
}
func (m *testMockIdentityRepo) GetAddressByID(ctx context.Context, id, userID int64) (*identity.UserAddress, error) {
	return nil, nil
}
func (m *testMockIdentityRepo) ListAddresses(ctx context.Context, userID int64) ([]*identity.UserAddress, error) {
	return nil, nil
}
func (m *testMockIdentityRepo) UpdateAddress(ctx context.Context, addr *identity.UserAddress) error {
	return nil
}
func (m *testMockIdentityRepo) DeleteAddress(ctx context.Context, id, userID int64) error { return nil }
func (m *testMockIdentityRepo) ListAddressHistory(ctx context.Context, userID int64, limit int) ([]*identity.UserAddressHistory, error) {
	return nil, nil
}

func (m *testMockIdentityRepo) AddFavorite(ctx context.Context, userID, productID int64) error {
	return nil
}
func (m *testMockIdentityRepo) RemoveFavorite(ctx context.Context, userID, productID int64) error {
	return nil
}
func (m *testMockIdentityRepo) ListFavorites(ctx context.Context, userID int64) ([]int64, error) {
	return nil, nil
}

func (m *testMockIdentityRepo) AdminListUsers(ctx context.Context, role, status string) ([]*identity.User, error) {
	return nil, nil
}
func (m *testMockIdentityRepo) ListStaffUserIDs(ctx context.Context) ([]int64, error) {
	return nil, nil
}
func (m *testMockIdentityRepo) AdminListUsersWithTotal(ctx context.Context, filter identity.AdminUserFilter, limit, offset int) ([]*identity.User, int, error) {
	return nil, 0, nil
}
func (m *testMockIdentityRepo) AdminUserStats(ctx context.Context) (identity.AdminUserStatsResult, error) {
	return identity.AdminUserStatsResult{}, nil
}
func (m *testMockIdentityRepo) SearchUsers(ctx context.Context, query, role string, limit int) ([]*identity.User, error) {
	return nil, nil
}
func (m *testMockIdentityRepo) AdminCountUsers(ctx context.Context) (int, error) {
	return len(m.users), nil
}
func (m *testMockIdentityRepo) DefaultOrgForUser(ctx context.Context, userID int64) (int64, error) {
	return 10, nil
}
func (m *testMockIdentityRepo) DefaultOrgInfoForUser(ctx context.Context, userID int64) (int64, string, string, error) {
	return 10, "customer", "approved", nil
}
func (m *testMockIdentityRepo) ListUserOrganizations(ctx context.Context, userID int64) ([]*identity.UserOrgMembership, error) {
	return []*identity.UserOrgMembership{
		{OrganizationID: 10, OrgType: "customer", OrgStatus: "approved", RoleKey: "owner", IsActive: true},
	}, nil
}
func (m *testMockIdentityRepo) AdminUpdateUserStatus(ctx context.Context, id int64, status string, actorID int64) error {
	return nil
}
func (m *testMockIdentityRepo) AdminResetMFA(ctx context.Context, id int64, actorID int64) error {
	return nil
}
func (m *testMockIdentityRepo) AdminAssignRole(ctx context.Context, id int64, role string, actorID int64) error {
	return nil
}
func (m *testMockIdentityRepo) AdminSetPassword(ctx context.Context, userID int64, passwordHash string, actorID int64) error {
	return nil
}
func (m *testMockIdentityRepo) AdminUpdateUserDetails(ctx context.Context, userID int64, in identity.AdminEditUserInput, actorID int64) error {
	return nil
}
func (m *testMockIdentityRepo) GetNationalID(ctx context.Context, userID int64) (string, error) {
	return "", nil
}

func (m *testMockIdentityRepo) ListModerators(ctx context.Context) ([]*identity.Moderator, error) {
	return nil, nil
}

func (m *testMockIdentityRepo) ModeratorSubordinateIDs(ctx context.Context, parentID int64) ([]int64, error) {
	return nil, nil
}

func (m *testMockIdentityRepo) ModeratorParentID(ctx context.Context, userID int64) (*int64, error) {
	return nil, nil
}

func (m *testMockIdentityRepo) SetModeratorParent(ctx context.Context, userID int64, parentID *int64, actorID int64) error {
	return nil
}

func (m *testMockIdentityRepo) ListPlatformRoles(ctx context.Context) ([]*identity.PlatformRole, error) {
	return nil, nil
}
func (m *testMockIdentityRepo) GetPlatformRole(ctx context.Context, key string) (*identity.PlatformRole, error) {
	return nil, nil
}
func (m *testMockIdentityRepo) CreatePlatformRole(ctx context.Context, role *identity.PlatformRole, actorID int64) error {
	return nil
}
func (m *testMockIdentityRepo) UpdatePlatformRole(ctx context.Context, role *identity.PlatformRole, actorID int64) error {
	return nil
}
func (m *testMockIdentityRepo) DeletePlatformRole(ctx context.Context, key string) error { return nil }

func (m *testMockIdentityRepo) GetPreferences(ctx context.Context, userID int64) (*identity.UserPreferences, error) {
	return nil, nil
}
func (m *testMockIdentityRepo) UpdatePreferences(ctx context.Context, p *identity.UserPreferences) error {
	return nil
}

func (m *testMockIdentityRepo) ListSessionPlans(ctx context.Context) ([]*identity.SessionPlan, error) {
	return nil, nil
}
func (m *testMockIdentityRepo) GetSessionPlanByID(ctx context.Context, id int64) (*identity.SessionPlan, error) {
	return nil, nil
}
func (m *testMockIdentityRepo) SetMaxLoginSessions(ctx context.Context, userID int64, max int) error {
	return nil
}
func (m *testMockIdentityRepo) GetOrgPlanLimits(ctx context.Context, orgID int64) (int, int, string, error) {
	return 5, 5, "باقة احترافية", nil
}

func (m *testMockIdentityRepo) CreateAccountDeletionRequest(ctx context.Context, req *identity.AccountDeletionRequest) error {
	return nil
}
func (m *testMockIdentityRepo) GetPendingAccountDeletionRequest(ctx context.Context, userID int64) (*identity.AccountDeletionRequest, error) {
	return nil, nil
}
func (m *testMockIdentityRepo) CancelAccountDeletionRequest(ctx context.Context, userID, requestID int64) error {
	return nil
}
func (m *testMockIdentityRepo) ListAccountDeletionRequests(ctx context.Context, status string) ([]*identity.AccountDeletionRequest, error) {
	return nil, nil
}
func (m *testMockIdentityRepo) ListAccountDeletionRequestsWithTotal(ctx context.Context, status string, limit, offset int) ([]*identity.AccountDeletionRequest, int, error) {
	return nil, 0, nil
}
func (m *testMockIdentityRepo) ReviewAccountDeletionRequest(ctx context.Context, requestID, reviewerID int64, approve bool, adminNotes string) error {
	return nil
}

func setupTestMFAHandler(t *testing.T) (*UIHandler, *identity.Service, *testMockIdentityRepo, *identity.SessionStore) {
	t.Helper()
	repo := newTestMockIdentityRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := identity.NewSessionStore(nil, config.Session{
		CookieName: "dawa24_session",
		TTL:        24 * time.Hour,
		SecureOnly: false,
	})
	idSvc := identity.NewService(repo, store, logger)

	h := &UIHandler{
		idSvc: idSvc,
		log:   logger,
	}
	return h, idSvc, repo, store
}
