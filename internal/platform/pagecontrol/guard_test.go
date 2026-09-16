package pagecontrol

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

const (
	okBody       = "SERVED"
	notFoundBody = "GENERIC_404"
)

func testNext() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, okBody)
	})
}

func testNotFound(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	_, _ = io.WriteString(w, notFoundBody)
}

// withGlobal installs e as the process engine for one test and restores nil
// after. Guard reads the engine through Global().
func withGlobal(t *testing.T, e *Engine) {
	t.Helper()
	globalMu.Lock()
	global = e
	globalMu.Unlock()
	t.Cleanup(func() {
		globalMu.Lock()
		global = nil
		globalMu.Unlock()
	})
}

func do(h http.Handler, method, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func TestGuardPassesWhenNoEngine(t *testing.T) {
	h := Guard(testNext(), testNotFound, nil)
	rec := do(h, http.MethodGet, "/about")
	if rec.Code != http.StatusOK || rec.Body.String() != okBody {
		t.Fatalf("no engine: got %d %q, want 200 %q", rec.Code, rec.Body.String(), okBody)
	}
}

func TestGuardBlocksDisabledExactForEveryCaller(t *testing.T) {
	withGlobal(t, engineWith(rule{id: 7, path: "/about", mode: MatchExact, enabled: false}))
	h := Guard(testNext(), testNotFound, nil)

	blocked := do(h, http.MethodGet, "/about")
	if blocked.Code != http.StatusNotFound || blocked.Body.String() != notFoundBody {
		t.Errorf("disabled /about: got %d %q, want 404 %q", blocked.Code, blocked.Body.String(), notFoundBody)
	}
	// Trailing slash must not be a way around it.
	if slash := do(h, http.MethodGet, "/about/"); slash.Code != http.StatusNotFound {
		t.Errorf("disabled /about/: got %d, want 404", slash.Code)
	}
	// A POST to the same path is refused too — a page is all its methods.
	if post := do(h, http.MethodPost, "/about"); post.Code != http.StatusNotFound {
		t.Errorf("POST disabled /about: got %d, want 404", post.Code)
	}
	// An unrelated path is untouched.
	if other := do(h, http.MethodGet, "/contact"); other.Code != http.StatusOK {
		t.Errorf("/contact: got %d, want 200", other.Code)
	}
}

func TestGuardPrefixWithMoreSpecificOverride(t *testing.T) {
	withGlobal(t, engineWith(
		rule{id: 1, path: "/vendor", mode: MatchPrefix, enabled: false},
		rule{id: 2, path: "/vendor/orders", mode: MatchExact, enabled: true},
	))
	h := Guard(testNext(), testNotFound, nil)

	if child := do(h, http.MethodGet, "/vendor/inventory"); child.Code != http.StatusNotFound {
		t.Errorf("/vendor/inventory under disabled prefix: got %d, want 404", child.Code)
	}
	if kept := do(h, http.MethodGet, "/vendor/orders"); kept.Code != http.StatusOK {
		t.Errorf("/vendor/orders re-enabled: got %d, want 200", kept.Code)
	}
}

func TestGuardNeverBlocksProtected(t *testing.T) {
	withGlobal(t, engineWith(
		rule{id: 1, path: "/admin", mode: MatchPrefix, enabled: false},
		rule{id: 2, path: "/admin/system-pages", mode: MatchPrefix, enabled: false},
		rule{id: 3, path: "/health", mode: MatchPrefix, enabled: false},
	))
	h := Guard(testNext(), testNotFound, nil)

	for _, p := range []string{"/health", "/admin/system-pages", "/admin/dashboard", "/admin/roles", "/auth/login"} {
		if rec := do(h, http.MethodGet, p); rec.Code != http.StatusOK {
			t.Errorf("protected %s: got %d, want 200 (never blocked)", p, rec.Code)
		}
	}
	// But a non-protected admin path under the disabled prefix is still refused.
	if rec := do(h, http.MethodGet, "/admin/settings"); rec.Code != http.StatusNotFound {
		t.Errorf("/admin/settings under disabled /admin: got %d, want 404", rec.Code)
	}
}

func TestGuardCustomDisabledHandler(t *testing.T) {
	withGlobal(t, engineWith(rule{
		id:          42,
		path:        "/market",
		mode:        MatchExact,
		enabled:     false,
		labelAr:     "سوق الأدوية",
		labelEn:     "Marketplace",
		description: "Marketplace test",
	}))

	var intercepted bool
	var capturedInfo BlockedInfo
	SetDisabledHandler(func(w http.ResponseWriter, r *http.Request) {
		intercepted = true
		info, ok := BlockedInfoFrom(r.Context())
		if ok {
			capturedInfo = info
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, "MAINTENANCE_PAGE")
	})
	t.Cleanup(func() {
		SetDisabledHandler(nil)
	})

	h := Guard(testNext(), testNotFound, nil)
	rec := do(h, http.MethodGet, "/market")

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("got status %d, want 503", rec.Code)
	}
	if rec.Body.String() != "MAINTENANCE_PAGE" {
		t.Errorf("got body %q, want MAINTENANCE_PAGE", rec.Body.String())
	}
	if !intercepted {
		t.Errorf("expected custom disabled handler to be called")
	}
	if capturedInfo.RuleID != 42 || capturedInfo.LabelAr != "سوق الأدوية" {
		t.Errorf("captured info mismatch: %+v", capturedInfo)
	}
}

