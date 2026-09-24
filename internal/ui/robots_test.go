package ui_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/moqasem061-dot/dawa24/internal/ui"
)

func TestRobotsTxt_ContentSignals(t *testing.T) {
	// The fetcher is package state another test may have installed; this test
	// is about the embedded file.
	saved := ui.DynamicRobotsTxtFetcher
	ui.DynamicRobotsTxtFetcher = nil
	t.Cleanup(func() { ui.DynamicRobotsTxtFetcher = saved })

	r := chi.NewRouter()
	ui.RegisterStaticRoutes(r, func(*http.Request) string { return "https://dawa24.com" })

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/robots.txt", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /robots.txt, got %d", rec.Code)
	}

	headerSig := rec.Header().Get("Content-Signal")
	if !strings.Contains(headerSig, "ai-train=no") || !strings.Contains(headerSig, "search=yes") || !strings.Contains(headerSig, "ai-input=no") {
		t.Errorf("expected Content-Signal header with ai-train=no, search=yes, ai-input=no, got %q", headerSig)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Content-Signal: ai-train=no, search=yes, ai-input=no") {
		t.Errorf("expected robots.txt to contain 'Content-Signal: ai-train=no, search=yes, ai-input=no', got body:\n%s", body)
	}

	if !strings.Contains(body, "Sitemap: https://dawa24.com/sitemap.xml") {
		t.Errorf("Sitemap must name this deployment's absolute sitemap URL, got body:\n%s", body)
	}

	lines := strings.Split(body, "\n")
	foundUserAgentAll := false
	foundContentSignalUnderAll := false
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if strings.EqualFold(line, "User-agent: *") {
			foundUserAgentAll = true
			continue
		}
		if foundUserAgentAll {
			if strings.HasPrefix(line, "Content-Signal:") {
				foundContentSignalUnderAll = true
				if !strings.Contains(line, "ai-train=no") || !strings.Contains(line, "search=yes") || !strings.Contains(line, "ai-input=no") {
					t.Errorf("unexpected Content-Signal under User-agent: *: %s", line)
				}
				break
			}
			if strings.HasPrefix(line, "User-agent:") {
				break
			}
		}
	}

	if !foundContentSignalUnderAll {
		t.Errorf("Content-Signal directive not found directly in User-agent: * block")
	}
}
