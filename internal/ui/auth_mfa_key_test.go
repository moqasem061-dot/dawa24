package ui

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"
)

// A pending-MFA cookie signed with the key that used to be compiled into the
// binary (and is therefore public) must not be accepted.
func TestPendingMFACookieRejectsPublicKey(t *testing.T) {
	h := &UIHandler{sessionSecret: "test-session-secret"}
	payload, _ := json.Marshal(pendingMFAPayload{UserID: 1, ExpiresAt: time.Now().Add(time.Hour).Unix()})
	b64 := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte("dawa24_mfa_intermediate_key_secret_2026"))
	mac.Write([]byte(b64))
	forged := b64 + "." + hex.EncodeToString(mac.Sum(nil))

	if _, err := h.parsePendingMFAToken(forged); err == nil {
		t.Fatal("cookie signed with the old public key was accepted")
	}

	genuine := h.signPendingMFAToken(&pendingMFAPayload{UserID: 7, ExpiresAt: time.Now().Add(time.Hour).Unix()})
	got, err := h.parsePendingMFAToken(genuine)
	if err != nil || got.UserID != 7 {
		t.Fatalf("own token did not round-trip: %v, %v", got, err)
	}

	other := &UIHandler{sessionSecret: "a-different-secret"}
	if _, err := other.parsePendingMFAToken(genuine); err == nil {
		t.Fatal("token verified under a different session secret")
	}
}
