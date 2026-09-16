package identity

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

func TestServiceProfileAndPermissions(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(repo, nil, logger)

	// Register user
	user, _, err := svc.Register(ctx, RegisterInput{
		Email:    "test-mfa@dawa24.eg",
		Password: "InitialPassword123!",
		NameAr:   "مستخدم تجريبي",
		NameEn:   "Test User",
		Role:     "user",
		Language: i18n.AR,
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// 1. Profile update
	updatedUser, err := svc.UpdateProfile(ctx, user.ID, "اسم جديد", "New Name", "+201111111111", "Africa/Cairo", "ar")
	if err != nil {
		t.Fatalf("UpdateProfile failed: %v", err)
	}
	if updatedUser.Phone != "+201111111111" {
		t.Errorf("got phone %q, want +201111111111", updatedUser.Phone)
	}

	// 2. Address validation & update
	_, err = svc.CreateAddress(ctx, &UserAddress{UserID: user.ID, Recipient: "", Address: "123", CityID: 1})
	if err == nil {
		t.Error("expected validation error on empty recipient, got nil")
	}
	_, err = svc.CreateAddress(ctx, &UserAddress{UserID: user.ID, Recipient: "R", Address: "", CityID: 1})
	if err == nil {
		t.Error("expected validation error on empty address, got nil")
	}
	_, err = svc.CreateAddress(ctx, &UserAddress{UserID: user.ID, Recipient: "R", Address: "A", CityID: 0})
	if err == nil {
		t.Error("expected validation error on 0 city_id, got nil")
	}

	addr := &UserAddress{
		UserID:    user.ID,
		Recipient: "Recipient",
		Address:   "Original Address",
		CityID:    1,
		IsDefault: false,
	}
	createdAddr, err := svc.CreateAddress(ctx, addr)
	if err != nil {
		t.Fatalf("CreateAddress failed: %v", err)
	}
	createdAddr.Address = "Updated Address"
	if err := svc.UpdateAddress(ctx, createdAddr); err != nil {
		t.Fatalf("UpdateAddress failed: %v", err)
	}

	// 3. UserBelongsToOrg & ResolvePermissions
	belongs, err := svc.UserBelongsToOrg(ctx, user.ID, 10)
	if err != nil || !belongs {
		t.Fatalf("UserBelongsToOrg failed: %v", err)
	}

	perms, err := svc.ResolvePermissions(ctx, user.ID, 10)
	if err != nil {
		t.Fatalf("ResolvePermissions failed: %v", err)
	}
	_ = perms

	// 4. Logout without session store is a no-op
	if err := svc.Logout(ctx, "nonexistent"); err != nil {
		t.Fatalf("Logout failed: %v", err)
	}
	_, err = svc.ValidateSession(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error validating session without store")
	}

	// 5. GetMe with activeOrgID
	orgID := int64(10)
	me, err := svc.GetMe(ctx, user.ID, &orgID)
	if err != nil {
		t.Fatalf("GetMe failed: %v", err)
	}
	if me.ActiveOrgID == nil || *me.ActiveOrgID != 10 {
		t.Errorf("got active org %v, want 10", me.ActiveOrgID)
	}

	// 6. Favorite validations
	if err := svc.AddFavorite(ctx, user.ID, 0); err == nil {
		t.Error("expected error adding favorite with 0 productID, got nil")
	}
}

func TestServiceValidationAndDomainEdgeCases(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(repo, nil, logger)

	// Invalid registration
	_, _, err := svc.Register(ctx, RegisterInput{Email: "", Password: "pass"})
	if err == nil {
		t.Error("expected error on empty email")
	}
	_, _, err = svc.Register(ctx, RegisterInput{Email: "invalid-email", Password: "pass"})
	if err == nil {
		t.Error("expected error on invalid email")
	}
	_, _, err = svc.Register(ctx, RegisterInput{Email: "valid@example.com", Password: ""})
	if err == nil {
		t.Error("expected error on empty password")
	}

	// Password checking edge cases
	if CheckPassword("", "password") {
		t.Error("expected CheckPassword to return false for empty hash")
	}
	if CheckPassword("hash", "") {
		t.Error("expected CheckPassword to return false for empty password")
	}

	// Status validation
	now := time.Now()
	u := &User{Status: StatusSuspended}
	if err := u.ValidateLogin(); err == nil || apperr.KindOf(err) != apperr.KindForbidden {
		t.Errorf("expected Forbidden on suspended status, got %v", err)
	}
	u.Status = StatusPending
	if err := u.ValidateLogin(); err == nil || apperr.KindOf(err) != apperr.KindForbidden {
		t.Errorf("expected Forbidden on pending status, got %v", err)
	}
	u.Status = StatusInactive
	if err := u.ValidateLogin(); err == nil || apperr.KindOf(err) != apperr.KindForbidden {
		t.Errorf("expected Forbidden on inactive status, got %v", err)
	}
	u.Status = "unknown"
	if err := u.ValidateLogin(); err == nil || apperr.KindOf(err) != apperr.KindUnauthorized {
		t.Errorf("expected Unauthorized on unknown status, got %v", err)
	}
	u.DeletedAt = &now
	if err := u.ValidateLogin(); err == nil || apperr.KindOf(err) != apperr.KindNotFound {
		t.Errorf("expected NotFound on deleted user, got %v", err)
	}

	// MFA Login handling
	regUser, _, err := svc.Register(ctx, RegisterInput{
		Email:    "mfauser@example.com",
		Password: "Password123!",
		NameAr:   "مستخدم",
		NameEn:   "User",
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	_ = repo.UpsertMFA(ctx, &UserMFA{
		UserID:     regUser.ID,
		Enabled:    true,
		TOTPSecret: []byte("JBSWY3DPEHPK3PXP"),
	})
	loginRes, err := svc.Login(ctx, LoginInput{
		Email:    "mfauser@example.com",
		Password: "Password123!",
		OrgID:    10,
	})
	if err != nil {
		t.Fatalf("Login with MFA enabled failed: %v", err)
	}
	if !loginRes.RequiresMFA {
		t.Error("expected RequiresMFA to be true")
	}
}

func TestSessionStore_Unit(t *testing.T) {
	ctx := context.Background()

	// 1. GenerateToken
	tok, err := GenerateToken()
	if err != nil || len(tok) != 64 {
		t.Fatalf("GenerateToken failed or wrong length: %v, tok=%s", err, tok)
	}

	// 2. Empty SessionStore methods (disconnected cache)
	store := NewSessionStore(nil, config.Session{
		CookieName: "dawa_sess",
		TTL:        24 * time.Hour,
		SecureOnly: false,
	})

	if _, err := store.Get(ctx, ""); err == nil {
		t.Error("expected unauthorized on empty token")
	}
	if err := store.Delete(ctx, ""); err != nil {
		t.Errorf("expected nil error on empty token delete, got %v", err)
	}

	// In-memory fallback operations when cache is nil
	sess := &Session{UserID: 1, Token: tok}
	if err := store.Create(ctx, sess); err != nil {
		t.Errorf("expected successful in-memory Create with nil cache, got %v", err)
	}

	fetched, err := store.Get(ctx, tok)
	if err != nil || fetched == nil || fetched.UserID != 1 {
		t.Errorf("expected successful Get from in-memory fallback, got err=%v, sess=%v", err, fetched)
	}

	list, err := store.ListForUser(ctx, 1)
	if err != nil || len(list) != 1 {
		t.Errorf("expected 1 session in ListForUser, got %d, err=%v", len(list), err)
	}

	if err := store.Delete(ctx, tok); err != nil {
		t.Errorf("expected nil error on Delete, got %v", err)
	}

	if _, err := store.Get(ctx, tok); err == nil {
		t.Error("expected unauthorized on deleted token")
	}
}

func TestParseUserAgentDevice(t *testing.T) {
	cases := []struct {
		ua       string
		wantType string
		wantOS   string
		wantBr   string
	}{
		{
			ua:       "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			wantType: "desktop",
			wantOS:   "Windows",
			wantBr:   "Chrome",
		},
		{
			ua:       "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1",
			wantType: "mobile",
			wantOS:   "iOS (iPhone)",
			wantBr:   "Safari",
		},
		{
			ua:       "Mozilla/5.0 (Linux; Android 14; SM-S918B) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36",
			wantType: "mobile",
			wantOS:   "Android",
			wantBr:   "Chrome",
		},
		{
			ua:       "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.0 Safari/605.1.15",
			wantType: "desktop",
			wantOS:   "macOS",
			wantBr:   "Safari",
		},
	}

	for _, tc := range cases {
		det := ParseUserAgentDevice(tc.ua, "127.0.0.1")
		if det.DeviceType != tc.wantType {
			t.Errorf("ua=%q: got type %s, want %s", tc.ua, det.DeviceType, tc.wantType)
		}
		if det.OS != tc.wantOS {
			t.Errorf("ua=%q: got os %s, want %s", tc.ua, det.OS, tc.wantOS)
		}
		if det.Browser != tc.wantBr {
			t.Errorf("ua=%q: got browser %s, want %s", tc.ua, det.Browser, tc.wantBr)
		}
		if det.DeviceName == "" {
			t.Errorf("ua=%q: expected non-empty device name", tc.ua)
		}
	}
}
