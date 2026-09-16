package telegramgateway

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestNormalizePhone(t *testing.T) {
	tests := []struct {
		input    string
		expected string
		wantErr  bool
	}{
		// Egyptian standard formats
		{"01012345678", "+201012345678", false},
		{"01123456789", "+201123456789", false},
		{"01234567890", "+201234567890", false},
		{"01555555555", "+201555555555", false},
		// With dashes, spaces, brackets
		{"010-1234-5678", "+201012345678", false},
		{"010 1234 5678", "+201012345678", false},
		{"(010) 12345678", "+201012345678", false},
		// Arabic-Indic digits
		{"٠١٠١٢٣٤٥٦٧٨", "+201012345678", false},
		// Already with country code
		{"+201012345678", "+201012345678", false},
		{"201012345678", "+201012345678", false},
		{"00201012345678", "+201012345678", false},
		// International
		{"+966501234567", "+966501234567", false},
		{"00966501234567", "+966501234567", false},
		// Invalid formats
		{"123", "", true},
		{"abcd", "", true},
		{"01812345678", "", true}, // 018 is not an Egyptian mobile prefix (only 010, 011, 012, 015)
		{"", "", true},
	}

	for _, tt := range tests {
		got, err := NormalizePhone(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("NormalizePhone(%q) err = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if got != tt.expected {
			t.Errorf("NormalizePhone(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestSignAndVerifyPhoneToken(t *testing.T) {
	secret := "test_super_secret_key_1234567890123456"
	phone := "01012345678"

	token, err := SignPhoneToken(phone, secret, 10*time.Minute)
	if err != nil {
		t.Fatalf("SignPhoneToken failed: %v", err)
	}

	// 1. Valid verification with matching phone
	valid, err := VerifyPhoneToken(token, "+201012345678", secret)
	if err != nil || !valid {
		t.Fatalf("VerifyPhoneToken failed: valid=%v, err=%v", valid, err)
	}

	// 2. Verification with local format (should normalize and match)
	valid, err = VerifyPhoneToken(token, "01012345678", secret)
	if err != nil || !valid {
		t.Fatalf("VerifyPhoneToken with local format failed: valid=%v, err=%v", valid, err)
	}

	// 3. Verification with different phone (tampering attempt)
	valid, err = VerifyPhoneToken(token, "01099999999", secret)
	if valid || err != ErrTokenPhoneMismatch {
		t.Fatalf("Expected ErrTokenPhoneMismatch, got valid=%v, err=%v", valid, err)
	}

	// 4. Verification with altered token (tampering payload)
	parts := strings.Split(token, ".")
	tamperedToken := "e30." + parts[1] // changed payload to "{}"
	valid, err = VerifyPhoneToken(tamperedToken, phone, secret)
	if valid || err != ErrTokenSignature {
		t.Fatalf("Expected ErrTokenSignature on tampered token, got valid=%v, err=%v", valid, err)
	}

	// 5. Verification with wrong secret
	valid, err = VerifyPhoneToken(token, phone, "wrong_secret_key_1234567890123456")
	if valid || err != ErrTokenSignature {
		t.Fatalf("Expected ErrTokenSignature on wrong secret, got valid=%v, err=%v", valid, err)
	}

	// 6. Expired token
	expiredToken, err := SignPhoneToken(phone, secret, -1*time.Minute)
	if err != nil {
		t.Fatalf("SignPhoneToken failed: %v", err)
	}
	valid, err = VerifyPhoneToken(expiredToken, phone, secret)
	if valid || err != ErrTokenExpired {
		t.Fatalf("Expected ErrTokenExpired, got valid=%v, err=%v", valid, err)
	}
}

func TestMockClientSendAndCheck(t *testing.T) {
	ctx := context.Background()
	client := New(Config{MockMode: true})

	if !client.IsMock() {
		t.Fatal("Expected client.IsMock() to be true")
	}

	// Send OTP
	res, err := client.SendVerification(ctx, "01012345678")
	if err != nil {
		t.Fatalf("SendVerification failed: %v", err)
	}

	if res.PhoneNumber != "+201012345678" {
		t.Fatalf("Expected +201012345678, got %q", res.PhoneNumber)
	}
	if res.RequestID == "" {
		t.Fatal("Expected non-empty RequestID")
	}

	// Check with invalid code
	checkRes, err := client.CheckVerification(ctx, res.RequestID, "999999")
	if err != nil {
		t.Fatalf("CheckVerification unexpected err: %v", err)
	}
	if checkRes.Valid {
		t.Fatal("Expected checkRes.Valid to be false for wrong code")
	}

	// Check with valid mock code
	checkRes, err = client.CheckVerification(ctx, res.RequestID, "123456")
	if err != nil {
		t.Fatalf("CheckVerification unexpected err: %v", err)
	}
	if !checkRes.Valid {
		t.Fatal("Expected checkRes.Valid to be true for 123456")
	}
}

func TestLimiterCooldown(t *testing.T) {
	limiter := NewLimiter()
	phone := "+201012345678"

	inCd, _ := limiter.CheckSendCooldown(phone)
	if inCd {
		t.Fatal("Expected no cooldown initially")
	}

	limiter.RecordSend(phone, 2*time.Second)

	inCd, remaining := limiter.CheckSendCooldown(phone)
	if !inCd || remaining <= 0 {
		t.Fatalf("Expected in cooldown, got inCd=%v, remaining=%d", inCd, remaining)
	}

	// Max hourly sends
	for i := 0; i < 4; i++ {
		limiter.RecordSend(phone, 2*time.Second)
	}
	exceeded := limiter.CheckHourlySendLimit(phone, 5)
	if !exceeded {
		t.Fatal("Expected hourly limit exceeded after 5 sends")
	}
}
