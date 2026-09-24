package identity

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
)

func newGuardTestService(t *testing.T) (*Service, *mockRepo) {
	t.Helper()
	repo := newMockRepo()
	return NewService(repo, nil, slog.New(slog.NewTextHandler(io.Discard, nil))), repo
}

func seedUser(t *testing.T, repo *mockRepo, email, role string) *User {
	t.Helper()
	u := &User{Email: email, Role: role, Status: StatusActive}
	if err := repo.CreateUser(context.Background(), u); err != nil {
		t.Fatalf("seed %s: %v", email, err)
	}
	return u
}

// A self-service registration must never produce a platform role, whether the
// role arrives in JSON (the public API) or in the Go struct.
func TestRegisterIgnoresClientRole(t *testing.T) {
	for _, role := range []string{"super_admin", "admin", "support", "developer", "moderator"} {
		svc, _ := newGuardTestService(t)
		var in RegisterInput
		body := `{"email":"x-` + role + `@example.com","password":"SecurePassword123!","role":"` + role +
			`","email_verified_at":"2020-01-01T00:00:00Z"}`
		if err := json.Unmarshal([]byte(body), &in); err != nil {
			t.Fatal(err)
		}
		in.Role = role // a server-side caller passing a staff role is refused too
		u, _, err := svc.Register(context.Background(), in)
		if err != nil {
			t.Fatalf("register %s: %v", role, err)
		}
		if u.Role != "user" {
			t.Errorf("role %q registered as %q; want user", role, u.Role)
		}
		if u.EmailVerifiedAt != nil {
			t.Errorf("email_verified_at from JSON was accepted for %q", role)
		}
	}
}

func TestRegisterKeepsJobSeeker(t *testing.T) {
	svc, _ := newGuardTestService(t)
	u, _, err := svc.Register(context.Background(), RegisterInput{
		Email: "seeker@example.com", Password: "SecurePassword123!", Role: RoleJobSeeker,
	})
	if err != nil || u.Role != RoleJobSeeker {
		t.Fatalf("job seeker register = %v, %v", u, err)
	}
}

// An administrator holding identity.user.update must not be able to take the
// owner account by resetting its password or second factor.
func TestAdminCannotTakeOverOwner(t *testing.T) {
	ctx := context.Background()
	svc, repo := newGuardTestService(t)
	owner := seedUser(t, repo, "owner@example.com", RoleSuperAdmin)
	admin := seedUser(t, repo, "admin@example.com", RoleAdmin)
	pharmacist := seedUser(t, repo, "ph@example.com", RolePharmacy)

	forbidden := func(name string, err error) {
		t.Helper()
		if apperr.KindOf(err) != apperr.KindForbidden {
			t.Errorf("%s: got %v; want Forbidden", name, err)
		}
	}
	forbidden("set owner password", svc.AdminSetPassword(ctx, owner.ID, admin.ID, "SuperSecret123!"))
	forbidden("reset owner mfa", svc.AdminResetMFA(ctx, owner.ID, admin.ID))
	forbidden("suspend owner", svc.AdminSuspendUser(ctx, owner.ID, admin.ID))
	forbidden("assign super_admin", svc.AdminAssignRole(ctx, pharmacist.ID, RoleSuperAdmin, admin.ID))
	forbidden("own role change", svc.AdminAssignRole(ctx, admin.ID, RoleSuperAdmin, admin.ID))

	// Ordinary accounts, including legacy roles, stay manageable by an admin.
	if err := svc.AdminSetPassword(ctx, pharmacist.ID, admin.ID, "SuperSecret123!"); err != nil {
		t.Errorf("admin resetting a pharmacist: %v", err)
	}
	// The owner may still manage staff.
	if err := svc.AdminResetMFA(ctx, admin.ID, owner.ID); err != nil {
		t.Errorf("owner resetting admin mfa: %v", err)
	}
}

// Recovery codes are stored hashed, still work once, and cannot be reused.
func TestRecoveryCodesStoredHashedAndSingleUse(t *testing.T) {
	ctx := context.Background()
	svc, repo := newGuardTestService(t)
	u := seedUser(t, repo, "mfa@example.com", "user")

	setup, err := svc.SetupMFA(ctx, u.ID, u.Email)
	if err != nil {
		t.Fatal(err)
	}
	code, err := GenerateTOTPCode(setup.Secret, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.ConfirmEnableMFA(ctx, u.ID, code)
	if err != nil || len(res.RecoveryCodes) == 0 {
		t.Fatalf("enable mfa: %v", err)
	}
	stored, _ := repo.GetMFA(ctx, u.ID)
	for _, c := range res.RecoveryCodes {
		if strings.Contains(string(stored.RecoveryCodes), c) {
			t.Fatalf("recovery code %q stored in plain text", c)
		}
	}
	if ok, err := svc.VerifyMFA(ctx, u.ID, res.RecoveryCodes[0]); !ok || err != nil {
		t.Fatalf("recovery code rejected: %v %v", ok, err)
	}
	if ok, _ := svc.VerifyMFA(ctx, u.ID, res.RecoveryCodes[0]); ok {
		t.Fatal("recovery code accepted twice")
	}
}
