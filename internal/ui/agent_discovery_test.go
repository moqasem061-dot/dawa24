package ui

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func newTestAgentRouter() http.Handler {
	r := chi.NewRouter()
	h := &UIHandler{}
	h.RegisterPublicRoutes(r)
	return r
}

func TestLinkHeaders(t *testing.T) {
	router := newTestAgentRouter()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	resp := w.Result()
	linkHeader := resp.Header.Get("Link")
	if linkHeader == "" {
		t.Fatalf("expected Link header on homepage, got none")
	}

	for _, rel := range []string{`rel="describedby"`, `rel="service-desc"`, `rel="service-doc"`} {
		if !strings.Contains(linkHeader, rel) {
			t.Errorf("expected relation %s in Link header, got: %s", rel, linkHeader)
		}
	}
}

func TestSitemap(t *testing.T) {
	router := newTestAgentRouter()

	req := httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	cType := resp.Header.Get("Content-Type")
	if !strings.Contains(cType, "xml") {
		t.Errorf("expected Content-Type xml, got %s", cType)
	}

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "<urlset") || !strings.Contains(bodyStr, "<loc>") {
		t.Errorf("sitemap.xml missing urlset or loc tags: %s", bodyStr)
	}
}

func TestLLMsTxtAndAgentsIndex(t *testing.T) {
	router := newTestAgentRouter()

	// 1. Test /llms.txt
	req := httptest.NewRequest(http.MethodGet, "/llms.txt", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for /llms.txt, got %d", w.Result().StatusCode)
	}

	// 2. Test /.well-known/agents-index.json
	req2 := httptest.NewRequest(http.MethodGet, "/.well-known/agents-index.json", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	if w2.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for /.well-known/agents-index.json, got %d", w2.Result().StatusCode)
	}
}

func TestMarkdownNegotiation(t *testing.T) {
	router := newTestAgentRouter()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept", "text/markdown")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}

	cType := resp.Header.Get("Content-Type")
	if !strings.Contains(cType, "text/markdown") {
		t.Errorf("expected Content-Type text/markdown, got %s", cType)
	}

	if resp.Header.Get("X-Markdown-Tokens") == "" {
		t.Errorf("expected X-Markdown-Tokens header on markdown response")
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "# Dawa24") {
		t.Errorf("markdown response missing title: %s", string(body))
	}
}

func TestRemovedAgentToolsAndCatalogsReturn404(t *testing.T) {
	router := newTestAgentRouter()

	removedEndpoints := []string{
		"/.well-known/api-catalog",
		"/.well-known/ai-catalog.json",
		"/.well-known/openid-configuration",
		"/.well-known/oauth-authorization-server",
		"/.well-known/oauth-protected-resource",
		"/.well-known/jwks.json",
		"/.well-known/mcp/server-card.json",
		"/.well-known/agent-skills/index.json",
		"/.well-known/agent-skills/catalog-search/SKILL.md",
		"/auth.md",
		"/.well-known/auth.md",
		"/docs/api",
		"/api/v1/openapi.json",
	}

	for _, ep := range removedEndpoints {
		req := httptest.NewRequest(http.MethodGet, ep, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Result().StatusCode != http.StatusNotFound {
			t.Errorf("expected 404 for removed endpoint %s, got %d", ep, w.Result().StatusCode)
		}
	}
}
