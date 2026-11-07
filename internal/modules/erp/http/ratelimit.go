package http

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/platform/httpx"
)

type limitHeaders struct {
	limit int
	count int64
	retry time.Duration
}

func failureKey(ip string) string { return "dawa24:erp:auth-fail:" + ip }
func checkAuthFailures(ctx context.Context, lim *httpx.Limiter, ip string) (limitHeaders, error) {
	count, ttl, err := lim.FailedAuthCount(ctx, failureKey(ip))
	return limitHeaders{limit: erp.AuthFailuresPerIPPerMinute, count: count, retry: ttl}, err
}
func checkKeyLimit(ctx context.Context, lim *httpx.Limiter, p erp.Principal) (limitHeaders, *erp.Error, error) {
	limit := p.RatePerMinute
	if limit <= 0 {
		limit = erp.DefaultRatePerMinute
	}
	allowed, count, ttl, err := lim.AllowKey(ctx, fmt.Sprintf("dawa24:erp:key:%d", p.KeyID), limit, time.Minute)
	h := limitHeaders{limit: limit, count: count, retry: ttl}
	if err == nil && !allowed {
		return h, erp.NewError("ERP_RATE_LIMITED", 429, map[string]any{"retryAfterSeconds": int64(ttl.Seconds()), "limitPerMinute": limit}), nil
	}
	return h, nil, err
}
func setLimitHeaders(w http.ResponseWriter, h limitHeaders) {
	remaining := int64(h.limit) - h.count
	if remaining < 0 {
		remaining = 0
	}
	w.Header().Set("X-RateLimit-Limit", strconv.Itoa(h.limit))
	w.Header().Set("X-RateLimit-Remaining", strconv.FormatInt(remaining, 10))
	if h.retry > 0 {
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(int64(h.retry.Seconds()), 10))
	}
}
func setRetryAfter(w http.ResponseWriter, d time.Duration) {
	seconds := int64(d.Seconds())
	if seconds < 1 {
		seconds = 1
	}
	w.Header().Set("Retry-After", strconv.FormatInt(seconds, 10))
}
