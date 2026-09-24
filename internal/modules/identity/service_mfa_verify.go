package identity

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
)

// VerifyMFA checks whether a code is a valid 6-digit TOTP code or an unused recovery code.
func (s *Service) VerifyMFA(ctx context.Context, userID int64, code string) (bool, error) {
	if userID <= 0 {
		return false, apperr.Validation("user_id.invalid", "Valid user ID is required.", nil)
	}

	mfa, err := s.repo.GetMFA(ctx, userID)
	if err != nil || mfa == nil || !mfa.Enabled || len(mfa.TOTPSecret) == 0 {
		return false, apperr.Validation("mfa.not_enabled", i18n.TDefault("w4_mod.w4str_187_187"), nil)
	}

	// A second factor guessed at 10 requests a minute per IP falls in hours to
	// an attacker spreading requests across addresses. The account's own
	// failure counter — the one password login uses — caps guesses per user.
	now := time.Now().UTC()
	sec, _ := s.repo.GetSecurity(ctx, userID)
	if sec == nil {
		sec = &UserSecurity{UserID: userID}
	}
	if sec.IsLocked(now) {
		return false, apperr.Forbidden("mfa.locked", "Too many failed verification attempts. Try again later.")
	}
	ok := s.checkMFACode(ctx, mfa, code)
	if !ok {
		sec.RecordFailedAttempt(now)
		_ = s.repo.UpsertSecurity(ctx, sec)
	}
	return ok, nil
}

// checkMFACode reports whether code is a current TOTP code or an unused
// recovery code, consuming the recovery code when it matches.
func (s *Service) checkMFACode(ctx context.Context, mfa *UserMFA, code string) bool {
	userID := mfa.UserID
	cleanCode := strings.ToUpper(strings.TrimSpace(code))
	cleanCodeDigits := strings.ReplaceAll(cleanCode, "-", "")

	// 1. Try standard 6-digit TOTP code
	if len(cleanCodeDigits) == TOTPDigits {
		if ValidateTOTP(string(mfa.TOTPSecret), cleanCodeDigits, time.Now().UTC()) {
			return true
		}
	}

	// 2. Try single-use recovery code
	if len(mfa.RecoveryCodes) > 0 {
		var existingCodes []string
		if err := json.Unmarshal(mfa.RecoveryCodes, &existingCodes); err == nil && len(existingCodes) > 0 {
			for i, rCode := range existingCodes {
				if recoveryCodeMatches(rCode, cleanCode) {
					// Consume this recovery code
					updatedCodes := append(existingCodes[:i], existingCodes[i+1:]...)
					updatedBytes, _ := json.Marshal(updatedCodes)
					mfa.RecoveryCodes = updatedBytes
					_ = s.repo.UpsertMFA(ctx, mfa)
					s.log.WarnContext(ctx, "recovery code consumed for login", "user_id", userID, "remaining_codes", len(updatedCodes))
					return true
				}
			}
		}
	}

	return false
}

// Recovery codes are stored as SHA-256 digests (prefix "h1:"), so a database
// read no longer yields working second factors. Codes stored before this
// change are plain text and still accepted until used or regenerated.
const recoveryHashPrefix = "h1:"

func normalizeRecoveryCode(c string) string {
	return strings.ReplaceAll(strings.ToUpper(strings.TrimSpace(c)), "-", "")
}

func hashRecoveryCode(c string) string {
	sum := sha256.Sum256([]byte(normalizeRecoveryCode(c)))
	return recoveryHashPrefix + hex.EncodeToString(sum[:])
}

func hashRecoveryCodes(codes []string) []string {
	out := make([]string, len(codes))
	for i, c := range codes {
		out[i] = hashRecoveryCode(c)
	}
	return out
}

// recoveryCodeMatches compares a submitted code with one stored entry, hashed
// or legacy plain text, in constant time.
func recoveryCodeMatches(stored, submitted string) bool {
	if strings.HasPrefix(stored, recoveryHashPrefix) {
		return subtle.ConstantTimeCompare([]byte(stored), []byte(hashRecoveryCode(submitted))) == 1
	}
	return subtle.ConstantTimeCompare([]byte(normalizeRecoveryCode(stored)), []byte(normalizeRecoveryCode(submitted))) == 1
}
