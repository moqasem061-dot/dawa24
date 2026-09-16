package http_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	identityHttp "github.com/muhiya/dawa24-store/internal/modules/identity/http"
	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

type stubRepo struct{ t *testing.T }

func (r stubRepo) fail(method string) {
	r.t.Helper()
	r.t.Fatalf("repository.%s was called; the request should have been rejected before reaching the repository", method)
}

func (r stubRepo) CreateUser(context.Context, *identity.User) error { r.fail("CreateUser"); return nil }
func (r stubRepo) GetUserByID(context.Context, int64) (*identity.User, error) {
	r.fail("GetUserByID")
	return nil, nil
}
func (r stubRepo) GetUserByEmail(context.Context, string) (*identity.User, error) {
	r.fail("GetUserByEmail")
	return nil, nil
}
func (r stubRepo) UpdateUser(context.Context, *identity.User) error { r.fail("UpdateUser"); return nil }
func (r stubRepo) GetSecurity(context.Context, int64) (*identity.UserSecurity, error) {
	r.fail("GetSecurity")
	return nil, nil
}
func (r stubRepo) UpsertSecurity(context.Context, *identity.UserSecurity) error {
	r.fail("UpsertSecurity")
	return nil
}
func (r stubRepo) GetMFA(context.Context, int64) (*identity.UserMFA, error) {
	r.fail("GetMFA")
	return nil, nil
}
func (r stubRepo) UpsertMFA(context.Context, *identity.UserMFA) error {
	r.fail("UpsertMFA")
	return nil
}
func (r stubRepo) GetPermissionsForUser(context.Context, int64, int64) ([]string, error) {
	r.fail("GetPermissionsForUser")
	return nil, nil
}
func (r stubRepo) GetRolesForUser(context.Context, int64) ([]string, error) {
	r.fail("GetRolesForUser")
	return nil, nil
}
func (r stubRepo) UserBelongsToOrg(context.Context, int64, int64) (bool, error) {
	r.fail("UserBelongsToOrg")
	return false, nil
}
func (r stubRepo) CreateAddress(context.Context, *identity.UserAddress) error {
	r.fail("CreateAddress")
	return nil
}
func (r stubRepo) GetAddressByID(context.Context, int64, int64) (*identity.UserAddress, error) {
	r.fail("GetAddressByID")
	return nil, nil
}
func (r stubRepo) ListAddresses(context.Context, int64) ([]*identity.UserAddress, error) {
	r.fail("ListAddresses")
	return nil, nil
}
func (r stubRepo) UpdateAddress(context.Context, *identity.UserAddress) error {
	r.fail("UpdateAddress")
	return nil
}
func (r stubRepo) DeleteAddress(context.Context, int64, int64) error {
	r.fail("DeleteAddress")
	return nil
}
func (r stubRepo) AddFavorite(context.Context, int64, int64) error {
	r.fail("AddFavorite")
	return nil
}
func (r stubRepo) RemoveFavorite(context.Context, int64, int64) error {
	r.fail("RemoveFavorite")
	return nil
}
func (r stubRepo) ListFavorites(context.Context, int64) ([]int64, error) {
	r.fail("ListFavorites")
	return nil, nil
}
func (r stubRepo) AdminListUsers(context.Context, string, string) ([]*identity.User, error) {
	r.fail("AdminListUsers")
	return nil, nil
}
func (r stubRepo) ListStaffUserIDs(context.Context) ([]int64, error) {
	r.fail("ListStaffUserIDs")
	return nil, nil
}
func (r stubRepo) AdminListUsersWithTotal(context.Context, identity.AdminUserFilter, int, int) ([]*identity.User, int, error) {
	r.fail("AdminListUsersWithTotal")
	return nil, 0, nil
}
func (r stubRepo) AdminUserStats(context.Context) (identity.AdminUserStatsResult, error) {
	r.fail("AdminUserStats")
	return identity.AdminUserStatsResult{}, nil
}
func (r stubRepo) AdminUpdateUserStatus(context.Context, int64, string, int64) error {
	r.fail("AdminUpdateUserStatus")
	return nil
}
func (r stubRepo) AdminResetMFA(context.Context, int64, int64) error {
	r.fail("AdminResetMFA")
	return nil
}
func (r stubRepo) AdminAssignRole(context.Context, int64, string, int64) error {
	r.fail("AdminAssignRole")
	return nil
}
func (r stubRepo) AdminSetPassword(context.Context, int64, string, int64) error {
	r.fail("AdminSetPassword")
	return nil
}
func (r stubRepo) AdminUpdateUserDetails(context.Context, int64, identity.AdminEditUserInput, int64) error {
	r.fail("AdminUpdateUserDetails")
	return nil
}
func (r stubRepo) GetNationalID(context.Context, int64) (string, error) {
	r.fail("GetNationalID")
	return "", nil
}
func (r stubRepo) ListUserOrganizations(context.Context, int64) ([]*identity.UserOrgMembership, error) {
	r.fail("ListUserOrganizations")
	return nil, nil
}
func (r stubRepo) CreateAccountDeletionRequest(context.Context, *identity.AccountDeletionRequest) error {
	r.fail("CreateAccountDeletionRequest")
	return nil
}
func (r stubRepo) GetPendingAccountDeletionRequest(context.Context, int64) (*identity.AccountDeletionRequest, error) {
	r.fail("GetPendingAccountDeletionRequest")
	return nil, nil
}
func (r stubRepo) CancelAccountDeletionRequest(context.Context, int64, int64) error {
	r.fail("CancelAccountDeletionRequest")
	return nil
}
func (r stubRepo) ListAccountDeletionRequests(context.Context, string) ([]*identity.AccountDeletionRequest, error) {
	r.fail("ListAccountDeletionRequests")
	return nil, nil
}
func (r stubRepo) ListAccountDeletionRequestsWithTotal(context.Context, string, int, int) ([]*identity.AccountDeletionRequest, int, error) {
	r.fail("ListAccountDeletionRequestsWithTotal")
	return nil, 0, nil
}
func (r stubRepo) ReviewAccountDeletionRequest(context.Context, int64, int64, bool, string) error {
	r.fail("ReviewAccountDeletionRequest")
	return nil
}
func (r stubRepo) GetOrgPlanLimits(context.Context, int64) (int, int, string, error) {
	return 3, 3, "الباقة الأساسية", nil
}

type happyRepo struct{}

func (happyRepo) CreateUser(ctx context.Context, u *identity.User) error {
	u.ID = 1
	return nil
}
func (happyRepo) GetUserByID(ctx context.Context, id int64) (*identity.User, error) {
	return &identity.User{
		ID:       id,
		Email:    "user@example.com",
		Name:     i18n.Text{"en": "User"},
		Status:   identity.StatusActive,
		Role:     "user",
		Language: "en",
	}, nil
}
func (happyRepo) GetUserByEmail(ctx context.Context, email string) (*identity.User, error) {
	return &identity.User{
		ID:           1,
		Email:        email,
		Name:         i18n.Text{"en": "User"},
		Status:       identity.StatusActive,
		Role:         "user",
		Language:     "en",
		PasswordHash: "$2a$10$abcdefghijklmnopqrstuu",
	}, nil
}
func (happyRepo) UpdateUser(ctx context.Context, u *identity.User) error {
	return nil
}
func (happyRepo) RegisterOrganization(ctx context.Context, u *identity.User, org identity.RegisterOrgInput) (*identity.RegisterOrgResult, error) {
	u.ID = 1
	return &identity.RegisterOrgResult{
		OrganizationID:     1,
		OrganizationType:   org.Type,
		OrganizationStatus: "pending",
	}, nil
}
func (happyRepo) GetSecurity(ctx context.Context, id int64) (*identity.UserSecurity, error) {
	return &identity.UserSecurity{UserID: id}, nil
}
func (happyRepo) UpsertSecurity(ctx context.Context, s *identity.UserSecurity) error { return nil }
func (happyRepo) GetMFA(ctx context.Context, id int64) (*identity.UserMFA, error) {
	return &identity.UserMFA{UserID: id, Enabled: false}, nil
}
func (happyRepo) UpsertMFA(ctx context.Context, m *identity.UserMFA) error { return nil }
func (happyRepo) GetPermissionsForUser(ctx context.Context, userID, orgID int64) ([]string, error) {
	return []string{"customer"}, nil
}
func (happyRepo) GetRolesForUser(ctx context.Context, userID int64) ([]string, error) {
	return []string{"customer"}, nil
}
func (happyRepo) UserBelongsToOrg(ctx context.Context, userID, orgID int64) (bool, error) {
	return true, nil
}
func (happyRepo) CreateAddress(ctx context.Context, a *identity.UserAddress) error {
	a.ID = 1
	return nil
}
func (happyRepo) GetAddressByID(ctx context.Context, id, userID int64) (*identity.UserAddress, error) {
	return &identity.UserAddress{ID: id, UserID: userID, Title: "Home", Recipient: "User", Phone: "01000000000", Address: "123 Main St", CityID: 1}, nil
}
func (happyRepo) ListAddresses(ctx context.Context, userID int64) ([]*identity.UserAddress, error) {
	return []*identity.UserAddress{{ID: 1, UserID: userID, Title: "Home", Recipient: "User", Phone: "01000000000", Address: "123 Main St", CityID: 1}}, nil
}
func (happyRepo) UpdateAddress(ctx context.Context, a *identity.UserAddress) error { return nil }
func (happyRepo) DeleteAddress(ctx context.Context, id, userID int64) error        { return nil }
func (happyRepo) ListAddressHistory(ctx context.Context, userID int64, limit int) ([]*identity.UserAddressHistory, error) {
	return nil, nil
}
func (happyRepo) AddFavorite(ctx context.Context, userID, variantID int64) error {
	return nil
}
func (happyRepo) RemoveFavorite(ctx context.Context, userID, variantID int64) error {
	return nil
}
func (happyRepo) ListFavorites(ctx context.Context, userID int64) ([]int64, error) {
	return []int64{1, 2}, nil
}
func (happyRepo) AdminListUsers(ctx context.Context, role, status string) ([]*identity.User, error) {
	return []*identity.User{{ID: 1, Email: "user@example.com"}}, nil
}
func (happyRepo) ListStaffUserIDs(ctx context.Context) ([]int64, error) {
	return []int64{1}, nil
}
func (happyRepo) AdminListUsersWithTotal(ctx context.Context, filter identity.AdminUserFilter, limit, offset int) ([]*identity.User, int, error) {
	return []*identity.User{{ID: 1, Email: "user@example.com"}}, 1, nil
}
func (happyRepo) AdminUserStats(ctx context.Context) (identity.AdminUserStatsResult, error) {
	return identity.AdminUserStatsResult{Total: 1, Active: 1}, nil
}
func (happyRepo) SearchUsers(ctx context.Context, query, role string, limit int) ([]*identity.User, error) {
	return []*identity.User{{ID: 1, Email: "user@example.com"}}, nil
}
func (happyRepo) AdminCountUsers(ctx context.Context) (int, error) {
	return 1, nil
}
func (happyRepo) DefaultOrgForUser(ctx context.Context, userID int64) (int64, error) {
	return 1, nil
}
func (happyRepo) DefaultOrgInfoForUser(ctx context.Context, userID int64) (orgID int64, orgType, orgStatus string, err error) {
	return 1, "pharmacy", "approved", nil
}
func (happyRepo) ListUserOrganizations(ctx context.Context, userID int64) ([]*identity.UserOrgMembership, error) {
	return []*identity.UserOrgMembership{
		{
			OrganizationID: 1,
			OrgName:        i18n.Text{"ar": "Test Org"},
			OrgType:        "pharmacy",
			OrgStatus:      "approved",
			RoleKey:        "owner",
		},
	}, nil
}
func (happyRepo) AdminUpdateUserStatus(ctx context.Context, userID int64, status string, actorID int64) error {
	return nil
}
func (happyRepo) AdminResetMFA(ctx context.Context, userID int64, actorID int64) error {
	return nil
}
func (happyRepo) AdminAssignRole(ctx context.Context, userID int64, role string, actorID int64) error {
	return nil
}
func (happyRepo) AdminSetPassword(ctx context.Context, userID int64, passwordHash string, actorID int64) error {
	return nil
}
func (happyRepo) AdminUpdateUserDetails(ctx context.Context, userID int64, in identity.AdminEditUserInput, actorID int64) error {
	return nil
}
func (happyRepo) GetNationalID(ctx context.Context, userID int64) (string, error) {
	return "", nil
}
func (happyRepo) GetPreferences(ctx context.Context, userID int64) (*identity.UserPreferences, error) {
	return &identity.UserPreferences{UserID: userID}, nil
}
func (happyRepo) UpdatePreferences(ctx context.Context, p *identity.UserPreferences) error {
	return nil
}
func (happyRepo) ListSessionPlans(ctx context.Context) ([]*identity.SessionPlan, error) {
	return nil, nil
}
func (happyRepo) GetSessionPlanByID(ctx context.Context, id int64) (*identity.SessionPlan, error) {
	return nil, nil
}
func (happyRepo) SetMaxLoginSessions(ctx context.Context, userID int64, max int) error {
	return nil
}
func (happyRepo) CreateAccountDeletionRequest(ctx context.Context, req *identity.AccountDeletionRequest) error {
	return nil
}
func (happyRepo) GetPendingAccountDeletionRequest(ctx context.Context, userID int64) (*identity.AccountDeletionRequest, error) {
	return nil, nil
}
func (happyRepo) CancelAccountDeletionRequest(ctx context.Context, userID, requestID int64) error {
	return nil
}
func (happyRepo) ListAccountDeletionRequests(ctx context.Context, status string) ([]*identity.AccountDeletionRequest, error) {
	return nil, nil
}
func (happyRepo) ListAccountDeletionRequestsWithTotal(ctx context.Context, status string, limit, offset int) ([]*identity.AccountDeletionRequest, int, error) {
	return nil, 0, nil
}
func (happyRepo) ReviewAccountDeletionRequest(ctx context.Context, requestID, reviewerID int64, approve bool, adminNotes string) error {
	return nil
}
func (happyRepo) GetOrgPlanLimits(ctx context.Context, orgID int64) (int, int, string, error) {
	return 3, 3, "الباقة الأساسية", nil
}

const testCookieName = "dawa24_session"

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()

	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	svc := identity.NewService(stubRepo{t: t}, nil, log)
	handler := identityHttp.NewHandler(svc, config.Session{
		CookieName: testCookieName,
		TTL:        30 * 24 * time.Hour,
		SecureOnly: false,
	}, log)

	r := chi.NewRouter()
	r.Use(httpx.RequestID)
	r.Use(httpx.Recover(log))
	r.Use(httpx.Locale)
	handler.RegisterRoutes(r)
	return r
}

func (happyRepo) ListModerators(ctx context.Context) ([]*identity.Moderator, error) {
	return nil, nil
}

func (happyRepo) ModeratorSubordinateIDs(ctx context.Context, parentID int64) ([]int64, error) {
	return nil, nil
}

func (happyRepo) ModeratorParentID(ctx context.Context, userID int64) (*int64, error) {
	return nil, nil
}

func (happyRepo) SetModeratorParent(ctx context.Context, userID int64, parentID *int64, actorID int64) error {
	return nil
}

func (stubRepo) ListModerators(ctx context.Context) ([]*identity.Moderator, error) {
	return nil, nil
}

func (stubRepo) ModeratorSubordinateIDs(ctx context.Context, parentID int64) ([]int64, error) {
	return nil, nil
}

func (stubRepo) ModeratorParentID(ctx context.Context, userID int64) (*int64, error) {
	return nil, nil
}

func (stubRepo) SetModeratorParent(ctx context.Context, userID int64, parentID *int64, actorID int64) error {
	return nil
}
