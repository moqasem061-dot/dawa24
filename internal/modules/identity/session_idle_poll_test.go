package identity

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/config"
)

// A page that polls in the background must not keep a session alive: idle means
// the user did nothing, not that the tab did nothing.
func TestGetWithoutTouchDoesNotDeferIdleTimeout(t *testing.T) {
	ctx := context.Background()
	store := NewSessionStore(nil, config.Session{
		CookieName:  "dawa24_session",
		TTL:         24 * time.Hour,
		IdleTimeout: 30 * time.Minute,
	})

	sess := &Session{UserID: 301, ActiveOrgID: 1, Role: "pharmacist"}
	if err := store.Create(ctx, sess); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// The user has been away for 25 minutes; the page has polled all along.
	store.memMu.Lock()
	store.memSessions[sess.Token].LastActiveAt = time.Now().UTC().Add(-25 * time.Minute)
	store.memMu.Unlock()

	for i := 0; i < 5; i++ {
		if _, err := store.GetWithoutTouch(ctx, sess.Token); err != nil {
			t.Fatalf("poll %d: session already gone: %v", i, err)
		}
	}

	// Those polls must not have moved the clock: five more minutes of absence
	// and the session is over the 30-minute limit.
	store.memMu.Lock()
	store.memSessions[sess.Token].LastActiveAt = time.Now().UTC().Add(-31 * time.Minute)
	store.memMu.Unlock()

	if _, err := store.GetWithoutTouch(ctx, sess.Token); !errors.Is(err, ErrSessionIdleTimeout) { //nolint:errorlint
		t.Fatalf("expected ErrSessionIdleTimeout after 31 idle minutes, got %v", err)
	}
}

// A request the user actually made does refresh the idle clock.
func TestGetTouchesLastActive(t *testing.T) {
	ctx := context.Background()
	store := NewSessionStore(nil, config.Session{
		CookieName:  "dawa24_session",
		TTL:         24 * time.Hour,
		IdleTimeout: 30 * time.Minute,
	})

	sess := &Session{UserID: 302, ActiveOrgID: 1, Role: "pharmacist"}
	if err := store.Create(ctx, sess); err != nil {
		t.Fatalf("Create: %v", err)
	}
	stale := time.Now().UTC().Add(-20 * time.Minute)
	store.memMu.Lock()
	store.memSessions[sess.Token].LastActiveAt = stale
	store.memMu.Unlock()

	if _, gErr := store.Get(ctx, sess.Token); gErr != nil {
		t.Fatalf("Get: %v", gErr)
	}

	store.memMu.Lock()
	got := store.memSessions[sess.Token].LastActiveAt
	store.memMu.Unlock()
	if !got.After(stale) {
		t.Fatalf("Get did not refresh LastActiveAt: still %v", got)
	}
}
