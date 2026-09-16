package identity

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	cachepkg "github.com/muhiya/dawa24-store/internal/platform/cache"
	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// redisCmdable is the slice of the Redis client these helpers need. Naming it
// keeps the enforcement functions testable without a live server.
type redisCmdable interface {
	Pipeline() redis.Pipeliner
	MGet(ctx context.Context, keys ...string) *redis.SliceCmd
}

// Session holds the active user session stored in Redis.
type Session struct {
	Token    string `json:"token"`
	UserID   int64  `json:"user_id"`
	PublicID string `json:"public_id"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	// StaffRole records whether the platform role this session was issued
	// under reaches the admin dashboard. It is written at login from
	// identity.roles.is_staff, so a role an operator invented after this code
	// was written is recognised as staff without a code change.
	StaffRole    bool      `json:"staff_role"`
	ActiveOrgID  int64     `json:"active_org_id,omitempty"`
	OrgType      string    `json:"org_type,omitempty"`
	OrgStatus    string    `json:"org_status,omitempty"`
	Permissions  []string  `json:"permissions"`
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at"`
	IP           string    `json:"ip,omitempty"`
	UserAgent    string    `json:"user_agent,omitempty"`
	DeviceName   string    `json:"device_name,omitempty"`
	DeviceType   string    `json:"device_type,omitempty"`
	Browser      string    `json:"browser,omitempty"`
	OS           string    `json:"os,omitempty"`
	Icon         string    `json:"icon,omitempty"`
	LastActiveAt time.Time `json:"last_active_at,omitempty"`
	// MaxLoginSessions, when set, is the concurrent-sign-in limit enforced by
	// SessionStore.Create (evicting the oldest session beyond the limit).
	MaxLoginSessions *int `json:"max_login_sessions,omitempty"`
}

// IsStaff reports whether this session belongs to platform staff. Platform
// admin is staff, not an account type (Rebuild V2 rule 1); an organization
// member's capability comes from the membership, never from the platform role.
//
// The answer comes from StaffRole, stamped at login from the role row. The
// four hardcoded names remain only as a fallback for sessions issued before
// that field existed — without it, every logged-in administrator would be
// bounced out of /admin the moment this version deployed.
func (s *Session) IsStaff() bool {
	if s.StaffRole {
		return true
	}
	return s.Role == "super_admin" || s.Role == "admin" || s.Role == "support" || s.Role == "developer"
}

// SessionStore handles session persistence in Redis.
// SessionStore holds the cache handle rather than a redis.Client.
//
// The client does not exist when routes are mounted — the server starts before
// its dependencies connect — so capturing one at construction time captures nil
// forever. Asking the handle at each use gets whatever is live now.
type SessionStore struct {
	cache       *cachepkg.Cache
	cookieName  string
	ttl         time.Duration
	idleTimeout time.Duration
	idleMu      sync.RWMutex
	secure      bool

	memMu           sync.RWMutex
	memSessions     map[string]*Session
	memUserSessions map[int64]map[string]bool
	memOrgSessions  map[int64]map[string]bool
	memEvicted      map[string]string
}

// NewSessionStore creates a session store wrapping Redis.
func NewSessionStore(c *cachepkg.Cache, cfg config.Session) *SessionStore {
	idle := cfg.IdleTimeout
	if idle <= 0 {
		idle = 30 * time.Minute
	}
	return &SessionStore{
		cache:           c,
		cookieName:      cfg.CookieName,
		ttl:             cfg.TTL,
		idleTimeout:     idle,
		secure:          cfg.SecureOnly,
		memSessions:     make(map[string]*Session),
		memUserSessions: make(map[int64]map[string]bool),
		memOrgSessions:  make(map[int64]map[string]bool),
		memEvicted:      make(map[string]string),
	}
}

// SetIdleTimeout dynamically sets the maximum inactivity duration allowed before a session is expired.
func (s *SessionStore) SetIdleTimeout(d time.Duration) {
	if s == nil {
		return
	}
	s.idleMu.Lock()
	defer s.idleMu.Unlock()
	if d > 0 {
		s.idleTimeout = d
	}
}

// GetIdleTimeout returns the active session idle timeout duration.
func (s *SessionStore) GetIdleTimeout() time.Duration {
	if s == nil {
		return 30 * time.Minute
	}
	s.idleMu.RLock()
	defer s.idleMu.RUnlock()
	if s.idleTimeout <= 0 {
		return 30 * time.Minute
	}
	return s.idleTimeout
}

// GenerateToken generates a cryptographically secure 32-byte hex token.
func GenerateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("session: generate token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func sessionKey(token string) string {
	return fmt.Sprintf("session:%s", token)
}

func sessionEvictedKey(token string) string {
	return fmt.Sprintf("session_evicted:%s", token)
}

func userSessionsKey(userID int64) string {
	return fmt.Sprintf("user_sessions:%d", userID)
}

func orgSessionsKey(orgID int64) string {
	return fmt.Sprintf("org_sessions:%d", orgID)
}

// Create stores a new session in Redis with configured TTL.
func (s *SessionStore) Create(ctx context.Context, sess *Session) error {
	if sess.Token == "" {
		token, err := GenerateToken()
		if err != nil {
			return err
		}
		sess.Token = token
	}

	sess.CreatedAt = time.Now().UTC()
	sess.LastActiveAt = sess.CreatedAt
	sess.ExpiresAt = sess.CreatedAt.Add(s.ttl)

	data, err := json.Marshal(sess)
	if err != nil {
		return fmt.Errorf("session: marshal: %w", err)
	}

	rdb, err := s.client()
	if err != nil {
		s.memMu.Lock()
		defer s.memMu.Unlock()
		if s.memSessions == nil {
			s.memSessions = make(map[string]*Session)
			s.memUserSessions = make(map[int64]map[string]bool)
			s.memOrgSessions = make(map[int64]map[string]bool)
			s.memEvicted = make(map[string]string)
		}
		if s.memEvicted == nil {
			s.memEvicted = make(map[string]string)
		}

		// 1. Strict single active session per user account:
		// Evict any prior session for this user account.
		if existingTokens, ok := s.memUserSessions[sess.UserID]; ok {
			for oldTok := range existingTokens {
				if oldTok != sess.Token {
					s.memEvicted[oldTok] = "duplicate_login"
					delete(s.memSessions, oldTok)
					delete(existingTokens, oldTok)
					if sess.ActiveOrgID > 0 && s.memOrgSessions[sess.ActiveOrgID] != nil {
						delete(s.memOrgSessions[sess.ActiveOrgID], oldTok)
					}
				}
			}
		}

		s.memSessions[sess.Token] = sess
		if s.memUserSessions[sess.UserID] == nil {
			s.memUserSessions[sess.UserID] = make(map[string]bool)
		}
		s.memUserSessions[sess.UserID][sess.Token] = true
		if sess.ActiveOrgID > 0 {
			if s.memOrgSessions[sess.ActiveOrgID] == nil {
				s.memOrgSessions[sess.ActiveOrgID] = make(map[string]bool)
			}
			s.memOrgSessions[sess.ActiveOrgID][sess.Token] = true

			// 2. Organization plan concurrency limit:
			if sess.MaxLoginSessions != nil && *sess.MaxLoginSessions > 0 {
				orgTokens := s.memOrgSessions[sess.ActiveOrgID]
				if len(orgTokens) > *sess.MaxLoginSessions {
					var oldestTok string
					var oldestTime time.Time
					for tok := range orgTokens {
						if tok == sess.Token {
							continue
						}
						sObj, exists := s.memSessions[tok]
						if !exists {
							delete(orgTokens, tok)
							continue
						}
						created := sObj.CreatedAt
						if created.IsZero() {
							created = sObj.LastActiveAt
						}
						if oldestTok == "" || created.Before(oldestTime) {
							oldestTok = tok
							oldestTime = created
						}
					}
					if oldestTok != "" {
						s.memEvicted[oldestTok] = "org_concurrent_limit"
						if sObj, exists := s.memSessions[oldestTok]; exists {
							if s.memUserSessions[sObj.UserID] != nil {
								delete(s.memUserSessions[sObj.UserID], oldestTok)
							}
						}
						delete(s.memSessions, oldestTok)
						delete(orgTokens, oldestTok)
					}
				}
			}
		}
		return nil
	}
	pipe := rdb.TxPipeline()
	pipe.Set(ctx, sessionKey(sess.Token), data, s.ttl)
	pipe.SAdd(ctx, userSessionsKey(sess.UserID), sess.Token)
	pipe.Expire(ctx, userSessionsKey(sess.UserID), s.ttl)
	if sess.ActiveOrgID > 0 {
		pipe.SAdd(ctx, orgSessionsKey(sess.ActiveOrgID), sess.Token)
		pipe.Expire(ctx, orgSessionsKey(sess.ActiveOrgID), s.ttl)
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return apperr.Unavailable("redis", err)
	}

	// Enforce the concurrent-sign-in limit:
	// 1. Strict single active session per user account:
	// If someone is already logged in on this user account, logging in from another
	// device/browser immediately evicts the older session and lets the new session in.
	if err := s.enforceLimit(ctx, sess.UserID, 1); err != nil {
		return err
	}

	// 2. Organization plan concurrency limit:
	// If the user belongs to an organization, enforce the subscription plan's maximum
	// concurrent session limit across distinct staff members in that organization.
	if sess.ActiveOrgID > 0 && sess.MaxLoginSessions != nil && *sess.MaxLoginSessions > 0 {
		if err := s.enforceOrgLimit(ctx, sess.ActiveOrgID, *sess.MaxLoginSessions); err != nil {
			return err
		}
	}
	return nil
}
