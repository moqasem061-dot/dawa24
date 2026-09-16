package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLimiterFailsOpenWhenRedisNil(t *testing.T) {
	limiter := NewLimiter(nil, "test:limit:")

	middleware := limiter.LimitUserOrIP(100, 10, time.Minute)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest("GET", "/api/v1/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 when redis is nil (fail open), got %d", w.Code)
	}
}

func TestSetRateLimitHeaders(t *testing.T) {
	w := httptest.NewRecorder()
	setRateLimitHeaders(w, 100, 25, 45*time.Second)

	if got := w.Header().Get("X-RateLimit-Limit"); got != "100" {
		t.Errorf("expected limit 100, got %s", got)
	}
	if got := w.Header().Get("X-RateLimit-Remaining"); got != "75" {
		t.Errorf("expected remaining 75, got %s", got)
	}
	if got := w.Header().Get("X-RateLimit-Reset"); got != "45" {
		t.Errorf("expected reset 45, got %s", got)
	}
}

func TestUserExtractorWiring(t *testing.T) {
	limiter := NewLimiter(nil, "test:limit:")
	called := false

	limiter.SetUserExtractor(func(r *http.Request) int64 {
		called = true
		return 42
	})

	if limiter.userFn == nil {
		t.Fatal("expected userFn to be set")
	}

	req := httptest.NewRequest("GET", "/", nil)
	uid := limiter.userFn(req)
	if !called || uid != 42 {
		t.Fatalf("expected extractor to return 42, got %d", uid)
	}
}
