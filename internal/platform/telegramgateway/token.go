package telegramgateway

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrTokenInvalidFormat = errors.New("telegramgateway: invalid token format")
	ErrTokenExpired       = errors.New("telegramgateway: token has expired")
	ErrTokenSignature     = errors.New("telegramgateway: token signature invalid")
	ErrTokenPhoneMismatch = errors.New("telegramgateway: phone number mismatch")
)

// VerifiedPhonePayload is serialized and signed to prove phone ownership.
type VerifiedPhonePayload struct {
	Phone     string `json:"phone"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
	Nonce     string `json:"nonce"`
}

// SignPhoneToken creates an HMAC-SHA256 signed token confirming the phone was verified.
func SignPhoneToken(phone, secret string, ttl time.Duration) (string, error) {
	normPhone, err := NormalizePhone(phone)
	if err != nil {
		return "", err
	}
	if secret == "" {
		return "", errors.New("telegramgateway: secret required for token signing")
	}
	if ttl == 0 {
		ttl = 30 * time.Minute
	}

	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	now := time.Now()
	payload := VerifiedPhonePayload{
		Phone:     normPhone,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(ttl).Unix(),
		Nonce:     hex.EncodeToString(nonceBytes),
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payloadB64))
	sig := hex.EncodeToString(mac.Sum(nil))

	return payloadB64 + "." + sig, nil
}

// VerifyPhoneToken verifies that the token was signed with secret, is not expired,
// and strictly matches the expected normalized phone number.
func VerifyPhoneToken(token, expectedPhone, secret string) (bool, error) {
	phoneInToken, err := ExtractPhoneFromToken(token, secret)
	if err != nil {
		return false, err
	}

	normExpected, err := NormalizePhone(expectedPhone)
	if err != nil {
		return false, fmt.Errorf("%w: %v", ErrInvalidPhone, err)
	}

	if phoneInToken != normExpected {
		return false, ErrTokenPhoneMismatch
	}

	return true, nil
}

// ExtractPhoneFromToken verifies signature and expiration, then returns the verified phone.
func ExtractPhoneFromToken(token, secret string) (string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", ErrTokenInvalidFormat
	}
	if secret == "" {
		return "", errors.New("telegramgateway: secret required for token verification")
	}

	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return "", ErrTokenInvalidFormat
	}

	payloadB64, sigHex := parts[0], parts[1]

	// Verify HMAC signature in constant time
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payloadB64))
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(sigHex), []byte(expectedSig)) {
		return "", ErrTokenSignature
	}

	payloadJSON, err := base64.RawURLEncoding.DecodeString(payloadB64)
	if err != nil {
		return "", ErrTokenInvalidFormat
	}

	var payload VerifiedPhonePayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return "", ErrTokenInvalidFormat
	}

	now := time.Now().Unix()
	if now > payload.ExpiresAt {
		return "", ErrTokenExpired
	}

	return payload.Phone, nil
}
