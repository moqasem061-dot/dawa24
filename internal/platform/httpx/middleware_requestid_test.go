package httpx

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/platform/observability"
)

func TestRequestIDShape(t *testing.T) {
	hex16 := regexp.MustCompile(`^[0-9a-f]{16}$`)

	cases := []struct {
		name    string
		inbound string
		keep    bool
	}{
		{name: "generated when absent", inbound: "", keep: false},
		{name: "21 characters replaced", inbound: "abcdefghijklmnopqrstu", keep: false},
		{name: "slash replaced", inbound: "a/b", keep: false},
		{name: "plus replaced", inbound: "abc+123", keep: false},
		{name: "short safe id kept", inbound: "abc-123", keep: true},
		{name: "20 characters kept", inbound: "abcdefghijklmnopqrst", keep: true},
		{name: "underscore kept", inbound: "req_42", keep: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var seen string
			h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				seen = observability.RequestIDFrom(r.Context())
			}))
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.inbound != "" {
				req.Header.Set("X-Request-ID", tc.inbound)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Header().Get("X-Request-ID") != seen {
				t.Fatalf("header %q does not match context %q", rec.Header().Get("X-Request-ID"), seen)
			}
			if tc.keep {
				if seen != tc.inbound {
					t.Fatalf("inbound %q was not kept, got %q", tc.inbound, seen)
				}
				return
			}
			if seen == tc.inbound {
				t.Fatalf("inbound %q should have been replaced", tc.inbound)
			}
			if !hex16.MatchString(seen) {
				t.Fatalf("generated id %q is not 16 lowercase hex characters", seen)
			}
		})
	}
}

func TestIsShortRequestID(t *testing.T) {
	cases := map[string]bool{
		"":                      false,
		"a":                     true,
		"abc-123":               true,
		"ABC_xyz-09":            true,
		"abcdefghijklmnopqrst":  true,
		"abcdefghijklmnopqrstu": false,
		"a/b":                   false,
		"a+b":                   false,
		"a b":                   false,
		"a.b":                   false,
	}
	for id, want := range cases {
		if got := isShortRequestID(id); got != want {
			t.Errorf("isShortRequestID(%q) = %v, want %v", id, got, want)
		}
	}
}

func TestNewRequestIDLength(t *testing.T) {
	for i := 0; i < 50; i++ {
		id := newRequestID()
		if len(id) != 16 {
			t.Fatalf("newRequestID() = %q, want 16 characters", id)
		}
	}
}
