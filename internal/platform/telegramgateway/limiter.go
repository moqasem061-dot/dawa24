package telegramgateway

import (
	"sync"
	"time"
)

const maxOTPEntries = 4096

type verifyAttempts struct {
	count     int
	expiresAt time.Time
}

// OTPRateLimiter keeps send reservations and verification attempts bounded.
type OTPRateLimiter struct {
	mu          sync.Mutex
	cooldowns   map[string]time.Time
	hourlySends map[string][]time.Time
	attempts    map[string]verifyAttempts
}

func NewLimiter() *OTPRateLimiter {
	return &OTPRateLimiter{
		cooldowns:   make(map[string]time.Time),
		hourlySends: make(map[string][]time.Time),
		attempts:    make(map[string]verifyAttempts),
	}
}

// ReserveSend atomically reserves one of five hourly sends and its cooldown.
// Failed upstream calls retain the reservation to avoid retry storms during outages.
func (l *OTPRateLimiter) ReserveSend(phone string) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	l.clean(now)
	if expiry := l.cooldowns[phone]; expiry.After(now) {
		return int(expiry.Sub(now).Seconds()) + 1, ErrRateLimited
	}
	if len(l.hourlySends[phone]) >= 5 || (l.hourlySends[phone] == nil && len(l.hourlySends) >= maxOTPEntries) {
		return 0, ErrRateLimited
	}
	l.cooldowns[phone] = now.Add(time.Minute)
	l.hourlySends[phone] = append(l.hourlySends[phone], now)
	return 0, nil
}

func (l *OTPRateLimiter) CheckAndRecordVerifyAttempt(requestID string, maxAttempts int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	l.clean(now)
	entry, exists := l.attempts[requestID]
	if !exists {
		if len(l.attempts) >= maxOTPEntries {
			return false
		}
		entry.expiresAt = now.Add(10 * time.Minute)
	}
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	if entry.count >= maxAttempts {
		return false
	}
	entry.count++
	l.attempts[requestID] = entry
	return true
}

// ConsumeRequest retains a spent request until its verification window expires.
func (l *OTPRateLimiter) ConsumeRequest(requestID string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry, exists := l.attempts[requestID]
	if !exists || entry.count > 5 || !entry.expiresAt.After(time.Now()) {
		return false
	}
	entry.count = 6
	l.attempts[requestID] = entry
	return true
}

func (l *OTPRateLimiter) clean(now time.Time) {
	for phone, expiry := range l.cooldowns {
		if !expiry.After(now) {
			delete(l.cooldowns, phone)
		}
	}
	cutoff := now.Add(-time.Hour)
	for phone, sends := range l.hourlySends {
		first := 0
		for first < len(sends) && !sends[first].After(cutoff) {
			first++
		}
		if first == len(sends) {
			delete(l.hourlySends, phone)
		} else {
			l.hourlySends[phone] = sends[first:]
		}
	}
	for id, entry := range l.attempts {
		if !entry.expiresAt.After(now) {
			delete(l.attempts, id)
		}
	}
}
