package mailer

import (
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
)

var (
	ErrOTPNotFound        = errors.New("mailer: no pending OTP found for this email")
	ErrOTPExpired         = errors.New("mailer: OTP code has expired")
	ErrOTPMaxAttempts     = errors.New("mailer: maximum verification attempts exceeded")
	ErrOTPInvalidCode     = errors.New("mailer: invalid OTP code")
	ErrOTPInCooldown      = errors.New("mailer: please wait before requesting another code")
	ErrOTPHourlyExceeded  = errors.New("mailer: hourly request limit reached")
)

type otpEntry struct {
	Code      string
	ExpiresAt time.Time
	Attempts  int
}

// OTPEngine manages in-memory OTP codes, expirations, cooldowns, and rate limits.
type OTPEngine struct {
	mu          sync.Mutex
	codes       map[string]otpEntry     // "purpose:email" -> entry
	cooldowns   map[string]time.Time    // "purpose:email" -> cooldown expiry
	hourlySends map[string][]time.Time  // "purpose:email" -> send timestamps
}

// NewOTPEngine creates a new OTPEngine.
func NewOTPEngine() *OTPEngine {
	return &OTPEngine{
		codes:       make(map[string]otpEntry),
		cooldowns:   make(map[string]time.Time),
		hourlySends: make(map[string][]time.Time),
	}
}

// GenerateOTP creates a cryptographically secure 6-digit code, stores it with ttl,
// and enforces rate limits. Returns (code, cooldownSeconds, error).
func (e *OTPEngine) GenerateOTP(email, purpose string, ttl time.Duration) (string, int, error) {
	cleanEmail := identity.NormalizeEmail(email)
	if cleanEmail == "" {
		return "", 0, errors.New("mailer: email cannot be empty")
	}
	key := purpose + ":" + cleanEmail

	e.mu.Lock()
	defer e.mu.Unlock()
	e.pruneExpired()

	// Check cooldown (60 seconds)
	if exp, ok := e.cooldowns[key]; ok {
		if remaining := time.Until(exp); remaining > 0 {
			return "", int(remaining.Seconds()) + 1, ErrOTPInCooldown
		}
		delete(e.cooldowns, key)
	}

	// Check hourly limit (5 per hour)
	cutoff := time.Now().Add(-1 * time.Hour)
	var recentSends []time.Time
	for _, t := range e.hourlySends[key] {
		if t.After(cutoff) {
			recentSends = append(recentSends, t)
		}
	}
	if len(recentSends) >= 5 {
		e.hourlySends[key] = recentSends
		return "", 0, ErrOTPHourlyExceeded
	}

	// Generate 6-digit random code: 100000..999999
	n, err := rand.Int(rand.Reader, big.NewInt(900000))
	if err != nil {
		return "", 0, fmt.Errorf("mailer: generate random OTP: %w", err)
	}
	code := fmt.Sprintf("%06d", n.Int64()+100000)

	if ttl <= 0 {
		ttl = 15 * time.Minute
	}

	now := time.Now()
	e.codes[key] = otpEntry{
		Code:      code,
		ExpiresAt: now.Add(ttl),
		Attempts:  0,
	}
	e.cooldowns[key] = now.Add(60 * time.Second)
	e.hourlySends[key] = append(recentSends, now)

	return code, 60, nil
}

// VerifyOTP validates a submitted 6-digit code against the stored OTP in constant-time.
// On success, the OTP is consumed and removed.
func (e *OTPEngine) VerifyOTP(email, purpose, submittedCode string) (bool, error) {
	cleanEmail := identity.NormalizeEmail(email)
	if cleanEmail == "" {
		return false, errors.New("mailer: email cannot be empty")
	}
	key := purpose + ":" + cleanEmail

	e.mu.Lock()
	defer e.mu.Unlock()

	entry, ok := e.codes[key]
	if !ok {
		return false, ErrOTPNotFound
	}

	if time.Now().After(entry.ExpiresAt) {
		delete(e.codes, key)
		return false, ErrOTPExpired
	}

	if entry.Attempts >= 5 {
		delete(e.codes, key)
		return false, ErrOTPMaxAttempts
	}

	// Constant-time string comparison
	codeA := []byte(entry.Code)
	codeB := []byte(submittedCode)
	match := subtle.ConstantTimeCompare(codeA, codeB) == 1

	if !match {
		entry.Attempts++
		e.codes[key] = entry
		return false, ErrOTPInvalidCode
	}

	// Code is valid; consume it
	delete(e.codes, key)
	return true, nil
}

func (e *OTPEngine) pruneExpired() {
	now := time.Now()
	for k, v := range e.codes {
		if now.After(v.ExpiresAt) {
			delete(e.codes, k)
		}
	}
	for k, exp := range e.cooldowns {
		if now.After(exp) {
			delete(e.cooldowns, k)
		}
	}
}
