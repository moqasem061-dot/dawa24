package ui

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"sync"
)

// processMFAKey signs pending-MFA cookies when no session secret is configured.
// A per-process key only means a pending login must finish on the replica that
// started it; it never falls back to a value an attacker can read.
var (
	processMFAKeyOnce sync.Once
	processMFAKey     []byte
)

// mfaPendingKey returns the key that signs the dawa24_mfa_pending cookie.
//
// The cookie names the user who passed the password step. Anyone who can sign
// it skips that step, so the key must be secret: it is derived from the
// configured session secret under its own label, so it is never the session
// secret itself and never a constant compiled into the binary.
func (h *UIHandler) mfaPendingKey() []byte {
	if h.sessionSecret != "" {
		mac := hmac.New(sha256.New, []byte(h.sessionSecret))
		mac.Write([]byte("dawa24/mfa-pending/v1"))
		return mac.Sum(nil)
	}
	processMFAKeyOnce.Do(func() {
		processMFAKey = make([]byte, 32)
		if _, err := rand.Read(processMFAKey); err != nil {
			panic("ui: cannot generate MFA pending key: " + err.Error())
		}
	})
	return processMFAKey
}
