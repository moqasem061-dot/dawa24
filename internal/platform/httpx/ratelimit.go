package httpx

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Limiter provides Redis-backed rate limiting middlewares.
type Limiter struct {
	rdb    *redis.Client
	rdbFn  func() *redis.Client
	prefix string
	userFn func(r *http.Request) int64
}

// NewLimiter creates a new rate limiter instance.
func NewLimiter(rdb *redis.Client, prefix string) *Limiter {
	if prefix == "" {
		prefix = "dawa24:ratelimit:"
	}
	return &Limiter{rdb: rdb, prefix: prefix}
}

// NewLazyLimiter creates a rate limiter backed by a dynamic Redis resolver.
func NewLazyLimiter(rdbFn func() *redis.Client, prefix string) *Limiter {
	if prefix == "" {
		prefix = "dawa24:ratelimit:"
	}
	return &Limiter{rdbFn: rdbFn, prefix: prefix}
}

// SetUserExtractor sets a custom resolver for extracting authenticated user IDs.
func (l *Limiter) SetUserExtractor(fn func(r *http.Request) int64) {
	if l != nil {
		l.userFn = fn
	}
}

func (l *Limiter) client() *redis.Client {
	if l == nil {
		return nil
	}
	if l.rdbFn != nil {
		return l.rdbFn()
	}
	return l.rdb
}

// LimitByIP creates a middleware that restricts requests per client IP address.
func (l *Limiter) LimitByIP(limit int, window time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rdb := l.client()
			if rdb == nil {
				next.ServeHTTP(w, r)
				return
			}

			ip := ClientIP(r, 1)
			key := fmt.Sprintf("%sip:%s", l.prefix, ip)

			allowed, count, ttl, err := l.allow(r.Context(), key, limit, window)
			setRateLimitHeaders(w, limit, count, ttl)
			if err != nil || !allowed {
				if ttl > 0 {
					w.Header().Set("Retry-After", strconv.FormatInt(int64(max(1, int(ttl.Seconds()))), 10))
				}
				Error(w, r, nil, apperr.New(apperr.KindRateLimited, "rate_limit_exceeded", "Too many requests. Please try again later."))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// LimitByOrg creates a middleware that restricts requests per tenant organization.
func (l *Limiter) LimitByOrg(limit int, window time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rdb := l.client()
			if rdb == nil {
				next.ServeHTTP(w, r)
				return
			}

			orgID, ok := database.TenantFrom(r.Context())
			if !ok {
				orgID = 0
			}

			key := fmt.Sprintf("%sorg:%d", l.prefix, orgID)
			allowed, count, ttl, err := l.allow(r.Context(), key, limit, window)
			setRateLimitHeaders(w, limit, count, ttl)
			if err != nil || !allowed {
				if ttl > 0 {
					w.Header().Set("Retry-After", strconv.FormatInt(int64(max(1, int(ttl.Seconds()))), 10))
				}
				Error(w, r, nil, apperr.New(apperr.KindRateLimited, "org_rate_limit_exceeded", "Organization rate limit exceeded. Please try again later."))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// LimitUserOrIP provides dynamic tiered rate limiting for backend APIs:
// - Authenticated users are metered per user ID with userLimit (e.g. 240 req/min).
// - Unauthenticated callers are metered per client IP with ipLimit (e.g. 60 req/min).
// Calibrated to comfortably support fast-paced pharmacists and legitimate API consumers while
// instantly blocking automated scrapers, flood scripts, and abusive spammers.
func (l *Limiter) LimitUserOrIP(userLimit, ipLimit int, window time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rdb := l.client()
			if rdb == nil {
				next.ServeHTTP(w, r)
				return
			}

			var key string
			limit := ipLimit

			var userID int64
			if l.userFn != nil {
				userID = l.userFn(r)
			}

			if userID > 0 {
				key = fmt.Sprintf("%suser:%d", l.prefix, userID)
				limit = userLimit
			} else if orgID, ok := database.TenantFrom(r.Context()); ok && orgID > 0 {
				key = fmt.Sprintf("%sorg:%d", l.prefix, orgID)
				limit = userLimit
			} else {
				ip := ClientIP(r, 1)
				key = fmt.Sprintf("%sip:%s", l.prefix, ip)
				limit = ipLimit
			}

			allowed, count, ttl, err := l.allow(r.Context(), key, limit, window)
			setRateLimitHeaders(w, limit, count, ttl)
			if err != nil || !allowed {
				if ttl > 0 {
					w.Header().Set("Retry-After", strconv.FormatInt(int64(max(1, int(ttl.Seconds()))), 10))
				}
				Error(w, r, nil, apperr.New(apperr.KindRateLimited, "rate_limit_exceeded", "Too many requests. Please slow down and try again."))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func (l *Limiter) allow(ctx context.Context, key string, limit int, window time.Duration) (bool, int64, time.Duration, error) {
	rdb := l.client()
	if rdb == nil {
		return true, 0, 0, nil
	}
	count, err := rdb.Incr(ctx, key).Result()
	if err != nil {
		return true, 0, 0, err // Fail open on Redis error so legitimate traffic is not dropped
	}
	if count == 1 {
		rdb.Expire(ctx, key, window)
	}
	ttl, _ := rdb.TTL(ctx, key).Result()
	if ttl < 0 {
		rdb.Expire(ctx, key, window)
		ttl = window
	}
	return count <= int64(limit), count, ttl, nil
}

func setRateLimitHeaders(w http.ResponseWriter, limit int, count int64, ttl time.Duration) {
	remaining := int64(limit) - count
	if remaining < 0 {
		remaining = 0
	}
	w.Header().Set("X-RateLimit-Limit", strconv.Itoa(limit))
	w.Header().Set("X-RateLimit-Remaining", strconv.FormatInt(remaining, 10))
	if ttl > 0 {
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(int64(ttl.Seconds()), 10))
	}
}

