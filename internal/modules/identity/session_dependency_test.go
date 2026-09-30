package identity

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strconv"
	"testing"
	"time"

	cachepkg "github.com/moqasem061-dot/dawa24/internal/platform/cache"
	"github.com/moqasem061-dot/dawa24/internal/platform/config"
	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
)

type sessionLookupRepo struct {
	Repository
	lookupErr error
}

func (r *sessionLookupRepo) GetUserByID(context.Context, int64) (*User, error) {
	return &User{ID: 101, Status: StatusActive}, r.lookupErr
}

func TestSessionSurvivesUserLookupFailure(t *testing.T) {
	for _, ttl := range []time.Duration{0, 30 * time.Second} {
		for _, background := range []bool{false, true} {
			t.Run(ttl.String()+"/background="+strconv.FormatBool(background), func(t *testing.T) {
				ctx := context.Background()
				store := NewSessionStore(nil, config.Session{TTL: time.Hour})
				sess := &Session{UserID: 101}
				if err := store.Create(ctx, sess); err != nil {
					t.Fatal(err)
				}
				repo := &sessionLookupRepo{lookupErr: errors.New("temporary database failure")}
				svc := NewService(repo, store, slog.New(slog.NewTextHandler(io.Discard, nil)))
				svc.SetUserCacheTTL(ttl)
				validate := svc.ValidateSession
				if background {
					validate = svc.ValidateSessionWithoutTouch
				}
				if _, err := validate(ctx, sess.Token); apperr.KindOf(err) != apperr.KindUnavailable {
					t.Fatalf("expected unavailable, got %v", err)
				}
				if _, err := store.GetWithoutTouch(ctx, sess.Token); err != nil {
					t.Fatalf("session revoked: %v", err)
				}
				repo.lookupErr = nil
				if _, err := validate(ctx, sess.Token); err != nil {
					t.Fatalf("failure cached as inactive: %v", err)
				}
			})
		}
	}
}

func TestDisconnectedSessionStoreIsUnavailable(t *testing.T) {
	store := NewSessionStore(cachepkg.New(config.Env("test")), config.Session{TTL: time.Hour})
	if _, err := store.Get(context.Background(), "existing-redis-session"); apperr.KindOf(err) != apperr.KindUnavailable {
		t.Fatalf("expected unavailable rather than logout, got %v", err)
	}
}

func TestUserActiveCacheStaysBounded(t *testing.T) {
	svc := NewService(&sessionLookupRepo{}, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	for id := int64(1); id <= userCacheMaxEntries+10; id++ {
		if _, err := svc.isUserActive(context.Background(), id); err != nil {
			t.Fatal(err)
		}
	}
	if len(svc.userCache) > userCacheMaxEntries {
		t.Fatalf("cache grew to %d", len(svc.userCache))
	}
}
