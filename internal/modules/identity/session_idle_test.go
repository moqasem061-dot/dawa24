package identity

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

func TestSessionIdleTimeout(t *testing.T) {
	ctx := context.Background()
	cfg := config.Session{
		CookieName:  "dawa24_session",
		TTL:         24 * time.Hour,
		IdleTimeout: 30 * time.Minute,
	}
	store := NewSessionStore(nil, cfg)

	// 1. Create a session
	sess := &Session{
		UserID:      101,
		ActiveOrgID: 1,
		Role:        "pharmacist",
	}
	err := store.Create(ctx, sess)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	tok := sess.Token

	// 2. Fetching session within idle timeout should succeed
	retrieved, err := store.Get(ctx, tok)
	if err != nil {
		t.Fatalf("expected session to be valid, got err: %v", err)
	}
	if retrieved.UserID != 101 {
		t.Errorf("expected UserID 101, got %d", retrieved.UserID)
	}

	// 3. Artificially age the session's LastActiveAt beyond the 30-minute idle limit
	store.memMu.Lock()
	store.memSessions[tok].LastActiveAt = time.Now().UTC().Add(-31 * time.Minute)
	store.memMu.Unlock()

	// 4. Fetching now must return ErrSessionIdleTimeout and evict session
	_, err = store.Get(ctx, tok)
	if err == nil {
		t.Fatalf("expected error for idle-timed-out session, got nil")
	}

	var appErr *apperr.Error
	if !errors.Is(err, ErrSessionIdleTimeout) && (!errors.As(err, &appErr) || appErr.Code != "session.idle_timeout") {
		t.Fatalf("expected ErrSessionIdleTimeout, got: %v", err)
	}

	// 5. Subsequent lookup returns unauthorized (session was evicted from store)
	_, err = store.Get(ctx, tok)
	if err == nil {
		t.Fatalf("expected session to be evicted, but got valid session")
	}

	// 6. Dynamic idle timeout reconfiguration
	sess2 := &Session{
		UserID:      102,
		ActiveOrgID: 1,
		Role:        "vendor",
	}
	err = store.Create(ctx, sess2)
	if err != nil {
		t.Fatalf("failed to create second session: %v", err)
	}
	tok2 := sess2.Token

	// Change idle timeout to 10 minutes
	store.SetIdleTimeout(10 * time.Minute)
	if store.GetIdleTimeout() != 10*time.Minute {
		t.Fatalf("expected idle timeout 10m, got %v", store.GetIdleTimeout())
	}

	// Age by 15 minutes (past 10m, but within previous 30m)
	store.memMu.Lock()
	store.memSessions[tok2].LastActiveAt = time.Now().UTC().Add(-15 * time.Minute)
	store.memMu.Unlock()

	_, err = store.Get(ctx, tok2)
	if err == nil {
		t.Fatalf("expected session to expire under new 10m idle limit")
	}
}
