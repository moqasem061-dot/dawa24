package identity

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
)

const batch5Password = "correct-horse-1"

func batch5Service(t *testing.T, status UserStatus) (*Service, *mockRepo) {
	t.Helper()
	hash, err := HashPassword(batch5Password)
	if err != nil {
		t.Fatal(err)
	}
	repo := newMockRepo()
	u := &User{ID: 7, Email: "pharmacy@dawa24.test", PasswordHash: hash, Status: status}
	repo.usersByMail[u.Email] = u
	repo.users[u.ID] = u
	return NewService(repo, nil, slog.New(slog.NewTextHandler(io.Discard, nil))), repo
}

// S18: a wrong password gets the same answer whatever the account status, and a
// correct password is the only way to learn it.
func TestLoginRevealsStatusOnlyAfterCorrectPassword(t *testing.T) {
	for _, status := range []UserStatus{StatusSuspended, StatusPending, StatusInactive} {
		svc, _ := batch5Service(t, status)
		_, err := svc.Login(context.Background(), LoginInput{Email: "pharmacy@dawa24.test", Password: "wrong-password-1"})
		if !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("status %s, wrong password: want invalid credentials, got %v", status, err)
		}

		_, err = svc.Login(context.Background(), LoginInput{Email: "pharmacy@dawa24.test", Password: batch5Password})
		appErr, ok := apperr.As(err)
		if !ok {
			t.Fatalf("status %s, correct password: want a status error, got %v", status, err)
		}
		want := map[UserStatus]string{StatusSuspended: "auth.suspended", StatusPending: "auth.pending", StatusInactive: "auth.inactive"}[status]
		if appErr.Code != want {
			t.Fatalf("status %s, correct password: want code %s, got %s", status, want, appErr.Code)
		}
	}
}

// S18: an unknown email is refused with the same error, and its check runs at
// the same bcrypt cost as a real account.
func TestLoginUnknownEmailUsesRealBcryptCost(t *testing.T) {
	svc, _ := batch5Service(t, StatusActive)
	_, err := svc.Login(context.Background(), LoginInput{Email: "nobody@dawa24.test", Password: batch5Password})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("unknown email: want invalid credentials, got %v", err)
	}
	if cost, err := bcrypt.Cost([]byte(unknownUserHash())); err != nil || cost != bcrypt.DefaultCost {
		t.Fatalf("dummy hash cost = %d (err %v), want %d", cost, err, bcrypt.DefaultCost)
	}
}

// S20: a correct password does not clear the second-factor counter, and the
// lock holds even for the right code.
func TestMFAGuessesStayCappedAcrossCorrectPasswordLogins(t *testing.T) {
	svc, repo := batch5Service(t, StatusActive)
	secret, err := GenerateTOTPSecret()
	if err != nil {
		t.Fatal(err)
	}
	repo.mfas[7] = &UserMFA{UserID: 7, Enabled: true, TOTPSecret: []byte(secret)}
	ctx := context.Background()

	login := func() {
		t.Helper()
		res, err := svc.Login(ctx, LoginInput{Email: "pharmacy@dawa24.test", Password: batch5Password})
		if err != nil || res == nil || !res.RequiresMFA {
			t.Fatalf("correct password should reach the MFA step, got res=%v err=%v", res, err)
		}
	}

	login()
	for i := 0; i < MaxFailedLoginsBeforeLockout-1; i++ {
		if ok, err := svc.VerifyMFA(ctx, 7, "000000"); ok || err != nil {
			t.Fatalf("wrong code %d: ok=%v err=%v", i+1, ok, err)
		}
		login() // a correct password between guesses must not reset the count
	}

	if ok, err := svc.VerifyMFA(ctx, 7, "000000"); ok || err != nil {
		t.Fatalf("fifth wrong code: ok=%v err=%v", ok, err)
	}
	code, err := GenerateTOTPCode(secret, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.VerifyMFA(ctx, 7, code); codeOf(err) != "mfa.locked" {
		t.Fatalf("locked MFA must refuse even the right code, got %v", err)
	}
}

func codeOf(err error) string {
	if appErr, ok := apperr.As(err); ok {
		return appErr.Code
	}
	return ""
}

// S21: a reset token works once. A password change after it was issued also
// makes it unusable.
func TestResetTokenIsSingleUse(t *testing.T) {
	svc, _ := batch5Service(t, StatusActive)
	ctx := context.Background()
	issuedAt := time.Now().Add(-time.Second)

	if err := svc.ResetPasswordWithToken(ctx, "pharmacy@dawa24.test", "first-new-pass-1", issuedAt); err != nil {
		t.Fatalf("first use: %v", err)
	}
	if err := svc.ResetPasswordWithToken(ctx, "pharmacy@dawa24.test", "second-new-pass-2", issuedAt); !errors.Is(err, ErrResetTokenSpent) {
		t.Fatalf("second use of the same token: want ErrResetTokenSpent, got %v", err)
	}
}

func TestResetTokenIssuedBeforePasswordChangeIsRefused(t *testing.T) {
	svc, _ := batch5Service(t, StatusActive)
	ctx := context.Background()
	issuedAt := time.Now().Add(-time.Second)

	if err := svc.ResetPassword(ctx, "pharmacy@dawa24.test", "settings-new-pass-1"); err != nil {
		t.Fatalf("password change in settings: %v", err)
	}
	if err := svc.ResetPasswordWithToken(ctx, "pharmacy@dawa24.test", "reset-new-pass-2", issuedAt); !errors.Is(err, ErrResetTokenSpent) {
		t.Fatalf("token issued before a password change: want ErrResetTokenSpent, got %v", err)
	}
}
