package identity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	cachepkg "github.com/moqasem061-dot/dawa24/internal/platform/cache"
	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
)

// Get retrieves a session by token, enforces the idle inactivity timeout, and
// records the request as user activity.
func (s *SessionStore) Get(ctx context.Context, token string) (*Session, error) {
	return s.get(ctx, token, true)
}

// GetWithoutTouch is Get for a request the browser made on its own — a
// background poll, a heartbeat, a prefetch. It still enforces the idle timeout,
// but it does not push LastActiveAt forward.
//
// This is what makes the 30-minute idle logout actually fire. A logged-in tab
// polls the notification badge every 60 seconds whether or not anybody is at
// the keyboard; when that poll counted as activity, LastActiveAt was never more
// than a minute old and the session lived forever. Idle means "the user did
// nothing", not "the page did nothing".
func (s *SessionStore) GetWithoutTouch(ctx context.Context, token string) (*Session, error) {
	return s.get(ctx, token, false)
}

func (s *SessionStore) get(ctx context.Context, token string, touch bool) (*Session, error) {
	if token == "" {
		return nil, apperr.Unauthorized()
	}

	idleLimit := s.GetIdleTimeout()

	rdb, err := s.client()
	if err != nil {
		s.memMu.Lock()
		defer s.memMu.Unlock()
		sess, ok := s.memSessions[token]
		if !ok {
			if reason, evOk := s.memEvicted[token]; evOk {
				if reason == "duplicate_login" {
					return nil, ErrSessionEvictedDuplicateLogin
				}
				if reason == "concurrent_limit" || reason == "org_concurrent_limit" {
					return nil, ErrSessionEvictedConcurrentLimit
				}
				if reason == "idle_timeout" {
					return nil, ErrSessionIdleTimeout
				}
			}
			if s.cache != nil {
				return nil, err
			}
			return nil, apperr.Unauthorized()
		}

		lastActive := sess.LastActiveAt
		if lastActive.IsZero() {
			lastActive = sess.CreatedAt
		}

		// Enforce Idle Timeout
		if idleLimit > 0 && time.Since(lastActive) > idleLimit {
			delete(s.memSessions, token)
			if s.memEvicted == nil {
				s.memEvicted = make(map[string]string)
			}
			s.memEvicted[token] = "idle_timeout"
			if s.memUserSessions[sess.UserID] != nil {
				delete(s.memUserSessions[sess.UserID], token)
			}
			if sess.ActiveOrgID > 0 && s.memOrgSessions[sess.ActiveOrgID] != nil {
				delete(s.memOrgSessions[sess.ActiveOrgID], token)
			}
			return nil, ErrSessionIdleTimeout
		}

		// Debounced activity touch
		now := time.Now().UTC()
		if touch && now.Sub(lastActive) >= 15*time.Second {
			sess.LastActiveAt = now
		}
		copy := *sess
		copy.Permissions = append([]string(nil), sess.Permissions...)
		return &copy, nil
	}

	val, err := rdb.Get(ctx, sessionKey(token)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			// Check if this token was evicted due to duplicate login, concurrent session limit or idle timeout
			if reason, evErr := rdb.Get(ctx, sessionEvictedKey(token)).Result(); evErr == nil {
				if reason == "duplicate_login" {
					return nil, ErrSessionEvictedDuplicateLogin
				}
				if reason == "concurrent_limit" || reason == "org_concurrent_limit" {
					return nil, ErrSessionEvictedConcurrentLimit
				}
				if reason == "idle_timeout" {
					return nil, ErrSessionIdleTimeout
				}
			}
			return nil, apperr.Unauthorized()
		}
		return nil, apperr.Unavailable("redis", err)
	}

	var sess Session
	if err := json.Unmarshal(val, &sess); err != nil {
		return nil, fmt.Errorf("session: unmarshal: %w", err)
	}

	lastActive := sess.LastActiveAt
	if lastActive.IsZero() {
		lastActive = sess.CreatedAt
	}

	// Enforce Idle Timeout
	if idleLimit > 0 && time.Since(lastActive) > idleLimit {
		deleted, err := expireRedisSession(ctx, rdb, &sess, val)
		if err != nil {
			return nil, err
		}
		if !deleted {
			return nil, apperr.Unavailable("session", errors.New("session changed during idle check; retry"))
		}
		return nil, ErrSessionIdleTimeout
	}

	// Debounced activity update in Redis
	now := time.Now().UTC()
	if touch && now.Sub(lastActive) >= 15*time.Second {
		sess.LastActiveAt = now
		if err := touchRedisSession(ctx, rdb, &sess, val); err != nil {
			return nil, err
		}
	}

	return &sess, nil
}

// Delete invalidates a session token.
func (s *SessionStore) Delete(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}

	rdb, err := s.client()
	if err != nil {
		s.memMu.Lock()
		defer s.memMu.Unlock()
		if sess, ok := s.memSessions[token]; ok {
			delete(s.memSessions, token)
			if s.memUserSessions[sess.UserID] != nil {
				delete(s.memUserSessions[sess.UserID], token)
			}
			if sess.ActiveOrgID > 0 && s.memOrgSessions[sess.ActiveOrgID] != nil {
				delete(s.memOrgSessions[sess.ActiveOrgID], token)
			}
		}
		delete(s.memEvicted, token)
		return nil
	}

	// Deletion must not validate or touch the session: expiry calls this path too.
	blob, err := rdb.Get(ctx, sessionKey(token)).Bytes()
	if err != nil && !errors.Is(err, redis.Nil) {
		return apperr.Unavailable("redis", err)
	}
	if errors.Is(err, redis.Nil) {
		return nil
	}
	var sess Session
	if err := json.Unmarshal(blob, &sess); err != nil {
		return fmt.Errorf("session: unmarshal for deletion: %w", err)
	}
	return deleteRedisSession(ctx, rdb, &sess)
}

// DeleteAllForUser invalidates all active sessions for a given user.
func (s *SessionStore) DeleteAllForUser(ctx context.Context, userID int64) error {
	rdb, err := s.client()
	if err != nil {
		s.memMu.Lock()
		defer s.memMu.Unlock()
		if set, ok := s.memUserSessions[userID]; ok {
			for tok := range set {
				delete(s.memSessions, tok)
				delete(s.memEvicted, tok)
			}
			delete(s.memUserSessions, userID)
		}
		return nil
	}
	tokens, err := rdb.SMembers(ctx, userSessionsKey(userID)).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return apperr.Unavailable("redis", err)
	}

	if len(tokens) > 0 {
		keys := make([]string, 0, len(tokens)+1)
		for _, tok := range tokens {
			keys = append(keys, sessionKey(tok))
		}
		keys = append(keys, userSessionsKey(userID))
		if err := rdb.Del(ctx, keys...).Err(); err != nil {
			return apperr.Unavailable("redis", err)
		}
	}
	return nil
}

// DeleteAllOtherForOrg revokes all active sessions for an organization EXCEPT the given current token.
func (s *SessionStore) DeleteAllOtherForOrg(ctx context.Context, orgID int64, currentToken string) error {
	rdb, err := s.client()
	if err != nil {
		s.memMu.Lock()
		defer s.memMu.Unlock()
		if set, ok := s.memOrgSessions[orgID]; ok {
			for tok := range set {
				if tok != currentToken {
					if sess, ok := s.memSessions[tok]; ok {
						if s.memUserSessions[sess.UserID] != nil {
							delete(s.memUserSessions[sess.UserID], tok)
						}
					}
					delete(s.memSessions, tok)
					delete(set, tok)
				}
			}
		}
		return nil
	}
	tokens, err := rdb.SMembers(ctx, orgSessionsKey(orgID)).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return apperr.Unavailable("redis", err)
	}

	for _, tok := range tokens {
		if tok != currentToken && tok != "" {
			_ = s.Delete(ctx, tok)
		}
	}
	return nil
}

// DeleteAllOtherForUser revokes all active sessions for a user EXCEPT the given current token.
func (s *SessionStore) DeleteAllOtherForUser(ctx context.Context, userID int64, currentToken string) error {
	rdb, err := s.client()
	if err != nil {
		s.memMu.Lock()
		defer s.memMu.Unlock()
		if set, ok := s.memUserSessions[userID]; ok {
			for tok := range set {
				if tok != currentToken {
					if sess, ok := s.memSessions[tok]; ok {
						if sess.ActiveOrgID > 0 && s.memOrgSessions[sess.ActiveOrgID] != nil {
							delete(s.memOrgSessions[sess.ActiveOrgID], tok)
						}
					}
					delete(s.memSessions, tok)
					delete(set, tok)
				}
			}
		}
		return nil
	}
	tokens, err := rdb.SMembers(ctx, userSessionsKey(userID)).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return apperr.Unavailable("redis", err)
	}

	for _, tok := range tokens {
		if tok != currentToken && tok != "" {
			_ = s.Delete(ctx, tok)
		}
	}
	return nil
}

// client resolves the live Redis client, or reports that sessions are
// unavailable because the cache has not connected yet.
func (s *SessionStore) client() (*redis.Client, error) {
	if s == nil || s.cache == nil {
		return nil, apperr.Unavailable("session", cachepkg.ErrNotConnected)
	}
	rdb := s.cache.Redis()
	if rdb == nil {
		return nil, apperr.Unavailable("session", cachepkg.ErrNotConnected)
	}
	return rdb, nil
}
