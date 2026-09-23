package httpx_test

import (
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/httpx"
)

// directive returns one CSP directive's value, or "" when it is absent.
func directive(csp, name string) string {
	for _, d := range strings.Split(csp, ";") {
		d = strings.TrimSpace(d)
		if d == name || strings.HasPrefix(d, name+" ") {
			return d
		}
	}
	return ""
}

func TestCSPAdmitsClarityOnlyWhenEnabled(t *testing.T) {
	on, onRes := serveSecurityHeaders(t, httpx.SecurityOptions{Clarity: true})
	defer func() { _ = onRes.Body.Close() }()
	for _, name := range []string{"script-src", "img-src", "connect-src"} {
		d := directive(on, name)
		if !strings.Contains(d, "https://*.clarity.ms") || !strings.Contains(d, "https://c.bing.com") {
			t.Errorf("with Clarity, %s must admit Clarity's hosts: %q", name, d)
		}
	}
	// The nonce requirement is untouched.
	if !strings.Contains(directive(on, "script-src"), "'nonce-") {
		t.Error("script-src lost its nonce")
	}
	if strings.Contains(directive(on, "frame-src"), "clarity") {
		t.Error("frame-src must not be widened for Clarity")
	}

	off, offRes := serveSecurityHeaders(t, httpx.SecurityOptions{})
	defer func() { _ = offRes.Body.Close() }()
	if strings.Contains(off, "clarity") || strings.Contains(off, "bing.com") {
		t.Errorf("without Clarity the policy must stay narrow: %q", off)
	}
}
