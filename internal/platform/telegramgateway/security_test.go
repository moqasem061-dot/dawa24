package telegramgateway

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type gatewayFixtureTransport func(*http.Request) (*http.Response, error)

func (f gatewayFixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestLiveVerificationUsesTelegramStatusAndPhone(t *testing.T) {
	fixtures := []struct {
		name, response string
		valid          bool
		wantError      bool
	}{
		{"valid", `{"ok":true,"result":{"request_id":"request-1","phone_number":"+201012345678","verification_status":{"status":"code_valid"}}}`, true, false},
		{"invalid", `{"ok":true,"result":{"request_id":"request-1","phone_number":"+201012345678","verification_status":{"status":"code_invalid"}}}`, false, false},
		{"expired", `{"ok":true,"result":{"request_id":"request-1","verification_status":{"status":"expired"}}}`, false, false},
		{"mismatched request", `{"ok":true,"result":{"request_id":"request-2","verification_status":{"status":"code_valid"}}}`, false, true},
		{"oversized", strings.Repeat("x", 65537), false, true},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			client := New(Config{Token: "fixture-token", HTTPClient: &http.Client{Transport: gatewayFixtureTransport(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(fixture.response))}, nil
			})}})
			check, err := client.CheckVerification(context.Background(), "request-1", "123456")
			if (err != nil) != fixture.wantError {
				t.Fatalf("error=%v", err)
			}
			if err == nil && check.Valid != fixture.valid {
				t.Fatalf("valid=%v", check.Valid)
			}
			if fixture.valid && check.PhoneNumber != "+201012345678" {
				t.Fatal("lost verified phone")
			}
		})
	}
}

func TestMockRejectsForgedAndSpentRequests(t *testing.T) {
	client := New(Config{MockMode: true})
	for _, id := range []string{"mock_forged", "mock_req_1"} {
		check, err := client.CheckVerification(context.Background(), id, "123456")
		if err != nil || check.Valid {
			t.Fatalf("forged request accepted: %v %v", check, err)
		}
	}
	sent, err := client.SendVerification(context.Background(), "01012345678")
	if err != nil {
		t.Fatal(err)
	}
	check, err := client.CheckVerification(context.Background(), sent.RequestID, "123456")
	if err != nil || !check.Valid || check.PhoneNumber != sent.PhoneNumber {
		t.Fatalf("valid request: %v %v", check, err)
	}
	check, err = client.CheckVerification(context.Background(), sent.RequestID, "123456")
	if err != nil || check.Valid {
		t.Fatalf("spent request accepted: %v %v", check, err)
	}
}

func TestLimiterReservationsAndConsumptionAreAtomic(t *testing.T) {
	limiter := NewLimiter()
	var accepted atomic.Int32
	var workers sync.WaitGroup
	for range 20 {
		workers.Go(func() {
			if _, err := limiter.ReserveSend("+201012345678"); err == nil {
				accepted.Add(1)
			}
		})
	}
	workers.Wait()
	if accepted.Load() != 1 {
		t.Fatalf("parallel sends=%d", accepted.Load())
	}
	if !limiter.CheckAndRecordVerifyAttempt("request", 5) {
		t.Fatal("new request refused")
	}
	accepted.Store(0)
	for range 20 {
		workers.Go(func() {
			if limiter.ConsumeRequest("request") {
				accepted.Add(1)
			}
		})
	}
	workers.Wait()
	if accepted.Load() != 1 || limiter.CheckAndRecordVerifyAttempt("request", 5) {
		t.Fatal("spent request reusable")
	}
}

func TestLimiterMemoryIsBoundedAndExpiredEntriesReclaimed(t *testing.T) {
	limiter := NewLimiter()
	for i := range maxOTPEntries {
		if !limiter.CheckAndRecordVerifyAttempt(fmt.Sprint(i), 5) {
			t.Fatal("capacity reduced unexpectedly")
		}
	}
	if limiter.CheckAndRecordVerifyAttempt("overflow", 5) {
		t.Fatal("attempt map exceeded bound")
	}
	limiter.attempts["0"] = verifyAttempts{expiresAt: time.Now().Add(-time.Second)}
	if !limiter.CheckAndRecordVerifyAttempt("overflow", 5) || len(limiter.attempts) != maxOTPEntries {
		t.Fatal("expired slot not reclaimed")
	}
}
