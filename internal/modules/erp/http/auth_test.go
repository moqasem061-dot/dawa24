package http

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/moqasem061-dot/dawa24/internal/modules/erp"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/platform/httpx"
	"github.com/redis/go-redis/v9"
)

type authRedisHook struct {
	mu     sync.Mutex
	counts map[string]int64
	expiry map[string]time.Time
	now    time.Time
	fail   bool
}

func (h *authRedisHook) DialHook(next redis.DialHook) redis.DialHook { return next }
func (h *authRedisHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
func (h *authRedisHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(_ context.Context, cmd redis.Cmder) error {
		h.mu.Lock()
		defer h.mu.Unlock()
		if h.fail {
			return errors.New("redis unavailable")
		}
		args := cmd.Args()
		key := fmt.Sprint(args[1])
		if until, ok := h.expiry[key]; ok && !h.now.Before(until) {
			delete(h.counts, key)
			delete(h.expiry, key)
		}
		switch cmd.Name() {
		case "get":
			c := cmd.(*redis.StringCmd)
			if n, ok := h.counts[key]; ok {
				c.SetVal(strconv.FormatInt(n, 10))
			} else {
				c.SetErr(redis.Nil)
				return redis.Nil
			}
		case "incr":
			h.counts[key]++
			cmd.(*redis.IntCmd).SetVal(h.counts[key])
		case "expire":
			h.expiry[key] = h.now.Add(time.Minute)
			cmd.(*redis.BoolCmd).SetVal(true)
		case "ttl":
			ttl := time.Duration(-2)
			if at, ok := h.expiry[key]; ok {
				ttl = at.Sub(h.now)
			}
			cmd.(*redis.DurationCmd).SetVal(ttl)
		default:
			return errors.New("unexpected Redis command")
		}
		return nil
	}
}
func newAuthHarness(t *testing.T) (*authLayer, *authRedisHook, *int, []erp.CallRecord) {
	t.Helper()
	hook := &authRedisHook{counts: map[string]int64{}, expiry: map[string]time.Time{}, now: time.Now()}
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	client.AddHook(hook)
	t.Cleanup(func() { _ = client.Close() })
	lim := httpx.NewLimiter(client, "test:erp:")
	count := new(int)
	var records []erp.CallRecord
	layer := &authLayer{lim: lim, log: slog.New(slog.NewTextHandler(io.Discard, nil)), authenticate: func(_ context.Context, raw, ip string) (erp.Principal, error) {
		*count++
		if raw != "valid" {
			return erp.Principal{}, erp.NewError("ERP_AUTH_INVALID", 401, nil)
		}
		return erp.Principal{OrganizationID: 123, KeyID: 7, KeyPrefix: "abcdefgh", RatePerMinute: 1000}, nil
	}, record: func(c erp.CallRecord) { records = append(records, c) }}
	return layer, hook, count, records
}
func authRequest(h http.Handler, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", "/resource?organization_id=999", nil)
	req.RemoteAddr = "203.0.113.7:1234"
	req.Header.Set("X-Organization-Id", "999")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}
func TestAuthFailureLimit(t *testing.T) {
	layer, hook, calls, _ := newAuthHarness(t)
	h := layer.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := principalFromContext(r.Context())
		org, tenant := database.TenantFrom(r.Context())
		if !ok || !tenant || p.OrganizationID != 123 || org != 123 {
			t.Error("tenant not from principal")
		}
		w.WriteHeader(204)
	}))
	for i := 0; i < 100; i++ {
		if w := authRequest(h, "valid"); w.Code != 204 {
			t.Fatalf("success %d: %d", i, w.Code)
		}
	}
	for i := 0; i < erp.AuthFailuresPerIPPerMinute; i++ {
		if w := authRequest(h, "bad"); w.Code != 401 {
			t.Fatalf("failure %d: %d", i, w.Code)
		}
	}
	before := *calls
	w := authRequest(h, "valid")
	if w.Code != 429 || w.Header().Get("Retry-After") == "" || *calls != before {
		t.Fatalf("blocked valid status=%d calls=%d before=%d", w.Code, *calls, before)
	}
	hook.mu.Lock()
	hook.now = hook.now.Add(time.Minute + time.Second)
	hook.mu.Unlock()
	if w = authRequest(h, "valid"); w.Code != 204 {
		t.Fatalf("after expiry: %d", w.Code)
	}
}
func TestAuthFailureIsolationAndRedisOutage(t *testing.T) {
	layer, hook, _, _ := newAuthHarness(t)
	handler := layer.middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	for i := 0; i < erp.AuthFailuresPerIPPerMinute; i++ {
		authRequest(handler, "bad")
	}
	login := layer.lim.LimitByIP(1, time.Minute)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	if w := authRequest(login, ""); w.Code != 204 {
		t.Fatalf("ERP failures affected login: %d", w.Code)
	}
	hook.mu.Lock()
	hook.fail = true
	hook.mu.Unlock()
	if w := authRequest(handler, "valid"); w.Code != 204 {
		t.Fatalf("Redis outage blocked valid key: %d", w.Code)
	}
}
func TestAuthMissingQueryAndRateHeaders(t *testing.T) {
	layer, _, _, _ := newAuthHarness(t)
	layer.authenticate = func(_ context.Context, raw, ip string) (erp.Principal, error) {
		return erp.Principal{OrganizationID: 123, KeyID: 7, RatePerMinute: 1}, nil
	}
	h := layer.middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	w := authRequest(h, "")
	if w.Code != 401 || !strings.Contains(w.Body.String(), "ERP_AUTH_MISSING") {
		t.Fatalf("missing auth: %d %s", w.Code, w.Body.String())
	}
	w = authRequest(h, "valid")
	if w.Code != 204 || w.Header().Get("X-RateLimit-Limit") != "1" {
		t.Fatalf("headers: %v", w.Header())
	}
	w = authRequest(h, "valid")
	if w.Code != 429 || w.Header().Get("Retry-After") == "" || w.Header().Get("X-RateLimit-Remaining") != "0" || w.Header().Get("X-RateLimit-Reset") == "" {
		t.Fatalf("limit: %d %v", w.Code, w.Header())
	}
}
func TestAuthCallRoutePattern(t *testing.T) {
	layer, _, _, _ := newAuthHarness(t)
	var calls []erp.CallRecord
	layer.record = func(c erp.CallRecord) { calls = append(calls, c) }
	r := chi.NewRouter()
	r.With(layer.middleware).Get("/resource/{id}", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })
	req := httptest.NewRequest("GET", "/resource/12", nil)
	req.RemoteAddr = "203.0.113.7:1234"
	req.Header.Set("Authorization", "Bearer valid")
	r.ServeHTTP(httptest.NewRecorder(), req)
	if len(calls) != 1 || calls[0].Route != "/resource/{id}" {
		t.Fatalf("records=%+v", calls)
	}
}
