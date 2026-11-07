package identity

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
)

// Batch 3 (S07): enrolling again must not switch off a factor that is on.
func TestSetupMFARefusesWhenEnabled(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	svc := NewService(repo, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))

	user, _, err := svc.Register(ctx, RegisterInput{
		Email: "mfa-guard@dawa24.eg", Password: "InitialPassword123!", NameAr: "م", NameEn: "M", Role: "user",
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := repo.UpsertMFA(ctx, &UserMFA{UserID: user.ID, TOTPSecret: []byte("ACTIVE-SECRET"), Enabled: true}); err != nil {
		t.Fatalf("UpsertMFA: %v", err)
	}

	if _, err := svc.SetupMFA(ctx, user.ID, user.Email); apperr.KindOf(err) != apperr.KindConflict {
		t.Fatalf("SetupMFA on an enabled account: err=%v, want a conflict", err)
	}
	got := repo.mfas[user.ID]
	if !got.Enabled || string(got.TOTPSecret) != "ACTIVE-SECRET" {
		t.Fatalf("active factor changed: enabled=%v secret=%q", got.Enabled, got.TOTPSecret)
	}
}

func TestSetupMFAStillWorksWhenNotEnabled(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	svc := NewService(repo, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))

	user, _, err := svc.Register(ctx, RegisterInput{
		Email: "mfa-new@dawa24.eg", Password: "InitialPassword123!", NameAr: "م", NameEn: "M", Role: "user",
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	data, err := svc.SetupMFA(ctx, user.ID, user.Email)
	if err != nil {
		t.Fatalf("SetupMFA on a fresh account: %v", err)
	}
	if data.Secret == "" {
		t.Fatal("expected a pending secret")
	}
	if repo.mfas[user.ID].Enabled {
		t.Fatal("a pending enrolment must not be enabled")
	}
}

// Batch 3 (S06): a team member created with an administrator-chosen password
// is flagged, and the flag clears when that member sets their own password.
func TestMustChangePasswordClearsOnChange(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	svc := NewService(repo, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))

	user, _, err := svc.Register(ctx, RegisterInput{
		Email: "team@dawa24.eg", Password: "Chosen-By-Owner-1", NameAr: "م", NameEn: "M", Role: "user",
		MustChangePassword: true,
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if !repo.users[user.ID].MustChangePassword {
		t.Fatal("new team member must be flagged to change the password")
	}

	if err := svc.ChangePassword(ctx, user.ID, "Chosen-By-Owner-1", "Own-Password-2026"); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	if repo.users[user.ID].MustChangePassword {
		t.Fatal("flag must clear once the member sets their own password")
	}
}

// Batch 3 (S06): an account created without the flag, such as a self-registered
// one, is never held on the password page.
func TestRegisterWithoutFlagIsNotHeld(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	svc := NewService(repo, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))

	user, _, err := svc.Register(ctx, RegisterInput{
		Email: "self@dawa24.eg", Password: "Self-Chosen-Pass-1", NameAr: "م", NameEn: "M", Role: "user",
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if repo.users[user.ID].MustChangePassword {
		t.Fatal("self-registered account must not be flagged")
	}
}
