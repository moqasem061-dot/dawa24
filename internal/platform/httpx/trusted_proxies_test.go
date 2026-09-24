package httpx

import (
	"net/http/httptest"
	"testing"
)

func TestClientIPIgnoresForwardedHeadersFromUntrustedPeer(t *testing.T) {
	t.Cleanup(func() { SetTrustedProxies(nil) })

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "203.0.113.9:4000"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")

	// No allowlist: hop counting, as before.
	if got := ClientIP(req, 1); got != "1.2.3.4" {
		t.Fatalf("without allowlist ClientIP = %q; want 1.2.3.4", got)
	}

	if bad := SetTrustedProxies([]string{"172.16.0.0/12", "10.0.0.5", "nonsense"}); len(bad) != 1 {
		t.Fatalf("invalid entries = %v; want one", bad)
	}
	// A direct caller cannot choose its address.
	if got := ClientIP(req, 1); got != "203.0.113.9" {
		t.Errorf("untrusted peer: ClientIP = %q; want the TCP peer", got)
	}
	// Through a listed proxy, the forwarded address is used.
	req.RemoteAddr = "172.18.0.2:5000"
	if got := ClientIP(req, 1); got != "1.2.3.4" {
		t.Errorf("trusted proxy: ClientIP = %q; want 1.2.3.4", got)
	}
	req.RemoteAddr = "10.0.0.5:5000"
	if got := ClientIP(req, 1); got != "1.2.3.4" {
		t.Errorf("single-IP proxy entry: ClientIP = %q; want 1.2.3.4", got)
	}
}
