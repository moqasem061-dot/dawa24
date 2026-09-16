package ui_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/antiscrape"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui"
)

// guardedRouter mounts the real route tables with a live guard, which is
// the only way to test the thing that actually matters here: not what the guard
// decides, but which routes it is mounted on.
func guardedRouter() http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := ui.NewUIHandler(
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger,
	)
	handler.SetScrapeGuard(antiscrape.New(antiscrape.Options{
		Enabled:   true,
		Log:       logger,
		KeyPrefix: "test:routes:",
	}))

	r := chi.NewRouter()
	handler.RegisterPublicRoutes(r)
	handler.RegisterCustomerRoutes(r)
	return r
}

func customerActorReq(r *http.Request) *http.Request {
	actor := authctx.Actor{
		UserID:         1,
		OrganizationID: 10,
		OrgStatus:      "approved",
		Permissions:    []string{"pharmacy.purchase_request.view", "pharmacy.supplier.view", "pharmacy.offer.view"},
	}
	return r.WithContext(authctx.WithActor(r.Context(), actor))
}

// scraperRequest is what the guard exists to refuse: a HTTP client that names
// itself.
func scraperRequest(path string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, path, nil)
	r.Header.Set("User-Agent", "python-requests/2.31.0")
	return r
}

// The guard is mounted on the catalogue listing and the supplier directory,
// and on nothing else. This test is the enforcement of that: a future edit that
// widens the guarded group fails here rather than quietly changing how the rest
// of the public surface answers.
func TestOnlyTheListingRoutesAreGuarded(t *testing.T) {
	router := guardedRouter()

	for _, path := range []string{"/catalog", "/suppliers", "/suppliers/1"} {
		t.Run("guarded "+path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, scraperRequest(path))

			if rec.Code != http.StatusForbidden {
				t.Fatalf("GET %s as python-requests returned %d, want 403", path, rec.Code)
			}
		})
	}

	unguarded := []string{
		"/catalog/123",
		"/offers",
		"/offers/1",
		"/jobs",
		"/jobs/1",
		"/compare/search?q=panadol",
		"/api/v1/compare/search?q=panadol",
		"/",
		"/about",
		"/auth/login",
	}

	for _, path := range unguarded {
		t.Run("unguarded "+path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, scraperRequest(path))

			// The services behind these handlers are nil in this harness, so
			// the status varies (200, 303, 404). The only thing asserted is
			// that it is not the guard's refusal: 403 here would mean the
			// guard had spread beyond /catalog.
			if rec.Code == http.StatusForbidden {
				t.Fatalf("GET %s returned 403 — the guard must be mounted on /catalog only", path)
			}
		})
	}
}

// A real browser must reach /catalog untouched. A guard that refuses customers
// is worse than no guard.
func TestCatalogStillServesABrowser(t *testing.T) {
	router := guardedRouter()

	r := httptest.NewRequest(http.MethodGet, "/catalog", nil)
	r.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 "+
		"(KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36")
	r.Header.Set("Accept", "text/html,application/xhtml+xml")
	r = customerActorReq(r)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, r)

	if rec.Code == http.StatusForbidden || rec.Code == http.StatusTooManyRequests {
		t.Fatalf("GET /catalog from Chrome returned %d — the guard refused a browser", rec.Code)
	}
}

// Googlebot must keep reaching the catalogue, or the guard silently de-indexes
// the marketplace it is meant to protect.
func TestGuardedPagesStillServeGooglebot(t *testing.T) {
	router := guardedRouter()

	for _, path := range []string{"/catalog", "/suppliers"} {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		r.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)")
		r.Header.Set("Accept", "text/html")
		r = customerActorReq(r)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, r)

		if rec.Code == http.StatusForbidden {
			t.Fatalf("GET %s as Googlebot returned 403 - the public pages must stay indexable", path)
		}
	}
}

// The platform is meant to work well with AI agents. An assistant fetching a
// page to answer somebody's question is the good case and must get through; the
// training collector from the same vendor must not. The two are one substring
// apart, so this is worth pinning at the routing layer and not only in the
// classifier's own unit test.
func TestAssistantsPassAndTrainingCrawlersDoNot(t *testing.T) {
	router := guardedRouter()

	allowed := []string{
		"Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko); compatible; ChatGPT-User/1.0; +https://openai.com/bot",
		"Mozilla/5.0 (compatible; OAI-SearchBot/1.0; +https://openai.com/searchbot)",
		"Mozilla/5.0 (compatible; Perplexity-User/1.0; +https://perplexity.ai/perplexity-user)",
		"Mozilla/5.0 (compatible; Claude-User/1.0; +Claude-User@anthropic.com)",
	}
	for _, ua := range allowed {
		r := httptest.NewRequest(http.MethodGet, "/catalog", nil)
		r.Header.Set("User-Agent", ua)
		r.Header.Set("Accept", "text/html")
		r = customerActorReq(r)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, r)

		if rec.Code == http.StatusForbidden {
			t.Errorf("GET /catalog as %q returned 403 - an assistant answering a person must get through", ua)
		}
	}

	refused := []string{
		"Mozilla/5.0 (compatible; GPTBot/1.1; +https://openai.com/gptbot)",
		"Mozilla/5.0 (compatible; ClaudeBot/1.0; +claudebot@anthropic.com)",
		"Mozilla/5.0 (compatible; CCBot/2.0; +https://commoncrawl.org/faq/)",
		"Mozilla/5.0 (compatible; Bytespider; spider-feedback@bytedance.com)",
		"Mozilla/5.0 (compatible; Google-Extended)",
	}
	for _, ua := range refused {
		r := httptest.NewRequest(http.MethodGet, "/catalog", nil)
		r.Header.Set("User-Agent", ua)
		r.Header.Set("Accept", "text/html")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, r)

		if rec.Code != http.StatusForbidden {
			t.Errorf("GET /catalog as %q returned %d, want 403 - training collectors take the price list and send nobody", ua, rec.Code)
		}
	}
}

// Without a guard the routing table must be identical. The guard is a property
// of the deployment, not of which routes exist.
func TestAbsentGuardChangesNothing(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := ui.NewUIHandler(
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger,
	)
	r := chi.NewRouter()
	handler.RegisterPublicRoutes(r)
	handler.RegisterCustomerRoutes(r)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, scraperRequest("/catalog"))

	if rec.Code == http.StatusForbidden {
		t.Fatalf("GET /catalog with no guard wired returned 403, want the handler's own answer")
	}
}
