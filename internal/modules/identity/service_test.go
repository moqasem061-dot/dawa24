package identity

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
)

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
	if !errors.Is(err, ErrInvalidCredentials) {
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
