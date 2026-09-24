package httpx

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
)

// trustedProxyNets, when set, lists the peers allowed to speak for the client
// through X-Forwarded-For / X-Real-IP.
//
// Counting hops from the right is only safe when every request arrives through
// the proxy. If the process is also reachable directly — a published container
// port — a caller writes their own X-Forwarded-For and picks the address every
// per-IP limit (login, MFA, OTP) keys on. With TRUSTED_PROXIES configured, a
// request whose TCP peer is not a listed proxy is keyed on that peer instead.
var trustedProxyNets atomic.Pointer[[]*net.IPNet]

// SetTrustedProxies installs the proxy allowlist from TRUSTED_PROXIES. Entries
// are CIDRs or single IPs; invalid entries are returned and skipped. An empty
// list keeps the hop-count behaviour.
func SetTrustedProxies(entries []string) (invalid []string) {
	nets := make([]*net.IPNet, 0, len(entries))
	for _, e := range entries {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		if !strings.Contains(e, "/") {
			if ip := net.ParseIP(e); ip != nil {
				bits := 128
				if ip.To4() != nil {
					bits = 32
				}
				e = ip.String() + "/" + strconv.Itoa(bits)
			}
		}
		_, n, err := net.ParseCIDR(e)
		if err != nil {
			invalid = append(invalid, e)
			continue
		}
		nets = append(nets, n)
	}
	trustedProxyNets.Store(&nets)
	return invalid
}

// peerIsUntrusted reports whether an allowlist is configured and the request's
// TCP peer is not on it.
func peerIsUntrusted(r *http.Request) bool {
	p := trustedProxyNets.Load()
	if p == nil || len(*p) == 0 {
		return false
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return true
	}
	for _, n := range *p {
		if n.Contains(ip) {
			return false
		}
	}
	return true
}
