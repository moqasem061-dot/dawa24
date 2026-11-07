package httpx

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
}

func serve(h http.Handler, method, path string) int {
	req := httptest.NewRequest(method, path, nil)
	req.RemoteAddr = "203.0.113.7:4000"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w.Code
}

// Routes with different limits must not share a counter: a busy route (ad
// impressions) must not use up the allowance of a sensitive one (login).
func TestLimitByIPKeepsOneCounterPerRoute(t *testing.T) {
	url := os.Getenv("TEST_REDIS_URL")
	if url == "" {
		t.Skip("set TEST_REDIS_URL")
	}
	opts, err := redis.ParseURL(url)
	if err != nil {
		t.Fatal(err)
	}
	client := redis.NewClient(opts)
	defer func() { _ = client.Close() }()

	l := NewLimiter(client, fmt.Sprintf("routetest:%d:", time.Now().UnixNano()))
	r := chi.NewRouter()
	r.With(l.LimitByIP(300, time.Minute)).Get("/ads/impression/{ad}", okHandler().ServeHTTP)
	r.With(l.LimitByIP(5, time.Minute)).Post("/auth/login", okHandler().ServeHTTP)

	for i := 0; i < 50; i++ {
		if c := serve(r, http.MethodGet, fmt.Sprintf("/ads/impression/%d", i)); c != http.StatusOK {
			t.Fatalf("impression %d: status %d", i, c)
		}
	}
	for i := 1; i <= 5; i++ {
		if c := serve(r, http.MethodPost, "/auth/login"); c != http.StatusOK {
			t.Fatalf("login %d after 50 impressions: status %d, want 200", i, c)
		}
	}
	if c := serve(r, http.MethodPost, "/auth/login"); c != http.StatusTooManyRequests {
		t.Fatalf("sixth login: status %d, want 429", c)
	}
	// Different ad ids are one route and share its counter.
	r2 := chi.NewRouter()
	r2.With(l.LimitByIP(2, time.Minute)).Get("/ads/click/{ad}", okHandler().ServeHTTP)
	serve(r2, http.MethodGet, "/ads/click/1")
	serve(r2, http.MethodGet, "/ads/click/2")
	if c := serve(r2, http.MethodGet, "/ads/click/3"); c != http.StatusTooManyRequests {
		t.Fatalf("third click on varying ad ids: status %d, want 429", c)
	}
}

// A Redis outage must not turn into 429 errors: the limiter fails open.
func TestLimitByIPFailsOpenOnRedisError(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1, DialTimeout: 200 * time.Millisecond})
	defer func() { _ = client.Close() }()
	l := NewLimiter(client, "failopen:")
	h := l.LimitByIP(1, time.Minute)(okHandler())
	if c := serve(h, http.MethodGet, "/tracking"); c != http.StatusOK {
		t.Fatalf("status %d during a Redis outage, want 200", c)
	}
}
