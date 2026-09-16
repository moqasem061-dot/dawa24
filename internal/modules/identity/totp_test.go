package identity

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"
)

func TestTOTP_GenerateAndValidate(t *testing.T) {
	secret, err := GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("GenerateTOTPSecret failed: %v", err)
	}
	if len(secret) < 16 {
		t.Fatalf("secret too short: %s", secret)
	}

	now := time.Now().UTC()
	code, err := GenerateTOTPCode(secret, now)
	if err != nil {
		t.Fatalf("GenerateTOTPCode failed: %v", err)
	}
	if len(code) != 6 {
		t.Fatalf("expected 6-digit code, got %s", code)
	}

	// 1. Current time should validate
	if !ValidateTOTP(secret, code, now) {
		t.Errorf("ValidateTOTP failed for current timestamp")
	}

	// 2. Skew -25 seconds (within ±30s window) should validate
	if !ValidateTOTP(secret, code, now.Add(25*time.Second)) {
		t.Errorf("ValidateTOTP failed for skew +25s")
	}

	// 3. Skew -90 seconds (outside window) should fail
	if ValidateTOTP(secret, code, now.Add(90*time.Second)) {
		t.Errorf("ValidateTOTP should fail for skew +90s")
	}

	// 4. Invalid codes
	if ValidateTOTP(secret, "000000", now) && code != "000000" {
		t.Errorf("ValidateTOTP accepted random wrong code")
	}
	if ValidateTOTP(secret, "123", now) {
		t.Errorf("ValidateTOTP accepted short code")
	}
	if ValidateTOTP(secret, "abcdef", now) {
		t.Errorf("ValidateTOTP accepted alphabetic code")
	}
}

func TestTOTP_RecoveryCodes(t *testing.T) {
	codes, err := GenerateRecoveryCodes(8)
	if err != nil {
		t.Fatalf("GenerateRecoveryCodes failed: %v", err)
	}
	if len(codes) != 8 {
		t.Fatalf("expected 8 recovery codes, got %d", len(codes))
	}
	for _, c := range codes {
		if len(c) != 9 || c[4] != '-' {
			t.Errorf("malformed recovery code format: %s", c)
		}
	}
}

func TestTOTP_QRCodeGeneration(t *testing.T) {
	url := GenerateOTPAuthURL("user@dawa24.eg", "JBSWY3DPEHPK3PXP")
	if url == "" {
		t.Fatal("empty otpauth url")
	}
	dataURI, err := GenerateQRCodeDataURI(url)
	if err != nil {
		t.Fatalf("GenerateQRCodeDataURI failed: %v", err)
	}
	if len(dataURI) < 50 || dataURI[:22] != "data:image/png;base64," {
		t.Fatalf("malformed QR data uri: %s", dataURI)
	}
}

func TestMFA_ServiceFullLifecycle(t *testing.T) {
	ctx := context.Background()
	repo := newMockRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(repo, nil, logger)

	// Register user
	user, _, err := svc.Register(ctx, RegisterInput{
		Email:    "mfa-lifecycle@dawa24.eg",
		Password: "SecurePassword123!",
		NameAr:   "مستخدم تجريبي",
		NameEn:   "Test User",
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Initial status should be disabled
	status, err := svc.GetMFAStatus(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetMFAStatus failed: %v", err)
	}
	if status.Enabled {
		t.Errorf("expected MFA to be disabled initially")
	}

	// 1. Initiate setup
	setupData, err := svc.SetupMFA(ctx, user.ID, user.Email)
	if err != nil {
		t.Fatalf("SetupMFA failed: %v", err)
	}
	if setupData.Secret == "" || setupData.QRCodeDataURI == "" {
		t.Fatalf("incomplete setup data: %+v", setupData)
	}

	// 2. Generate valid TOTP code and confirm
	validCode, _ := GenerateTOTPCode(setupData.Secret, time.Now().UTC())
	actResult, err := svc.ConfirmEnableMFA(ctx, user.ID, validCode)
	if err != nil {
		t.Fatalf("ConfirmEnableMFA failed: %v", err)
	}
	if !actResult.Success || len(actResult.RecoveryCodes) != 8 {
		t.Fatalf("expected 8 recovery codes, got %d", len(actResult.RecoveryCodes))
	}

	// 3. Status should now be enabled
	status, err = svc.GetMFAStatus(ctx, user.ID)
	if err != nil || !status.Enabled || status.ConfirmedAt == nil {
		t.Fatalf("expected MFA to be enabled, status: %+v", status)
	}
	if status.RecoveryCodesCount != 8 {
		t.Errorf("expected 8 recovery codes count, got %d", status.RecoveryCodesCount)
	}

	// 4. Verify MFA with TOTP code
	freshCode, _ := GenerateTOTPCode(setupData.Secret, time.Now().UTC())
	valid, err := svc.VerifyMFA(ctx, user.ID, freshCode)
	if err != nil || !valid {
		t.Fatalf("VerifyMFA with TOTP code failed: valid=%v, err=%v", valid, err)
	}

	// 5. Verify MFA with single-use Recovery Code
	firstRecoveryCode := actResult.RecoveryCodes[0]
	validRec, err := svc.VerifyMFA(ctx, user.ID, firstRecoveryCode)
	if err != nil || !validRec {
		t.Fatalf("VerifyMFA with recovery code failed: valid=%v, err=%v", validRec, err)
	}

	// Consumed recovery code should now fail
	usedRec, _ := svc.VerifyMFA(ctx, user.ID, firstRecoveryCode)
	if usedRec {
		t.Errorf("reusing already consumed recovery code should fail")
	}

	// Recovery code count should now be 7
	status, _ = svc.GetMFAStatus(ctx, user.ID)
	if status.RecoveryCodesCount != 7 {
		t.Errorf("expected 7 recovery codes remaining, got %d", status.RecoveryCodesCount)
	}

	// 6. Regenerate recovery codes
	newCodes, err := svc.RegenerateRecoveryCodes(ctx, user.ID, "SecurePassword123!")
	if err != nil || len(newCodes) != 8 {
		t.Fatalf("RegenerateRecoveryCodes failed: %v", err)
	}

	// 7. Disable MFA with password
	err = svc.DisableMFA(ctx, user.ID, "SecurePassword123!", "")
	if err != nil {
		t.Fatalf("DisableMFA failed: %v", err)
	}

	status, _ = svc.GetMFAStatus(ctx, user.ID)
	if status.Enabled {
		t.Errorf("expected MFA to be disabled after DisableMFA")
	}
}
