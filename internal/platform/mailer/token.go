package mailer

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

	"github.com/muhiya/dawa24-store/internal/modules/identity"
)

var (
	ErrTokenInvalidFormat = errors.New("mailer: invalid token format")
	ErrTokenExpired       = errors.New("mailer: token has expired")
	ErrTokenSignature     = errors.New("mailer: token signature invalid")
	ErrTokenEmailMismatch = errors.New("mailer: email mismatch")
	ErrTokenPurposeWrong  = errors.New("mailer: token purpose mismatch")
)

// VerifiedEmailPayload is serialized and signed to verify email ownership or authorize password reset.
type VerifiedEmailPayload struct {
	Email     string `json:"email"`
	Purpose   string `json:"purpose"` // "email_verify" or "password_reset"
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
	Nonce     string `json:"nonce"`
}

// SignEmailToken generates a signed proof token confirming that the email address was verified via OTP.
func SignEmailToken(email, secret string, ttl time.Duration) (string, error) {
	return signToken(email, "email_verify", secret, ttl)
}

// VerifyEmailToken checks that the token is valid, unexpired, and matches expected email.
func VerifyEmailToken(token, expectedEmail, secret string) (bool, error) {
	email, err := extractTokenEmail(token, "email_verify", secret)
	if err != nil {
		return false, err
	}
	cleanExpected := identity.NormalizeEmail(expectedEmail)
	if cleanExpected == "" || email != cleanExpected {
		return false, ErrTokenEmailMismatch
	}
	return true, nil
}

// SignPasswordResetToken creates a signed token allowing the user to submit a new password.
func SignPasswordResetToken(email, secret string, ttl time.Duration) (string, error) {
	return signToken(email, "password_reset", secret, ttl)
}

// VerifyPasswordResetToken validates the reset token and returns the authorized email address.
func VerifyPasswordResetToken(token, secret string) (string, error) {
	return extractTokenEmail(token, "password_reset", secret)
}

func signToken(email, purpose, secret string, ttl time.Duration) (string, error) {
	cleanEmail := identity.NormalizeEmail(email)
	if cleanEmail == "" {
		return "", errors.New("mailer: email cannot be empty")
	}
	if secret == "" {
		return "", errors.New("mailer: secret required for token signing")
	}
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}

	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		return "", fmt.Errorf("mailer: generate nonce: %w", err)
	}

	now := time.Now()
	payload := VerifiedEmailPayload{
		Email:     cleanEmail,
		Purpose:   purpose,
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

func extractTokenEmail(token, expectedPurpose, secret string) (string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", ErrTokenInvalidFormat
	}
	if secret == "" {
		return "", errors.New("mailer: secret required for token verification")
	}

	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return "", ErrTokenInvalidFormat
	}

	payloadB64, sigHex := parts[0], parts[1]

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

	var payload VerifiedEmailPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return "", ErrTokenInvalidFormat
	}

	if payload.Purpose != expectedPurpose {
		return "", ErrTokenPurposeWrong
	}

	if time.Now().Unix() > payload.ExpiresAt {
		return "", ErrTokenExpired
	}

	return payload.Email, nil
}
