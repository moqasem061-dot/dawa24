package identity

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	cachepkg "github.com/moqasem061-dot/dawa24/internal/platform/cache"
	"github.com/moqasem061-dot/dawa24/internal/platform/config"
)

func redisSessionStore(t *testing.T) *SessionStore {
	t.Helper()
	url := os.Getenv("TEST_REDIS_URL")
	if url == "" {
		t.Skip("set TEST_REDIS_URL to run Redis session regressions")
	}
	c, err := cachepkg.Open(context.Background(), config.Redis{URL: url, PoolSize: 5}, config.Env("test"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Error(err)
		}
	})
	return NewSessionStore(c, config.Session{TTL: time.Hour, IdleTimeout: 30 * time.Minute})
}

func createRedisTestSession(t *testing.T, store *SessionStore) *Session {
	t.Helper()
	sess := &Session{UserID: time.Now().UnixNano(), ActiveOrgID: time.Now().UnixNano()}
	if err := store.Create(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	rdb := store.cache.Redis()
	t.Cleanup(func() {
		if err := rdb.Del(context.Background(), sessionKey(sess.Token), sessionEvictedKey(sess.Token), userSessionsKey(sess.UserID), orgSessionsKey(sess.ActiveOrgID)).Err(); err != nil {
			t.Error(err)
		}
	})
	return sess
}

func TestRedisIdleExpiryAndDeletion(t *testing.T) {
	store := redisSessionStore(t)
	for _, operation := range []string{"get", "background", "delete"} {
		t.Run(operation, func(t *testing.T) {
			sess := createRedisTestSession(t, store)
			sess.LastActiveAt = time.Now().Add(-31 * time.Minute)
			blob, err := json.Marshal(sess)
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			rdb := store.cache.Redis()
			if err := rdb.Set(ctx, sessionKey(sess.Token), blob, time.Hour).Err(); err != nil {
				t.Fatal(err)
			}
			switch operation {
			case "delete":
				if err := store.Delete(ctx, sess.Token); err != nil {
					t.Fatal(err)
				}
			default:
				get := store.Get
				if operation == "background" {
					get = store.GetWithoutTouch
				}
				for i := 0; i < 2; i++ {
					if _, err := get(ctx, sess.Token); !errors.Is(err, ErrSessionIdleTimeout) {
						t.Fatalf("expected idle timeout, got %v", err)
					}
				}
			}
			if exists := rdb.Exists(ctx, sessionKey(sess.Token)).Val(); exists != 0 {
				t.Fatal("expired session still exists")
			}
			for _, key := range []string{userSessionsKey(sess.UserID), orgSessionsKey(sess.ActiveOrgID)} {
				if rdb.SIsMember(ctx, key, sess.Token).Val() {
					t.Fatalf("expired token left in %s", key)
				}
			}
		})
	}
}

func TestRedisTouchPreservesExpiryAndRevocation(t *testing.T) {
	store := redisSessionStore(t)
	sess := createRedisTestSession(t, store)
	ctx := context.Background()
	rdb := store.cache.Redis()
	original, err := rdb.Get(ctx, sessionKey(sess.Token)).Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if err := rdb.PExpire(ctx, sessionKey(sess.Token), 2*time.Minute).Err(); err != nil {
		t.Fatal(err)
	}
	sess.LastActiveAt = time.Now().Add(time.Second)
	if err := touchRedisSession(ctx, rdb, sess, original); err != nil {
		t.Fatal(err)
	}
	ttl := rdb.PTTL(ctx, sessionKey(sess.Token)).Val()
	if ttl <= 0 || ttl > 2*time.Minute {
		t.Fatalf("touch changed expiry: %v", ttl)
	}
	updated, err := rdb.Get(ctx, sessionKey(sess.Token)).Bytes()
	if err != nil {
		t.Fatal(err)
	}
	sess.LastActiveAt = sess.LastActiveAt.Add(-time.Minute)
	if err := touchRedisSession(ctx, rdb, sess, original); err != nil {
		t.Fatal(err)
	}
	if got := rdb.Get(ctx, sessionKey(sess.Token)).Val(); got != string(updated) {
		t.Fatal("stale touch overwrote newer activity")
	}
	if err := store.Delete(ctx, sess.Token); err != nil {
		t.Fatal(err)
	}
	if err := touchRedisSession(ctx, rdb, sess, updated); err != nil {
		t.Fatal(err)
	}
	if rdb.Exists(ctx, sessionKey(sess.Token)).Val() != 0 {
		t.Fatal("touch resurrected a revoked session")
	}
}

func TestRedisIdleCheckPreservesConcurrentActivity(t *testing.T) {
	store := redisSessionStore(t)
	sess := createRedisTestSession(t, store)
	ctx := context.Background()
	rdb := store.cache.Redis()
	sess.LastActiveAt = time.Now().Add(-31 * time.Minute)
	original, err := json.Marshal(sess)
	if err != nil {
		t.Fatal(err)
	}
	if err := rdb.Set(ctx, sessionKey(sess.Token), original, time.Hour).Err(); err != nil {
		t.Fatal(err)
	}
	sess.LastActiveAt = time.Now()
	if err := touchRedisSession(ctx, rdb, sess, original); err != nil {
		t.Fatal(err)
	}
	deleted, err := expireRedisSession(ctx, rdb, sess, original)
	if err != nil || deleted {
		t.Fatalf("stale idle check deleted fresh activity: deleted=%v, err=%v", deleted, err)
	}
	if _, err := store.GetWithoutTouch(ctx, sess.Token); err != nil {
		t.Fatalf("active session lost: %v", err)
	}
}
