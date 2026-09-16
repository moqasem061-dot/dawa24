package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/whatsapp"
)

const secret = "0123456789abcdef0123456789abcdef0123456789abcdef"

// The service is never reached when authentication fails; a nil one proves
// it, because touching it would panic.
func TestBridgeRefusesWithoutTheSharedSecret(t *testing.T) {
	cases := map[string]struct{ token, header string }{
		"no header":         {secret, ""},
		"wrong token":       {secret, "Bearer " + strings.Repeat("z", len(secret))},
		"basic scheme":      {secret, "Basic " + secret},
		"no secret on host": {"", "Bearer "},
	}
	for name, c := range cases {
		r := chi.NewRouter()
		NewBridge((*whatsapp.Service)(nil), c.token, nil).RegisterRoutes(r)
		for _, path := range []string{"/webhook", "/outbox/claim", "/outbox/report"} {
			req := httptest.NewRequest(http.MethodPost, Prefix+path, strings.NewReader(`{}`))
			if c.header != "" {
				req.Header.Set("Authorization", c.header)
			}
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("%s %s: status %d, want 401", name, path, rec.Code)
			}
		}
	}
}
