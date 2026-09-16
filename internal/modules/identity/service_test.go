package identity

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// mockRepo is an in-memory repository for unit testing identity workflows.
type mockRepo struct {
	users       map[int64]*User
	usersByMail map[string]*User
	securities  map[int64]*UserSecurity
	mfas        map[int64]*UserMFA
	permissions map[int64][]string
	orgMembers  map[string]bool
	addresses        map[int64]*UserAddress
	favorites        map[int64][]int64
	moderatorParents map[int64]*int64
	nextID           int64
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		users:            map[int64]*User{},
		usersByMail:      map[string]*User{},
		securities:       map[int64]*UserSecurity{},
		mfas:             map[int64]*UserMFA{},
		permissions:      map[int64][]string{},
		orgMembers:       map[string]bool{},
		addresses:        map[int64]*UserAddress{},
		favorites:        map[int64][]int64{},
		moderatorParents: map[int64]*int64{},
		nextID:           1,
	}
}

func (m *mockRepo) CreateUser(_ context.Context, u *User) error {
	cleanEmail := NormalizeEmail(u.Email)
	if _, exists := m.usersByMail[cleanEmail]; exists {
		return apperr.Conflict("user.email_exists", "Email already exists")
	}
	u.ID = m.nextID
	u.PublicID = "pub-test-uuid"
	u.CreatedAt = time.Now()
	u.UpdatedAt = time.Now()
	m.nextID++

	m.users[u.ID] = u
	m.usersByMail[cleanEmail] = u
	return nil
}

func (m *mockRepo) GetUserByID(_ context.Context, id int64) (*User, error) {
	u, ok := m.users[id]
	if !ok {
		return nil, apperr.NotFound("user")
	}
	return u, nil
}

func (m *mockRepo) GetUserByEmail(_ context.Context, email string) (*User, error) {
	u, ok := m.usersByMail[NormalizeEmail(email)]
	if !ok {
		return nil, apperr.NotFound("user")
	}
	return u, nil
}

func (m *mockRepo) UpdateUser(_ context.Context, u *User) error {
	m.users[u.ID] = u
	m.usersByMail[NormalizeEmail(u.Email)] = u
	return nil
}

func (m *mockRepo) GetSecurity(_ context.Context, userID int64) (*UserSecurity, error) {
	s, ok := m.securities[userID]
	if !ok {
		return &UserSecurity{UserID: userID}, nil
	}
	return s, nil
}

func (m *mockRepo) UpsertSecurity(_ context.Context, s *UserSecurity) error {
	m.securities[s.UserID] = s
	return nil
}

func (m *mockRepo) GetMFA(_ context.Context, userID int64) (*UserMFA, error) {
	mfa, ok := m.mfas[userID]
	if !ok {
		return &UserMFA{UserID: userID}, nil
	}
	return mfa, nil
}

func (m *mockRepo) UpsertMFA(_ context.Context, mfa *UserMFA) error {
	m.mfas[mfa.UserID] = mfa
	return nil
}

func (m *mockRepo) GetPermissionsForUser(_ context.Context, userID int64, orgID int64) ([]string, error) {
	return m.permissions[userID], nil
}

func (m *mockRepo) GetRolesForUser(_ context.Context, userID int64) ([]string, error) {
	if u, ok := m.users[userID]; ok {
		return []string{u.Role}, nil
	}
	return []string{}, nil
}

func (m *mockRepo) UserBelongsToOrg(_ context.Context, userID int64, orgID int64) (bool, error) {
	return true, nil
}

func (m *mockRepo) CreateAddress(_ context.Context, addr *UserAddress) error {
	addr.ID = m.nextID
	m.nextID++
	m.addresses[addr.ID] = addr
	return nil
}
func (m *mockRepo) GetAddressByID(_ context.Context, id, userID int64) (*UserAddress, error) {
	a, ok := m.addresses[id]
	if !ok || a.UserID != userID {
		return nil, apperr.NotFound("user_address")
	}
	return a, nil
}
func (m *mockRepo) ListAddresses(_ context.Context, userID int64) ([]*UserAddress, error) {
	var list []*UserAddress
	for _, a := range m.addresses {
		if a.UserID == userID {
			list = append(list, a)
		}
	}
	return list, nil
}
func (m *mockRepo) UpdateAddress(_ context.Context, addr *UserAddress) error {
	m.addresses[addr.ID] = addr
	return nil
}
func (m *mockRepo) DeleteAddress(_ context.Context, id, userID int64) error {
	delete(m.addresses, id)
	return nil
}

func (m *mockRepo) AddFavorite(_ context.Context, userID, productID int64) error {
	m.favorites[userID] = append(m.favorites[userID], productID)
	return nil
}
func (m *mockRepo) RemoveFavorite(_ context.Context, userID, productID int64) error {
	var rem []int64
	for _, id := range m.favorites[userID] {
		if id != productID {
			rem = append(rem, id)
		}
	}
	m.favorites[userID] = rem
	return nil
}
func (m *mockRepo) ListFavorites(_ context.Context, userID int64) ([]int64, error) {
	return m.favorites[userID], nil
}
func (m *mockRepo) AdminListUsers(ctx context.Context, role, status string) ([]*User, error) {
	var list []*User
	for _, u := range m.users {
		list = append(list, u)
	}
	return list, nil
}
func (m *mockRepo) ListStaffUserIDs(_ context.Context) ([]int64, error) {
	return []int64{1}, nil
}
func (m *mockRepo) AdminListUsersWithTotal(ctx context.Context, filter AdminUserFilter, limit, offset int) ([]*User, int, error) {
	var list []*User
	for _, u := range m.users {
		list = append(list, u)
	}
	return list, len(list), nil
}
func (m *mockRepo) AdminUserStats(ctx context.Context) (AdminUserStatsResult, error) {
	return AdminUserStatsResult{Total: len(m.users)}, nil
}
func (m *mockRepo) SearchUsers(ctx context.Context, query, role string, limit int) ([]*User, error) {
	var list []*User
	for _, u := range m.users {
		list = append(list, u)
	}
	return list, nil
}
func (m *mockRepo) AdminUpdateUserStatus(ctx context.Context, id int64, status string, actorID int64) error {
	if u, ok := m.users[id]; ok {
		u.Status = UserStatus(status)
	}
	return nil
}
func (m *mockRepo) AdminResetMFA(ctx context.Context, id int64, actorID int64) error {
	delete(m.mfas, id)
	return nil
}
func (m *mockRepo) AdminAssignRole(ctx context.Context, id int64, role string, actorID int64) error {
	if u, ok := m.users[id]; ok {
		u.Role = role
	}
	return nil
}
func (m *mockRepo) AdminSetPassword(ctx context.Context, userID int64, passwordHash string, actorID int64) error {
	if u, ok := m.users[userID]; ok {
		u.PasswordHash = passwordHash
	}
	return nil
}
func (m *mockRepo) AdminUpdateUserDetails(ctx context.Context, userID int64, in AdminEditUserInput, actorID int64) error {
	if u, ok := m.users[userID]; ok {
		u.Name = i18n.Text{"ar": in.NameAr, "en": in.NameEn}
		u.Email = in.Email
		u.Phone = in.Phone
		u.AvatarURL = in.AvatarURL
		if in.Status != "" {
			u.Status = in.Status
		}
	}
	return nil
}
func (m *mockRepo) GetNationalID(ctx context.Context, userID int64) (string, error) {
	return "", nil
}
func (m *mockRepo) CreateAccountDeletionRequest(_ context.Context, _ *AccountDeletionRequest) error {
	return nil
}
func (m *mockRepo) GetPendingAccountDeletionRequest(_ context.Context, _ int64) (*AccountDeletionRequest, error) {
	return nil, nil
}
func (m *mockRepo) CancelAccountDeletionRequest(_ context.Context, _, _ int64) error {
	return nil
}
func (m *mockRepo) ListAccountDeletionRequests(_ context.Context, _ string) ([]*AccountDeletionRequest, error) {
	return nil, nil
}
func (m *mockRepo) ListAccountDeletionRequestsWithTotal(_ context.Context, _ string, _, _ int) ([]*AccountDeletionRequest, int, error) {
	return nil, 0, nil
}
func (m *mockRepo) ReviewAccountDeletionRequest(_ context.Context, _, _ int64, _ bool, _ string) error {
	return nil
}

func TestServiceRegisterAndLogin(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(repo, nil, logger)

	// 1. Register a new user
	regInput := RegisterInput{
		Email:    "pharmacist@dawa24.eg",
		Password: "SecurePassword123!",
		NameAr:   "صيدلية الشفاء",
		NameEn:   "El-Shefaa Pharmacy",
		Role:     "user",
		Language: i18n.AR,
		Phone:    "+201012345678",
	}

	user, _, err := svc.Register(ctx, regInput)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	if user.Email != "pharmacist@dawa24.eg" {
		t.Errorf("User email = %q; want %q", user.Email, "pharmacist@dawa24.eg")
	}

	// 2. Duplicate registration fails
	_, _, err = svc.Register(ctx, regInput)
	if err == nil || apperr.KindOf(err) != apperr.KindConflict {
		t.Errorf("expected Conflict error on duplicate email, got %v", err)
	}

	// 3. Login with correct password
	loginRes, err := svc.Login(ctx, LoginInput{
		Email:    "pharmacist@dawa24.eg",
		Password: "SecurePassword123!",
	})
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	if loginRes.User.ID != user.ID {
		t.Errorf("logged in user ID = %d; want %d", loginRes.User.ID, user.ID)
	}

	// 4. Login with wrong password
	_, err = svc.Login(ctx, LoginInput{
		Email:    "pharmacist@dawa24.eg",
		Password: "WrongPassword!",
	})
	if err != ErrInvalidCredentials {
		t.Errorf("expected ErrInvalidCredentials on wrong password, got %v", err)
	}

	// 5. Account lockout after 5 consecutive failures
	for i := 0; i < 4; i++ {
		_, _ = svc.Login(ctx, LoginInput{
			Email:    "pharmacist@dawa24.eg",
			Password: "WrongPassword!",
		})
	}

	// Next attempt should be locked
	_, err = svc.Login(ctx, LoginInput{
		Email:    "pharmacist@dawa24.eg",
		Password: "SecurePassword123!",
	})
	if err == nil || apperr.KindOf(err) != apperr.KindForbidden {
		t.Errorf("expected Forbidden error due to account lockout, got %v", err)
	}

	// 6. Admin functions
	users, err := svc.AdminListUsers(ctx, "", "")
	if err != nil || len(users) == 0 {
		t.Fatalf("AdminListUsers failed: %v", err)
	}
	if err := svc.AdminSuspendUser(ctx, user.ID, 1); err != nil {
		t.Fatalf("AdminSuspendUser failed: %v", err)
	}
	if err := svc.AdminReactivateUser(ctx, user.ID, 1); err != nil {
		t.Fatalf("AdminReactivateUser failed: %v", err)
	}
	if err := svc.AdminResetMFA(ctx, user.ID, 1); err != nil {
		t.Fatalf("AdminResetMFA failed: %v", err)
	}
	if err := svc.AdminAssignRole(ctx, user.ID, "manager", 1); err != nil {
		t.Fatalf("AdminAssignRole failed: %v", err)
	}

	// AdminSetPassword tests:
	// Weak password rejection
	if err := svc.AdminSetPassword(ctx, user.ID, 1, "weak"); err == nil {
		t.Fatalf("expected error setting short password, got nil")
	}
	if err := svc.AdminSetPassword(ctx, user.ID, 1, "alllowercasenodigits"); err == nil {
		t.Fatalf("expected error setting weak password, got nil")
	}

	// Strong password succeeds
	newPass := "SuperSecret123!"
	if err := svc.AdminSetPassword(ctx, user.ID, 1, newPass); err != nil {
		t.Fatalf("AdminSetPassword failed: %v", err)
	}

	// Verify user can login with new password
	loginRes, err = svc.Login(ctx, LoginInput{
		Email:    "pharmacist@dawa24.eg",
		Password: newPass,
	})
	if err != nil || loginRes.User.ID != user.ID {
		t.Fatalf("Login with new password failed: %v", err)
	}

	// AdminUpdateUserDetails test
	editIn := AdminEditUserInput{
		NameAr:     "صيدلي معدل",
		NameEn:     "Updated Pharmacist",
		Email:      "updated@dawa24.eg",
		Phone:      "+201099999999",
		AvatarURL:  "/uploads/avatars/user1.png",
		Status:     StatusActive,
		NationalID: "29001011234567",
	}
	if err := svc.AdminUpdateUserDetails(ctx, user.ID, editIn, 1); err != nil {
		t.Fatalf("AdminUpdateUserDetails failed: %v", err)
	}
	updatedUser, err := svc.AdminGetUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("AdminGetUser failed: %v", err)
	}
	if updatedUser.Email != "updated@dawa24.eg" || updatedUser.Phone != "+201099999999" || updatedUser.AvatarURL != "/uploads/avatars/user1.png" {
		t.Fatalf("AdminUpdateUserDetails did not update fields correctly: %+v", updatedUser)
	}

	// 7. Profile & Addresses
	me, err := svc.GetMe(ctx, user.ID, nil)
	if err != nil || me.User.ID != user.ID {
		t.Fatalf("GetMe failed: %v", err)
	}
	addr := &UserAddress{
		UserID:    user.ID,
		Recipient: "Pharmacist",
		Address:   "Tahrir St",
		CityID:    1,
		IsDefault: true,
	}
	createdAddr, err := svc.CreateAddress(ctx, addr)
	if err != nil {
		t.Fatalf("CreateAddress failed: %v", err)
	}
	addrs, err := svc.ListAddresses(ctx, user.ID)
	if err != nil || len(addrs) != 1 {
		t.Fatalf("ListAddresses failed: %v", err)
	}
	if err := svc.DeleteAddress(ctx, createdAddr.ID, user.ID); err != nil {
		t.Fatalf("DeleteAddress failed: %v", err)
	}

	// 8. Favorites
	if err := svc.AddFavorite(ctx, user.ID, 101); err != nil {
		t.Fatalf("AddFavorite failed: %v", err)
	}
	favs, err := svc.ListFavorites(ctx, user.ID)
	if err != nil || len(favs) != 1 {
		t.Fatalf("ListFavorites failed: %v", err)
	}
	if err := svc.RemoveFavorite(ctx, user.ID, 101); err != nil {
		t.Fatalf("RemoveFavorite failed: %v", err)
	}
}

func (m *mockRepo) AdminCountUsers(_ context.Context) (int, error) { return 0, nil }

func (m *mockRepo) DefaultOrgForUser(_ context.Context, _ int64) (int64, error) { return 0, nil }

func (m *mockRepo) DefaultOrgInfoForUser(_ context.Context, _ int64) (int64, string, string, error) {
	return 0, "", "", nil
}

func (m *mockRepo) ListUserOrganizations(_ context.Context, _ int64) ([]*UserOrgMembership, error) {
	return nil, nil
}

func (m *mockRepo) RegisterOrganization(_ context.Context, _ *User, _ RegisterOrgInput) (*RegisterOrgResult, error) {
	return &RegisterOrgResult{}, nil
}

func (m *mockRepo) ListAddressHistory(_ context.Context, _ int64, _ int) ([]*UserAddressHistory, error) {
	return nil, nil
}

func (m *mockRepo) GetPreferences(_ context.Context, _ int64) (*UserPreferences, error) {
	return &UserPreferences{}, nil
}

func (m *mockRepo) UpdatePreferences(_ context.Context, _ *UserPreferences) error {
	return nil
}

func (m *mockRepo) ListSessionPlans(_ context.Context) ([]*SessionPlan, error) {
	return nil, nil
}

func (m *mockRepo) GetSessionPlanByID(_ context.Context, _ int64) (*SessionPlan, error) {
	return &SessionPlan{MaxLoginSessions: 1}, nil
}

func (m *mockRepo) SetMaxLoginSessions(_ context.Context, _ int64, _ int) error {
	return nil
}

func (m *mockRepo) GetOrgPlanLimits(_ context.Context, _ int64) (int, int, string, error) {
	return 3, 3, "الباقة الأساسية", nil
}

func (m *mockRepo) ListPlatformRoles(_ context.Context) ([]*PlatformRole, error) { return nil, nil }
func (m *mockRepo) GetPlatformRole(_ context.Context, _ string) (*PlatformRole, error) {
	return nil, apperr.NotFound("role")
}
func (m *mockRepo) CreatePlatformRole(_ context.Context, _ *PlatformRole, _ int64) error { return nil }
func (m *mockRepo) UpdatePlatformRole(_ context.Context, _ *PlatformRole, _ int64) error { return nil }
func (m *mockRepo) DeletePlatformRole(_ context.Context, _ string) error                 { return nil }
