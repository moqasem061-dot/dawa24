package identity

import (
	"context"
	"io"
	"log/slog"
	"testing"
)

// B9: a password reset no longer writes the stored login limit. The limit comes
// from the subscription plan, so the stored column is left as it was.
func TestResetPasswordDoesNotWriteTheStoredLoginLimit(t *testing.T) {
	repo := newMockRepo()
	existing := 7
	repo.usersByMail[NormalizeEmail("reset@dawa24.test")] = &User{ID: 42, Email: "reset@dawa24.test", PasswordHash: "old"}
	repo.securities[42] = &UserSecurity{UserID: 42, MaxLoginSessions: &existing}
	svc := NewService(repo, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))

	if err := svc.ResetPassword(context.Background(), "reset@dawa24.test", "a-new-password-1"); err != nil {
		t.Fatalf("reset password: %v", err)
	}
	sec := repo.securities[42]
	if sec == nil {
		t.Fatal("security row not written")
	}
	if sec.MaxLoginSessions != nil {
		t.Fatalf("password reset wrote the stored login limit %d, want it untouched (nil in the record handed to the repository)", *sec.MaxLoginSessions)
	}
}
