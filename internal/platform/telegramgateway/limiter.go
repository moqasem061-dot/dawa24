package telegramgateway

import (
	"sync"
	"time"
)

// OTPRateLimiter manages cooldowns and attempt counts for phone OTP operations.
type OTPRateLimiter struct {
	mu           sync.Mutex
	cooldowns    map[string]time.Time // normalized phone -> cooldown expiry
	hourlySends  map[string][]time.Time // normalized phone -> timestamps of sends
	attempts     map[string]int       // requestID -> failed attempt count
	lastCleaned  time.Time
}

// NewLimiter creates a new in-memory OTPRateLimiter.
func NewLimiter() *OTPRateLimiter {
	return &OTPRateLimiter{
		cooldowns:   make(map[string]time.Time),
		hourlySends: make(map[string][]time.Time),
		attempts:    make(map[string]int),
		lastCleaned: time.Now(),
	}
}

// CheckSendCooldown checks if a phone number is currently in cooldown.
// Returns (inCooldown, remainingSeconds).
func (l *OTPRateLimiter) CheckSendCooldown(normPhone string) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.maybeClean()

	expiry, ok := l.cooldowns[normPhone]
	if !ok {
		return false, 0
	}

	remaining := time.Until(expiry)
	if remaining <= 0 {
		delete(l.cooldowns, normPhone)
		return false, 0
	}

	return true, int(remaining.Seconds()) + 1
}

// CheckHourlySendLimit checks if phone has sent too many OTPs in the last hour.
// Max 5 sends per hour.
func (l *OTPRateLimiter) CheckHourlySendLimit(normPhone string, maxPerHour int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.maybeClean()

	if maxPerHour <= 0 {
		maxPerHour = 5
	}

	cutoff := time.Now().Add(-1 * time.Hour)
	timestamps := l.hourlySends[normPhone]

	valid := make([]time.Time, 0, len(timestamps))
	for _, t := range timestamps {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	l.hourlySends[normPhone] = valid

	return len(valid) >= maxPerHour
}

// RecordSend records a successful OTP send, starting a 60s cooldown and incrementing the hourly count.
func (l *OTPRateLimiter) RecordSend(normPhone string, cooldownDuration time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if cooldownDuration <= 0 {
		cooldownDuration = 60 * time.Second
	}

	now := time.Now()
	l.cooldowns[normPhone] = now.Add(cooldownDuration)
	l.hourlySends[normPhone] = append(l.hourlySends[normPhone], now)
}

// CheckAndRecordVerifyAttempt checks if verification attempts for requestID have exceeded maxAttempts (default 5).
// Returns true if attempts are still allowed, false if limit exceeded.
func (l *OTPRateLimiter) CheckAndRecordVerifyAttempt(requestID string, maxAttempts int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if maxAttempts <= 0 {
		maxAttempts = 5
	}

	curr := l.attempts[requestID]
	if curr >= maxAttempts {
		return false
	}

	l.attempts[requestID] = curr + 1
	return true
}

// ClearRequest removes a requestID from tracking on successful verification.
func (l *OTPRateLimiter) ClearRequest(requestID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, requestID)
}

func (l *OTPRateLimiter) maybeClean() {
	now := time.Now()
	if now.Sub(l.lastCleaned) < 10*time.Minute {
		return
	}
	l.lastCleaned = now

	// Clean expired cooldowns
	for phone, exp := range l.cooldowns {
		if now.After(exp) {
			delete(l.cooldowns, phone)
		}
	}

	// Clean old hourly sends
	cutoff := now.Add(-1 * time.Hour)
	for phone, times := range l.hourlySends {
		valid := make([]time.Time, 0, len(times))
		for _, t := range times {
			if t.After(cutoff) {
				valid = append(valid, t)
			}
		}
		if len(valid) == 0 {
			delete(l.hourlySends, phone)
		} else {
			l.hourlySends[phone] = valid
		}
	}
}
