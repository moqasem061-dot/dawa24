package identity

import (
	"context"
	"time"
)

const userCacheMaxEntries = 4096

type userCacheEntry struct {
	active    bool
	expiresAt time.Time
}

// SetUserCacheTTL overrides the user active cache TTL (set to 0 in tests to disable).
func (s *Service) SetUserCacheTTL(d time.Duration) {
	s.userCacheMu.Lock()
	s.userCacheTTL = d
	s.userCacheMu.Unlock()
}

// isUserActive reports whether the user is active, using a short-lived cache (30s)
// to eliminate repetitive database hits during session validation on every request.
func (s *Service) isUserActive(ctx context.Context, userID int64) bool {
	if s.repo == nil {
		return true
	}
	s.userCacheMu.RLock()
	ttl := s.userCacheTTL
	s.userCacheMu.RUnlock()

	if ttl <= 0 {
		user, err := s.repo.GetUserByID(ctx, userID)
		return err == nil && user != nil && user.DeletedAt == nil && user.Status == StatusActive
	}

	now := time.Now()

	s.userCacheMu.RLock()
	entry, ok := s.userCache[userID]
	s.userCacheMu.RUnlock()

	if ok && now.Before(entry.expiresAt) {
		return entry.active
	}

	user, err := s.repo.GetUserByID(ctx, userID)
	active := err == nil && user != nil && user.DeletedAt == nil && user.Status == StatusActive

	s.userCacheMu.Lock()
	if len(s.userCache) >= userCacheMaxEntries {
		for k, v := range s.userCache {
			if now.After(v.expiresAt) {
				delete(s.userCache, k)
			}
		}
	}
	s.userCache[userID] = userCacheEntry{
		active:    active,
		expiresAt: now.Add(ttl),
	}
	s.userCacheMu.Unlock()

	return active
}

// InvalidateUserCache clears the cached active status for a user.
func (s *Service) InvalidateUserCache(userID int64) {
	s.userCacheMu.Lock()
	delete(s.userCache, userID)
	s.userCacheMu.Unlock()
}
