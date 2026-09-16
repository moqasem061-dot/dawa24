package httpx_test

import (
	"crypto/tls"
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/stretchr/testify/assert"
)

func TestDefaultTLSConfig_HardeningInvariants(t *testing.T) {
	cfg := httpx.DefaultTLSConfig()

	assert.NotNil(t, cfg)
	assert.Equal(t, uint16(tls.VersionTLS12), cfg.MinVersion)
	assert.True(t, cfg.PreferServerCipherSuites)

	// Post-Quantum Cryptography curves present
	assert.Contains(t, cfg.CurvePreferences, tls.X25519MLKEM768)
	assert.Contains(t, cfg.CurvePreferences, tls.SecP256r1MLKEM768)
	assert.Contains(t, cfg.CurvePreferences, tls.X25519)

	// All TLS 1.2 suites must provide Forward Secrecy (ECDHE) and AEAD (GCM / Poly1305)
	assert.NotEmpty(t, cfg.CipherSuites)
	for _, id := range cfg.CipherSuites {
		name := tls.CipherSuiteName(id)
		assert.Contains(t, name, "ECDHE", "cipher suite %s must support forward secrecy", name)
		assert.NotContains(t, name, "CBC", "cipher suite %s must not use vulnerable CBC mode", name)
		assert.False(t, strings.HasPrefix(name, "TLS_RSA_WITH_"), "cipher suite %s must not use static RSA key exchange", name)
	}

	assert.Contains(t, cfg.NextProtos, "h2")
	assert.Contains(t, cfg.NextProtos, "http/1.1")
}
