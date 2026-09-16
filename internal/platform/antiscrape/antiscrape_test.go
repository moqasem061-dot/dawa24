package antiscrape

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

const chromeUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 " +
	"(KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36"

func testGuard(t *testing.T) *Guard {
	t.Helper()
	return New(Options{
		Enabled:   true,
		Log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		KeyPrefix: "test:" + t.Name() + ":",
		// No Redis: the in-process store is what a deployment falls back to
		// during a Redis outage, so it is the one worth testing.
		TrustedProxyHops: 0,
	})
}

func browserRequest(path string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, path, nil)
	r.Header.Set("User-Agent", chromeUA)
	r.Header.Set("Accept", "text/html,application/xhtml+xml")
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	return r
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestClassify(t *testing.T) {
	cases := []struct {
		name   string
		ua     string
		accept string
		want   Class
	}{
		{"chrome", chromeUA, "text/html", ClassBrowser},
		{"safari ios", "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15", "text/html", ClassBrowser},
		{"googlebot", "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)", "text/html", ClassCrawler},
		{"bingbot", "Mozilla/5.0 (compatible; bingbot/2.0; +http://www.bing.com/bingbot.htm)", "*/*", ClassCrawler},
		{"curl", "curl/8.4.0", "*/*", ClassAutomation},
		{"python requests", "python-requests/2.31.0", "*/*", ClassAutomation},
		{"scrapy", "Scrapy/2.11 (+https://scrapy.org)", "*/*", ClassAutomation},
		{"go client", "Go-http-client/2.0", "*/*", ClassAutomation},
		{"headless chrome", "Mozilla/5.0 (X11; Linux x86_64) HeadlessChrome/120.0.0.0", "text/html", ClassAutomation},
		{"ahrefs", "Mozilla/5.0 (compatible; AhrefsBot/7.0; +http://ahrefs.com/robot/)", "*/*", ClassAutomation},
		{"gptbot", "Mozilla/5.0 (compatible; GPTBot/1.1; +https://openai.com/gptbot)", "*/*", ClassAutomation},
		{"google-extended", "Mozilla/5.0 (compatible; Google-Extended)", "*/*", ClassAutomation},
		{"meta training", "meta-externalagent/1.1 (+https://developers.facebook.com/docs/sharing/webmasters/crawler)", "*/*", ClassAutomation},
		// One substring apart from the four above, and on the other side of the
		// line: these fetch because a person just asked something.
		{"chatgpt-user", "Mozilla/5.0 ... ChatGPT-User/1.0; +https://openai.com/bot", "text/html", ClassCrawler},
		{"oai-searchbot", "Mozilla/5.0 (compatible; OAI-SearchBot/1.0; +https://openai.com/searchbot)", "text/html", ClassCrawler},
		{"perplexity-user", "Mozilla/5.0 (compatible; Perplexity-User/1.0)", "text/html", ClassCrawler},
		{"claude-user", "Mozilla/5.0 (compatible; Claude-User/1.0)", "text/html", ClassCrawler},
		{"no user agent", "", "text/html", ClassUnknown},
		{"not a browser string", "SomeInternalTool/1.0", "text/html", ClassUnknown},
		{"browser string, no accept", chromeUA, "", ClassUnknown},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/catalog", nil)
			if tc.ua != "" {
				r.Header.Set("User-Agent", tc.ua)
			}
			if tc.accept != "" {
				r.Header.Set("Accept", tc.accept)
			}
			if got := Classify(r); got != tc.want {
				t.Errorf("Classify() = %v, want %v", got, tc.want)
			}
		})
	}
}

// A crawler string must win over a harvester substring, or one unlucky
// substring silently de-indexes the marketplace.
func TestCrawlersAreNotMistakenForHarvesters(t *testing.T) {
	g := testGuard(t)
	r := httptest.NewRequest(http.MethodGet, "/catalog", nil)
	r.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)")
	r.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()

	g.Protect(okHandler()).ServeHTTP(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("Googlebot got %d, want 200 - the public catalogue must stay indexable", rec.Code)
	}
}

func TestAutomationIsRefusedOnDataRoutes(t *testing.T) {
	g := testGuard(t)
	r := httptest.NewRequest(http.MethodGet, "/catalog", nil)
	r.Header.Set("User-Agent", "python-requests/2.31.0")
	rec := httptest.NewRecorder()

	g.Protect(okHandler()).ServeHTTP(rec, r)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if got := rec.Header().Get("X-Robots-Tag"); !strings.Contains(got, "noindex") {
		t.Errorf("X-Robots-Tag = %q, want noindex: a refusal must not become the indexed /catalog", got)
	}
	// The reason must not reach the body. Naming the signal tells a scraper
	// exactly which header to change.
	if body := rec.Body.String(); strings.Contains(body, "user_agent") || strings.Contains(body, reasonAutomation) {
		t.Errorf("refusal body names the signal that caught it:\n%s", body)
	}
}

func TestAuthenticatedAutomationIsMeteredNotRefused(t *testing.T) {
	g := testGuard(t)
	r := httptest.NewRequest(http.MethodGet, "/catalog", nil)
	r.Header.Set("User-Agent", "curl/8.4.0")
	r = r.WithContext(authctx.WithActor(r.Context(), authctx.Actor{UserID: 7}))
	rec := httptest.NewRecorder()

	g.Protect(okHandler()).ServeHTTP(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: a signed-in caller has an account to lose and is metered, not blocked", rec.Code)
	}
}

func TestBurstBudgetIsEnforcedAndSignalled(t *testing.T) {
	g := testGuard(t)
	h := g.Protect(okHandler())
	limit := budgets[ClassBrowser].burst

	for i := 0; i < limit; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, browserRequest("/catalog"))
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d of %d got %d, want 200", i+1, limit, rec.Code)
		}
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, browserRequest("/catalog"))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("request %d got %d, want 429", limit+1, rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("a 429 without Retry-After tells a well-behaved client nothing")
	}
}

// A signed-in pharmacy browses far harder than any visitor and must not be
// refused at the anonymous ceiling.
func TestSignedInCallersGetALargerBudget(t *testing.T) {
	g := testGuard(t)
	h := g.Protect(okHandler())
	beyondAnonymous := budgets[ClassBrowser].burst + 1

	for i := 0; i < beyondAnonymous; i++ {
		r := browserRequest("/catalog")
		r = r.WithContext(authctx.WithActor(r.Context(), authctx.Actor{UserID: 42}))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		if rec.Code != http.StatusOK {
			t.Fatalf("signed-in request %d got %d, want 200", i+1, rec.Code)
		}
	}
}

// Two visitors behind different addresses must not share one allowance.
func TestBudgetsAreCountedPerCaller(t *testing.T) {
	g := testGuard(t)
	h := g.Protect(okHandler())

	for i := 0; i <= budgets[ClassBrowser].burst; i++ {
		r := browserRequest("/catalog")
		r.RemoteAddr = "203.0.113.9:1111"
		h.ServeHTTP(httptest.NewRecorder(), r)
	}

	r := browserRequest("/catalog")
	r.RemoteAddr = "198.51.100.4:2222"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("second address got %d, want 200: one visitor must not exhaust another's allowance", rec.Code)
	}
}

func TestHoneypotPenaltyOutlivesTheRequestThatTrippedIt(t *testing.T) {
	g := testGuard(t)
	h := g.Protect(okHandler())

	// The catalogue filter form carries a hidden field no person can see or tab
	// into. Filling it is what the handler reports here.
	trip := browserRequest("/catalog?company_tax_ref=1")
	trip.RemoteAddr = "203.0.113.55:3333"
	g.Penalize(trip, "honeypot_field")

	after := browserRequest("/catalog")
	after.RemoteAddr = "203.0.113.55:4444"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, after)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status after honeypot = %d, want 403 - a penalty that expires with the request is no penalty", rec.Code)
	}
}

// An /api/ refusal must be JSON. A caller parsing JSON that receives an HTML
// page reports a syntax error, and nobody finds the rate limit.
func TestAPIRefusalsAreJSON(t *testing.T) {
	g := testGuard(t)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/compare/search?q=x", nil)
	r.Header.Set("User-Agent", "curl/8.4.0")
	rec := httptest.NewRecorder()

	g.Protect(okHandler()).ServeHTTP(rec, r)

	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("Content-Type = %q, want JSON", ct)
	}
}

// htmx swaps the response into the page, so it must get HTML even though the
// request is a background fetch.
func TestHTMXRefusalsAreHTML(t *testing.T) {
	g := testGuard(t)
	r := httptest.NewRequest(http.MethodGet, "/catalog", nil)
	r.Header.Set("User-Agent", "curl/8.4.0")
	r.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()

	g.Protect(okHandler()).ServeHTTP(rec, r)

	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("Content-Type = %q, want HTML", ct)
	}
}

func TestDisabledAndNilGuardsArePassthroughs(t *testing.T) {
	for name, g := range map[string]*Guard{
		"nil":      nil,
		"disabled": New(Options{Enabled: false}),
	} {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/catalog", nil)
			r.Header.Set("User-Agent", "curl/8.4.0")
			rec := httptest.NewRecorder()

			g.Protect(okHandler()).ServeHTTP(rec, r)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200: an absent guard must not change which routes work", rec.Code)
			}
		})
	}
}

func TestMemStoreWindowExpires(t *testing.T) {
	m := newMemStore()
	if got := m.Hit(t.Context(), "k", 20*time.Millisecond); got != 1 {
		t.Fatalf("first hit = %d, want 1", got)
	}
	if got := m.Hit(t.Context(), "k", 20*time.Millisecond); got != 2 {
		t.Fatalf("second hit = %d, want 2", got)
	}
	time.Sleep(30 * time.Millisecond)
	if got := m.Hit(t.Context(), "k", 20*time.Millisecond); got != 1 {
		t.Fatalf("hit after the window = %d, want 1: the window must reset, not accumulate forever", got)
	}
}
