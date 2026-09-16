package identity

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

func TestValidateSession_AutoLogoutDeletedUser(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := NewSessionStore(nil, config.Session{
		CookieName: "dawa_sess",
		TTL:        24 * time.Hour,
	})
	svc := NewService(repo, store, logger)
	svc.SetUserCacheTTL(0)

	// 1. Create active user
	user := &User{
		ID:     101,
		Email:  "active@example.com",
		Role:   RoleVendor,
		Status: StatusActive,
	}
	repo.users[user.ID] = user
	repo.usersByMail[user.Email] = user

	// 2. Create valid session
	tok := "test_token_12345"
	sess := &Session{
		UserID: user.ID,
		Token:  tok,
		Role:   user.Role,
	}
	if err := store.Create(ctx, sess); err != nil {
		t.Fatalf("Session Create failed: %v", err)
	}

	// 3. Verify session is valid while user is active
	validated, err := svc.ValidateSession(ctx, tok)
	if err != nil || validated == nil {
		t.Fatalf("expected valid session, got err: %v", err)
	}
	if validated.UserID != user.ID {
		t.Errorf("expected user ID %d, got %d", user.ID, validated.UserID)
	}

	// 4. Soft-delete the user
	now := time.Now()
	user.DeletedAt = &now
	_ = repo.UpdateUser(ctx, user)

	// 5. Verify ValidateSession now fails with Unauthorized and purges the session
	_, err = svc.ValidateSession(ctx, tok)
	if err == nil {
		t.Fatal("expected unauthorized error for soft-deleted user, got nil")
	}
	var appErr *apperr.Error
	if !errors.As(err, &appErr) || appErr.Kind != apperr.KindUnauthorized {
		t.Errorf("expected KindUnauthorized, got: %v", err)
	}

	// 6. Verify session was purged from store
	_, err = store.Get(ctx, tok)
	if err == nil {
		t.Error("expected session to be purged from sessionStore, but it still exists")
	}

	// 7. Test ValidateSessionWithoutTouch with suspended user
	user.DeletedAt = nil
	user.Status = StatusSuspended
	_ = repo.UpdateUser(ctx, user)

	tok2 := "test_token_67890"
	sess2 := &Session{
		UserID: user.ID,
		Token:  tok2,
		Role:   user.Role,
	}
	if err := store.Create(ctx, sess2); err != nil {
		t.Fatalf("Session Create 2 failed: %v", err)
	}

	_, err = svc.ValidateSessionWithoutTouch(ctx, tok2)
	if err == nil {
		t.Fatal("expected unauthorized error for suspended user, got nil")
	}
	if !errors.As(err, &appErr) || appErr.Kind != apperr.KindUnauthorized {
		t.Errorf("expected KindUnauthorized for suspended user, got: %v", err)
	}
	_, err = store.Get(ctx, tok2)
	if err == nil {
		t.Error("expected session2 to be purged from sessionStore")
	}
}
