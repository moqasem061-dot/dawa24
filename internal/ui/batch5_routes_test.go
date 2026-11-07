package ui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"

	"github.com/moqasem061-dot/dawa24/internal/platform/httpx"
)

// S23: ad impressions from one address stop at the per-IP limit, and the
// limit is per address. Redis is needed for the counter, so the test skips
// without TEST_REDIS_URL.
func TestAdImpressionRouteIsRateLimitedPerIP(t *testing.T) {
	redisURL := os.Getenv("TEST_REDIS_URL")
	if redisURL == "" {
		t.Skip("set TEST_REDIS_URL to run the rate limit route test")
	}
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		t.Fatal(err)
	}
	client := redis.NewClient(opts)
	defer func() { _ = client.Close() }()

	h := &UIHandler{limiter: httpx.NewLimiter(client, fmt.Sprintf("batch5test:%d:", time.Now().UnixNano()))}
	router := chi.NewRouter()
	h.RegisterPublicRoutes(router)

	hit := func(ip string) int {
		req := httptest.NewRequest(http.MethodGet, "/ads/impression/1", nil)
		req.RemoteAddr = ip + ":40000"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w.Code
	}

	for i := 0; i < 300; i++ {
		if code := hit("203.0.113.5"); code != http.StatusNoContent {
			t.Fatalf("request %d from one address: status %d, want 204", i+1, code)
		}
	}
	if code := hit("203.0.113.5"); code != http.StatusTooManyRequests {
		t.Fatalf("request 301 from one address: status %d, want 429", code)
	}
	if code := hit("198.51.100.9"); code != http.StatusNoContent {
		t.Fatalf("another address was limited too: status %d", code)
	}
}
