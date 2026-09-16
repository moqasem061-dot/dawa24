package pagecontrol

import (
	"net/http"
	"sort"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestNormalizePath(t *testing.T) {
	cases := map[string]string{
		"/about":           "/about",
		"/about/":          "/about",
		"about":            "/about",
		"/about?x=1":       "/about",
		"/about#frag":      "/about",
		"//admin///users/": "/admin/users",
		"  /admin/users  ": "/admin/users",
		"/":                "/",
		"":                 "/",
		"/admin/users/42/": "/admin/users/42",
	}
	for in, want := range cases {
		if got := NormalizePath(in); got != want {
			t.Errorf("NormalizePath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestClassifyResource(t *testing.T) {
	cases := map[string]Resource{
		"/admin":          ResourceAdmin,
		"/admin/users":    ResourceAdmin,
		"/vendor/orders":  ResourceVendor,
		"/customer/cart":  ResourceClient,
		"/about":          ResourceIndependent,
		"/":               ResourceIndependent,
		"/administrative": ResourceIndependent, // not /admin/
	}
	for in, want := range cases {
		if got := ClassifyResource(in); got != want {
			t.Errorf("ClassifyResource(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsProtected(t *testing.T) {
	protected := []string{"/health", "/ready", "/auth/login", "/static/app.css", "/admin/system-pages", "/admin/system-pages/9/toggle", "/admin/dashboard", "/admin/roles/x"}
	for _, p := range protected {
		if !IsProtected(p) {
			t.Errorf("IsProtected(%q) = false, want true", p)
		}
	}
	notProtected := []string{"/about", "/admin/users", "/admin/settings", "/vendor/orders", "/healthy"}
	for _, p := range notProtected {
		if IsProtected(p) {
			t.Errorf("IsProtected(%q) = true, want false", p)
		}
	}
}

func TestValidatePathAndPrefixRule(t *testing.T) {
	if err := ValidatePath("/admin/users"); err != nil {
		t.Errorf("ValidatePath(/admin/users) errored: %v", err)
	}
	for _, bad := range []string{"", "admin", "/has space", "/q?x=1"} {
		if err := ValidatePath(bad); err == nil {
			t.Errorf("ValidatePath(%q) = nil, want error", bad)
		}
	}
	if err := ValidatePrefixRule("/", MatchPrefix); err == nil {
		t.Error("ValidatePrefixRule(/, prefix) = nil, want error")
	}
	if err := ValidatePrefixRule("/", MatchExact); err != nil {
		t.Errorf("ValidatePrefixRule(/, exact) errored: %v", err)
	}
}

func TestStaticPrefix(t *testing.T) {
	cases := map[string]string{
		"/admin/users/{id}/edit": "/admin/users",
		"/vendor/orders":         "/vendor/orders",
		"/catalog/{id}":          "/catalog",
		"/admin/*":               "/admin",
		"/":                      "/",
		"/{id}":                  "/",
	}
	for in, want := range cases {
		if got := staticPrefix(in); got != want {
			t.Errorf("staticPrefix(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDiscoverRoutes(t *testing.T) {
	r := chi.NewRouter()
	r.Get("/about", noopHandler)
	r.Get("/admin/users", noopHandler)
	r.Get("/admin/users/{id}", noopHandler)
	r.Post("/admin/users/{id}/suspend", noopHandler)
	r.Get("/vendor/orders", noopHandler)

	got := map[string]Candidate{}
	for _, c := range DiscoverRoutes(r) {
		got[c.Path] = c
	}

	if _, ok := got["/"]; ok {
		t.Error("discovery kept the root path; it must be skipped")
	}
	users, ok := got["/admin/users"]
	if !ok {
		t.Fatal("discovery missed /admin/users")
	}
	if users.Resource != ResourceAdmin {
		t.Errorf("/admin/users classified as %q, want admin", users.Resource)
	}
	if len(users.Patterns) != 3 {
		t.Errorf("/admin/users grouped %d patterns, want 3: %v", len(users.Patterns), users.Patterns)
	}
	if _, ok := got["/about"]; !ok {
		t.Error("discovery missed /about")
	}
}

// --- engine matching ---

func engineWith(rules ...rule) *Engine {
	e := &Engine{byExact: map[string]rule{}}
	for _, rl := range rules {
		rl.path = NormalizePath(rl.path)
		if rl.mode == MatchPrefix {
			e.prefixes = append(e.prefixes, rl)
		} else {
			e.byExact[rl.path] = rl
		}
	}
	sort.Slice(e.prefixes, func(i, j int) bool { return len(e.prefixes[i].path) > len(e.prefixes[j].path) })
	return e
}

func TestEngineDecision(t *testing.T) {
	e := engineWith(
		rule{id: 1, path: "/about", mode: MatchExact, enabled: false},
		rule{id: 2, path: "/contact", mode: MatchExact, enabled: true},
		rule{id: 3, path: "/vendor", mode: MatchPrefix, enabled: false},
		rule{id: 4, path: "/vendor/orders", mode: MatchExact, enabled: true},
		rule{id: 5, path: "/customer/reports", mode: MatchPrefix, enabled: false},
	)

	type tc struct {
		path    string
		blocked bool
	}
	for _, c := range []tc{
		{"/about", true},          // exact disabled
		{"/about/", true},         // normalized to /about
		{"/contact", false},       // exact enabled
		{"/vendor", true},         // prefix disabled, self
		{"/vendor/x", true},       // prefix disabled, child
		{"/vendor/orders", false}, // more specific exact enabled wins
		{"/customer/reports/weekly", true},
		{"/unmanaged", false}, // no rule
	} {
		if got, _ := e.Decision(c.path); got != c.blocked {
			t.Errorf("Decision(%q) = %v, want %v", c.path, got, c.blocked)
		}
	}
}

func TestEngineNeverBlocksProtected(t *testing.T) {
	e := engineWith(
		rule{id: 1, path: "/admin", mode: MatchPrefix, enabled: false},
		rule{id: 2, path: "/admin/system-pages", mode: MatchPrefix, enabled: false},
	)
	for _, p := range []string{"/admin/system-pages", "/admin/dashboard", "/admin/roles", "/health", "/auth/login"} {
		if blocked, _ := e.Decision(p); blocked {
			t.Errorf("Decision(%q) blocked a protected path", p)
		}
	}
	// A non-protected admin path under the disabled prefix is still blocked.
	if blocked, _ := e.Decision("/admin/settings"); !blocked {
		t.Error("Decision(/admin/settings) = not blocked, want blocked under disabled /admin prefix")
	}
}

func TestNilEngineBlocksNothing(t *testing.T) {
	var e *Engine
	if blocked, _ := e.Decision("/anything"); blocked {
		t.Error("nil engine blocked a path")
	}
	if Blocked("/anything") {
		t.Error("Blocked() with no global engine returned true")
	}
}

func noopHandler(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }
