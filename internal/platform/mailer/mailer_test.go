package mailer

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/config"
)

func TestMockMailer(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	m := New(config.SMTP{Enabled: false}, log)
	if !m.IsMock() {
		t.Fatalf("expected mock mailer")
	}

	ctx := context.Background()
	if err := m.SendOTP(ctx, "test@dawa24.eg", "123456", "password_reset", "ar"); err != nil {
		t.Fatalf("send OTP failed: %v", err)
	}

	if err := m.SendNotification(ctx, "test@dawa24.eg", "تنبيه جديد", "تم استلام طلب جديد", "/orders/1", "ar"); err != nil {
		t.Fatalf("send notification failed: %v", err)
	}
}

func TestEmailTokenSignAndVerify(t *testing.T) {
	secret := "test-secret-key-32-bytes-minimum-ok"
	email := "Doctor.Ahmed@Dawa24.eg"

	token, err := SignEmailToken(email, secret, 10*time.Minute)
	if err != nil {
		t.Fatalf("sign email token failed: %v", err)
	}

	// Verify matching email (case insensitive)
	valid, err := VerifyEmailToken(token, "doctor.ahmed@dawa24.eg", secret)
	if err != nil || !valid {
		t.Fatalf("verify failed: valid=%v, err=%v", valid, err)
	}

	// Verify email mismatch
	valid, err = VerifyEmailToken(token, "other@dawa24.eg", secret)
	if valid || err == nil {
		t.Fatalf("expected mismatch error, got valid=%v", valid)
	}

	// Verify wrong secret
	valid, err = VerifyEmailToken(token, email, "wrong-secret-key")
	if valid || err == nil {
		t.Fatalf("expected signature error with wrong secret")
	}
}

func TestPasswordResetTokenSignAndVerify(t *testing.T) {
	secret := "test-secret-key-32-bytes-minimum-ok"
	email := "pharmacist@dawa24.eg"

	token, err := SignPasswordResetToken(email, secret, 10*time.Minute)
	if err != nil {
		t.Fatalf("sign password reset token failed: %v", err)
	}

	extractedEmail, err := VerifyPasswordResetToken(token, secret)
	if err != nil {
		t.Fatalf("verify password reset token failed: %v", err)
	}
	if extractedEmail != email {
		t.Fatalf("expected %s, got %s", email, extractedEmail)
	}

	// Email verify token cannot be used for password reset
	emailToken, _ := SignEmailToken(email, secret, 10*time.Minute)
	_, err = VerifyPasswordResetToken(emailToken, secret)
	if err == nil {
		t.Fatalf("expected purpose mismatch error")
	}
}

func TestOTPEngine(t *testing.T) {
	engine := NewOTPEngine()
	email := "test@dawa24.net"
	purpose := "password_reset"

	code, cooldown, err := engine.GenerateOTP(email, purpose, 5*time.Minute)
	if err != nil {
		t.Fatalf("generate OTP failed: %v", err)
	}
	if len(code) != 6 || cooldown != 60 {
		t.Fatalf("unexpected code %s or cooldown %d", code, cooldown)
	}

	// Should be in cooldown immediately
	_, _, err = engine.GenerateOTP(email, purpose, 5*time.Minute)
	if err != ErrOTPInCooldown {
		t.Fatalf("expected cooldown error, got %v", err)
	}

	// Wrong code should fail
	ok, err := engine.VerifyOTP(email, purpose, "000000")
	if ok || err != ErrOTPInvalidCode {
		t.Fatalf("expected invalid code error, got %v", err)
	}

	// Correct code should succeed
	ok, err = engine.VerifyOTP(email, purpose, code)
	if !ok || err != nil {
		t.Fatalf("expected OTP verify success, got ok=%v, err=%v", ok, err)
	}

	// Reusing code should fail
	ok, err = engine.VerifyOTP(email, purpose, code)
	if ok || err != ErrOTPNotFound {
		t.Fatalf("expected OTP not found on second attempt, got %v", err)
	}
}
