package ui

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/ui/layouts"
)

func TestTutorialMapServesRoleSpecificHTML(t *testing.T) {
	for _, role := range []string{"vendor", "pharmacy", "driver"} {
		t.Run(role, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := httptest.NewRequestWithContext(context.Background(), "GET", "/tutorial", nil).WithContext(layouts.WithNonce(context.Background(), "test-csp-nonce"))
			tutorialMap(role).ServeHTTP(response, request)
			if response.Code != 200 || !strings.Contains(response.Header().Get("Content-Type"), "text/html") || response.Body.Len() < 1000 {
				t.Fatalf("tutorial response = %d %q %d bytes", response.Code, response.Header().Get("Content-Type"), response.Body.Len())
			}
			if !strings.Contains(response.Body.String(), `style nonce="test-csp-nonce"`) || strings.Contains(response.Body.String(), `https://dawa24.com`) {
				t.Fatal("tutorial inline styles or same-site links were not adapted to the app security context")
			}
		})
	}
}
