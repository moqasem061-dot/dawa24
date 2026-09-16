package identity

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// MFAStatus represents the current two-factor authentication state for a user.
type MFAStatus struct {
	UserID             int64      `json:"user_id"`
	Enabled            bool       `json:"enabled"`
	ConfirmedAt        *time.Time `json:"confirmed_at,omitempty"`
	HasRecoveryCodes   bool       `json:"has_recovery_codes"`
	RecoveryCodesCount int        `json:"recovery_codes_count"`
}

// MFASetupData holds setup credentials including the QR code data URI and manual secret key.
type MFASetupData struct {
	Secret        string `json:"secret"`
	OTPAuthURL    string `json:"otpauth_url"`
	QRCodeDataURI string `json:"qr_code_data_uri"`
}

// MFAActivationResult carries the generated single-use backup recovery codes upon activation.
type MFAActivationResult struct {
	Success       bool     `json:"success"`
	RecoveryCodes []string `json:"recovery_codes"`
}

// GetMFAStatus returns the active MFA configuration and backup code stats for a user.
func (s *Service) GetMFAStatus(ctx context.Context, userID int64) (*MFAStatus, error) {
	if userID <= 0 {
		return nil, apperr.Validation("user_id.invalid", "Valid user ID is required.", nil)
	}

	mfa, err := s.repo.GetMFA(ctx, userID)
	if err != nil {
		return nil, err
	}

	status := &MFAStatus{
		UserID:  userID,
		Enabled: mfa != nil && mfa.Enabled,
	}

	if mfa != nil && mfa.ConfirmedAt != nil {
		status.ConfirmedAt = mfa.ConfirmedAt
	}

	if mfa != nil && len(mfa.RecoveryCodes) > 0 {
		var codes []string
		if err := json.Unmarshal(mfa.RecoveryCodes, &codes); err == nil {
			status.HasRecoveryCodes = len(codes) > 0
			status.RecoveryCodesCount = len(codes)
		}
	}

	return status, nil
}

// SetupMFA generates a new TOTP secret, QR code, and staging record for initial confirmation.
func (s *Service) SetupMFA(ctx context.Context, userID int64, email string) (*MFASetupData, error) {
	if userID <= 0 {
		return nil, apperr.Validation("user_id.invalid", "Valid user ID is required.", nil)
	}

	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if email == "" {
		email = user.Email
	}

	secret, err := GenerateTOTPSecret()
	if err != nil {
		return nil, err
	}

	otpauthURL := GenerateOTPAuthURL(email, secret)
	qrDataURI, err := GenerateQRCodeDataURI(otpauthURL)
	if err != nil {
		return nil, err
	}

	// Store the pending secret (enabled = false, confirmed_at = nil)
	pendingMFA := &UserMFA{
		UserID:        userID,
		TOTPSecret:    []byte(secret),
		RecoveryCodes: nil,
		Enabled:       false,
		ConfirmedAt:   nil,
	}
	if err := s.repo.UpsertMFA(ctx, pendingMFA); err != nil {
		return nil, fmt.Errorf("store pending mfa secret: %w", err)
	}

	s.log.InfoContext(ctx, "mfa setup initiated", "user_id", userID, "email", email)

	return &MFASetupData{
		Secret:        secret,
		OTPAuthURL:    otpauthURL,
		QRCodeDataURI: qrDataURI,
	}, nil
}

// ConfirmEnableMFA verifies the initial 6-digit TOTP code and activates MFA for the account.
func (s *Service) ConfirmEnableMFA(ctx context.Context, userID int64, code string) (*MFAActivationResult, error) {
	if userID <= 0 {
		return nil, apperr.Validation("user_id.invalid", "Valid user ID is required.", nil)
	}

	cleanCode := strings.TrimSpace(code)
	if len(cleanCode) != TOTPDigits {
		return nil, apperr.Validation("mfa.code_invalid", i18n.T("ar", "mfa.err.code_len"), nil)
	}

	mfa, err := s.repo.GetMFA(ctx, userID)
	if err != nil || mfa == nil || len(mfa.TOTPSecret) == 0 {
		return nil, apperr.Validation("mfa.not_setup", i18n.T("ar", "mfa.err.not_setup"), nil)
	}

	secret := string(mfa.TOTPSecret)
	now := time.Now().UTC()
	if !ValidateTOTP(secret, cleanCode, now) {
		return nil, apperr.Validation("mfa.code_incorrect", i18n.T("ar", "mfa.err.code_incorrect"), nil)
	}

	// Generate 8 backup recovery codes
	recoveryCodes, err := GenerateRecoveryCodes(8)
	if err != nil {
		return nil, fmt.Errorf("generate recovery codes: %w", err)
	}

	recoveryBytes, err := json.Marshal(recoveryCodes)
	if err != nil {
		return nil, err
	}

	mfa.Enabled = true
	mfa.ConfirmedAt = &now
	mfa.RecoveryCodes = recoveryBytes

	if err := s.repo.UpsertMFA(ctx, mfa); err != nil {
		return nil, fmt.Errorf("activate mfa: %w", err)
	}

	s.log.InfoContext(ctx, "mfa successfully enabled", "user_id", userID)

	return &MFAActivationResult{
		Success:       true,
		RecoveryCodes: recoveryCodes,
	}, nil
}

// DisableMFA disables two-factor authentication after verifying the user's password.
func (s *Service) DisableMFA(ctx context.Context, userID int64, password, code string) error {
	if userID <= 0 {
		return apperr.Validation("user_id.invalid", "Valid user ID is required.", nil)
	}

	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}

	// Password verification is required to disable MFA
	if password != "" {
		if !CheckPassword(user.PasswordHash, password) {
			return apperr.Validation("password.incorrect", i18n.T("ar", "mfa.err.password_incorrect"), nil)
		}
	} else if code != "" {
		// Alternatively allow current valid TOTP code
		mfa, err := s.repo.GetMFA(ctx, userID)
		if err != nil || mfa == nil || !mfa.Enabled {
			return apperr.Validation("mfa.not_enabled", i18n.TDefault("w4_mod.w4str_184_184"), nil)
		}
		if !ValidateTOTP(string(mfa.TOTPSecret), code, time.Now().UTC()) {
			return apperr.Validation("mfa.code_incorrect", i18n.TDefault("w4_mod.w4str_185_185"), nil)
		}
	} else {
		return apperr.Validation("auth.confirmation_required", i18n.TDefault("w4_mod.w4str_186_186"), nil)
	}

	disabledMFA := &UserMFA{
		UserID:        userID,
		TOTPSecret:    nil,
		RecoveryCodes: nil,
		Enabled:       false,
		ConfirmedAt:   nil,
	}

	if err := s.repo.UpsertMFA(ctx, disabledMFA); err != nil {
		return fmt.Errorf("disable mfa: %w", err)
	}

	s.log.InfoContext(ctx, "mfa disabled by user", "user_id", userID)
	return nil
}

// VerifyMFA checks whether a code is a valid 6-digit TOTP code or an unused recovery code.
func (s *Service) VerifyMFA(ctx context.Context, userID int64, code string) (bool, error) {
	if userID <= 0 {
		return false, apperr.Validation("user_id.invalid", "Valid user ID is required.", nil)
	}

	mfa, err := s.repo.GetMFA(ctx, userID)
	if err != nil || mfa == nil || !mfa.Enabled || len(mfa.TOTPSecret) == 0 {
		return false, apperr.Validation("mfa.not_enabled", i18n.TDefault("w4_mod.w4str_187_187"), nil)
	}

	cleanCode := strings.ToUpper(strings.TrimSpace(code))
	cleanCodeDigits := strings.ReplaceAll(cleanCode, "-", "")

	// 1. Try standard 6-digit TOTP code
	if len(cleanCodeDigits) == TOTPDigits {
		if ValidateTOTP(string(mfa.TOTPSecret), cleanCodeDigits, time.Now().UTC()) {
			return true, nil
		}
	}

	// 2. Try single-use recovery code
	if len(mfa.RecoveryCodes) > 0 {
		var existingCodes []string
		if err := json.Unmarshal(mfa.RecoveryCodes, &existingCodes); err == nil && len(existingCodes) > 0 {
			for i, rCode := range existingCodes {
				cleanRCode := strings.ToUpper(strings.TrimSpace(rCode))
				if subtle.ConstantTimeCompare([]byte(cleanCode), []byte(cleanRCode)) == 1 ||
					subtle.ConstantTimeCompare([]byte(cleanCodeDigits), []byte(strings.ReplaceAll(cleanRCode, "-", ""))) == 1 {
					// Consume this recovery code
					updatedCodes := append(existingCodes[:i], existingCodes[i+1:]...)
					updatedBytes, _ := json.Marshal(updatedCodes)
					mfa.RecoveryCodes = updatedBytes
					_ = s.repo.UpsertMFA(ctx, mfa)
					s.log.WarnContext(ctx, "recovery code consumed for login", "user_id", userID, "remaining_codes", len(updatedCodes))
					return true, nil
				}
			}
		}
	}

	return false, nil
}

// CompleteMFALogin generates a full authenticated session for a user who passed MFA challenge.
func (s *Service) CompleteMFALogin(ctx context.Context, userID, orgID int64, ip, userAgent string) (*Session, error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if err := user.ValidateLogin(); err != nil {
		return nil, err
	}

	// Reset failed attempts upon successful MFA completion
	if sec, err := s.repo.GetSecurity(ctx, user.ID); err == nil && sec != nil {
		sec.ResetAttempts(time.Now().UTC(), ip, userAgent)
		_ = s.repo.UpsertSecurity(ctx, sec)
	}

	var orgType, orgStatus string
	if orgID > 0 {
		belongs, _ := s.repo.UserBelongsToOrg(ctx, user.ID, orgID)
		if !belongs {
			orgID = 0
		}
	}
	if orgID == 0 {
		if o, t, st, err := s.repo.DefaultOrgInfoForUser(ctx, user.ID); err == nil {
			orgID, orgType, orgStatus = o, t, st
		}
	}

	if orgType != "" {
		normType, _ := NormalizeOrgType(orgType)
		orgType = normType
	}

	permissions, _ := s.resolveGrant(ctx, user.ID, orgID)

	maxSessions := 3
	if maxS, _, _, err := s.repo.GetOrgPlanLimits(ctx, orgID); err == nil && maxS > 0 {
		maxSessions = maxS
	}

	dev := ParseUserAgentDevice(userAgent, ip)

	sess := &Session{
		UserID:           user.ID,
		PublicID:         user.PublicID,
		Email:            user.Email,
		Role:             user.Role,
		ActiveOrgID:      orgID,
		OrgType:          orgType,
		OrgStatus:        orgStatus,
		Permissions:      permissions,
		MaxLoginSessions: &maxSessions,
		IP:               ip,
		UserAgent:        userAgent,
		DeviceName:       dev.DeviceName,
		DeviceType:       dev.DeviceType,
		Browser:          dev.Browser,
		OS:               dev.OS,
		Icon:             dev.Icon,
	}

	if s.sessionStore != nil {
		if err := s.sessionStore.Create(ctx, sess); err != nil {
			return nil, err
		}
	}

	s.log.InfoContext(ctx, "mfa login completed successfully", "user_id", user.ID, "email", user.Email, "org_id", orgID)
	return sess, nil
}

// RegenerateRecoveryCodes creates a fresh set of 8 recovery codes for the user if password is correct.
func (s *Service) RegenerateRecoveryCodes(ctx context.Context, userID int64, password string) ([]string, error) {
	if userID <= 0 {
		return nil, apperr.Validation("user_id.invalid", "Valid user ID is required.", nil)
	}

	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	if !CheckPassword(user.PasswordHash, password) {
		return nil, apperr.Validation("password.incorrect", i18n.T("ar", "mfa.err.password_incorrect"), nil)
	}

	mfa, err := s.repo.GetMFA(ctx, userID)
	if err != nil || mfa == nil || !mfa.Enabled {
		return nil, apperr.Validation("mfa.not_enabled", i18n.TDefault("w4_mod.w4str_184_184"), nil)
	}

	newCodes, err := GenerateRecoveryCodes(8)
	if err != nil {
		return nil, err
	}

	codesBytes, err := json.Marshal(newCodes)
	if err != nil {
		return nil, err
	}

	mfa.RecoveryCodes = codesBytes
	if err := s.repo.UpsertMFA(ctx, mfa); err != nil {
		return nil, fmt.Errorf("save recovery codes: %w", err)
	}

	s.log.InfoContext(ctx, "recovery codes regenerated", "user_id", userID)
	return newCodes, nil
}
