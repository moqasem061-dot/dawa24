package httpx_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/httpx"
)

// X-Forwarded-For is written by the caller and appended to by each proxy, so
// the trustworthy end of the chain is the right-hand one. Reading the left-hand
// entry - the obvious reading, and the one this code used to do - hands a
// scraper a fresh identity on every request simply by sending the header, which
// makes every per-address defence built on it count to one forever.
func TestClientIPCountsFromTheRight(t *testing.T) {
	cases := []struct {
		name       string
		forwarded  string
		realIP     string
		remoteAddr string
		hops       int
		want       string
	}{
		{
			name:       "one proxy, honest client",
			forwarded:  "203.0.113.7",
			remoteAddr: "10.0.0.1:5000",
			hops:       1,
			want:       "203.0.113.7",
		},
		{
			name:       "one proxy, client forged a leading entry",
			forwarded:  "1.2.3.4, 203.0.113.7",
			remoteAddr: "10.0.0.1:5000",
			hops:       1,
			want:       "203.0.113.7",
		},
		{
			name:       "two proxies",
			forwarded:  "1.2.3.4, 203.0.113.7, 10.0.0.9",
			remoteAddr: "10.0.0.1:5000",
			hops:       2,
			want:       "203.0.113.7",
		},
		{
			name:       "chain shorter than the configured hops falls back to its left end",
			forwarded:  "203.0.113.7",
			remoteAddr: "10.0.0.1:5000",
			hops:       3,
			want:       "203.0.113.7",
		},
		{
			name:       "no proxy in front: the header is ignored entirely",
			forwarded:  "1.2.3.4",
			remoteAddr: "203.0.113.7:5000",
			hops:       0,
			want:       "203.0.113.7",
		},
		{
			name:       "no forwarding header, peer address is used",
			remoteAddr: "203.0.113.7:5000",
			hops:       1,
			want:       "203.0.113.7",
		},
		{
			name:       "X-Real-IP when there is no chain",
			realIP:     "203.0.113.7",
			remoteAddr: "10.0.0.1:5000",
			hops:       1,
			want:       "203.0.113.7",
		},
		{
			name:       "IPv6 peer address keeps its brackets off",
			remoteAddr: "[2001:db8::1]:5000",
			hops:       0,
			want:       "2001:db8::1",
		},
		{
			name:       "whitespace in the chain is trimmed",
			forwarded:  "1.2.3.4 ,   203.0.113.7  ",
			remoteAddr: "10.0.0.1:5000",
			hops:       1,
			want:       "203.0.113.7",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/catalog", nil)
			r.RemoteAddr = tc.remoteAddr
			if tc.forwarded != "" {
				r.Header.Set("X-Forwarded-For", tc.forwarded)
			}
			if tc.realIP != "" {
				r.Header.Set("X-Real-IP", tc.realIP)
			}

			if got := httpx.ClientIP(r, tc.hops); got != tc.want {
				t.Errorf("ClientIP(hops=%d) = %q, want %q", tc.hops, got, tc.want)
			}
		})
	}
}

// The whole point of counting from the right: a caller spraying forged
// addresses must keep landing in the same bucket.
func TestForgedForwardedForCannotMintIdentities(t *testing.T) {
	forgeries := []string{
		"1.1.1.1, 203.0.113.7",
		"2.2.2.2, 203.0.113.7",
		"3.3.3.3, 4.4.4.4, 203.0.113.7",
	}

	for _, xff := range forgeries {
		r := httptest.NewRequest(http.MethodGet, "/catalog", nil)
		r.RemoteAddr = "10.0.0.1:5000"
		r.Header.Set("X-Forwarded-For", xff)

		if got := httpx.ClientIP(r, 1); got != "203.0.113.7" {
			t.Errorf("X-Forwarded-For %q resolved to %q, want the proxy-written entry 203.0.113.7", xff, got)
		}
	}
}
