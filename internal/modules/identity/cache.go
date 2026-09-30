package identity

import (
	"context"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
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
func (s *Service) isUserActive(ctx context.Context, userID int64) (bool, error) {
	if s.repo == nil {
		return true, nil
	}
	s.userCacheMu.RLock()
	ttl := s.userCacheTTL
	s.userCacheMu.RUnlock()

	if ttl <= 0 {
		return s.loadUserActive(ctx, userID)
	}

	now := time.Now()

	s.userCacheMu.RLock()
	entry, ok := s.userCache[userID]
	s.userCacheMu.RUnlock()

	if ok && now.Before(entry.expiresAt) {
		return entry.active, nil
	}

	active, err := s.loadUserActive(ctx, userID)
	if err != nil {
		return false, err
	}

	s.userCacheMu.Lock()
	if len(s.userCache) >= userCacheMaxEntries {
		for k, v := range s.userCache {
			if now.After(v.expiresAt) {
				delete(s.userCache, k)
			}
		}
		if len(s.userCache) >= userCacheMaxEntries {
			for k := range s.userCache {
				delete(s.userCache, k)
				break
			}
		}
	}
	s.userCache[userID] = userCacheEntry{
		active:    active,
		expiresAt: now.Add(ttl),
	}
	s.userCacheMu.Unlock()

	return active, nil
}

func (s *Service) loadUserActive(ctx context.Context, userID int64) (bool, error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		if apperr.KindOf(err) == apperr.KindNotFound {
			return false, nil
		}
		return false, apperr.Unavailable("database", err)
	}
	return user != nil && user.DeletedAt == nil && user.Status == StatusActive, nil
}

// InvalidateUserCache clears the cached active status for a user.
func (s *Service) InvalidateUserCache(userID int64) {
	s.userCacheMu.Lock()
	delete(s.userCache, userID)
	s.userCacheMu.Unlock()
}
