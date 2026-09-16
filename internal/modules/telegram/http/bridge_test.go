package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/telegram"
)

const secret = "0123456789abcdef0123456789abcdef0123456789abcdef"

func serve(token, header string) int {
	r := chi.NewRouter()
	// The service is never reached when authentication fails; a nil one
	// proves it, because touching it would panic.
	NewBridge((*telegram.Service)(nil), token, nil).RegisterRoutes(r)
	req := httptest.NewRequest(http.MethodPost, Prefix+"/updates", strings.NewReader(`{"update_id":1}`))
	if header != "" {
		req.Header.Set("Authorization", header)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec.Code
}

func TestBridgeRefusesWithoutTheSharedSecret(t *testing.T) {
	cases := map[string]string{
		"no header":        "",
		"wrong token":      "Bearer " + strings.Repeat("z", len(secret)),
		"prefix of token":  "Bearer " + secret[:20],
		"basic scheme":     "Basic " + secret,
		"token no scheme":  secret,
		"token plus extra": "Bearer " + secret + "x",
	}
	for name, header := range cases {
		if code := serve(secret, header); code != http.StatusUnauthorized {
			t.Errorf("%s: status %d, want 401", name, code)
		}
	}
}

func TestBridgeWithNoConfiguredSecretRefusesEverything(t *testing.T) {
	for _, header := range []string{"", "Bearer ", "Bearer x"} {
		if code := serve("", header); code != http.StatusUnauthorized {
			t.Errorf("header %q: status %d, want 401", header, code)
		}
	}
}

func TestBridgeAcceptsTheSharedSecret(t *testing.T) {
	b := NewBridge(nil, secret, nil)
	if !b.auth.Authorized("Bearer "+secret) || !b.auth.Authorized("bearer  "+secret) {
		t.Fatal("valid token refused")
	}
}
