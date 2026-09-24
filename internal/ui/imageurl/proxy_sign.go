package imageurl

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"sync"
)

// The /uploads/proxy endpoint fetches a remote image and keeps a copy on the
// upload volume. It is public so <img> tags work for anonymous visitors, which
// made it an anonymous "download this URL to our disk" service: every distinct
// URL cost up to 10 MB of storage that nothing reclaimed. Only URLs this
// server rendered carry a valid signature, so a caller can no longer pick one.

var (
	proxyKeyMu sync.RWMutex
	proxyKey   []byte
	randomOnce sync.Once
)

// SetProxyKey installs the key that signs proxy URLs. Every replica must use
// the same key so a page rendered by one is served by another; the server
// derives it from the session secret.
func SetProxyKey(k []byte) {
	proxyKeyMu.Lock()
	defer proxyKeyMu.Unlock()
	proxyKey = append([]byte(nil), k...)
}

func currentProxyKey() []byte {
	proxyKeyMu.RLock()
	k := proxyKey
	proxyKeyMu.RUnlock()
	if len(k) > 0 {
		return k
	}
	// Unconfigured (tests, a single dev process): a per-process random key is
	// still unforgeable, it just does not survive a restart.
	randomOnce.Do(func() {
		b := make([]byte, 32)
		_, _ = rand.Read(b)
		proxyKeyMu.Lock()
		if len(proxyKey) == 0 {
			proxyKey = b
		}
		proxyKeyMu.Unlock()
	})
	proxyKeyMu.RLock()
	defer proxyKeyMu.RUnlock()
	return proxyKey
}

func proxySignature(remote string) string {
	mac := hmac.New(sha256.New, currentProxyKey())
	mac.Write([]byte(remote))
	return hex.EncodeToString(mac.Sum(nil)[:16])
}

// ProxyURL returns the signed /uploads/proxy address for a remote image.
func ProxyURL(remote string) string {
	return "/uploads/proxy?url=" + url.QueryEscape(remote) + "&sig=" + proxySignature(remote)
}

// ValidProxySignature reports whether sig was issued by ProxyURL for remote.
func ValidProxySignature(remote, sig string) bool {
	if remote == "" || sig == "" {
		return false
	}
	return hmac.Equal([]byte(sig), []byte(proxySignature(remote)))
}
