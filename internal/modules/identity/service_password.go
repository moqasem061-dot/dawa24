package identity

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/moqasem061-dot/dawa24/internal/platform/database"
	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
	"github.com/moqasem061-dot/dawa24/internal/shared/i18n"
)

// unknownUserHash is a bcrypt hash of a throwaway value, built once and checked
// against when there is no real hash to compare, so a failed login costs the
// same time either way.
var unknownUserHash = sync.OnceValue(func() string {
	hash, err := bcrypt.GenerateFromPassword([]byte("dawa24-no-such-account"), bcrypt.DefaultCost)
	if err != nil {
		return ""
	}
	return string(hash)
})

// ChangePassword verifies current password and updates to the new hashed password.
func (s *Service) ChangePassword(ctx context.Context, userID int64, currentPassword, newPassword string) error {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if !CheckPassword(user.PasswordHash, currentPassword) {
		return apperr.Validation("auth.invalid_password", i18n.TDefault("w4_mod.w4str_182_182"), nil)
	}
	if len(newPassword) < 8 {
		return apperr.Validation("auth.weak_password", i18n.TDefault("w4_mod.8_183"), nil)
	}

	hash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	user.PasswordHash = hash
	user.MustChangePassword = false
	user.UpdatedAt = time.Now()
	if err := s.repo.UpdateUser(ctx, user); err != nil {
		return err
	}
	// Any reset token issued before this change can no longer be used.
	if _, err := s.repo.StampPasswordChange(ctx, userID, user.UpdatedAt, user.UpdatedAt); err != nil {
		s.log.WarnContext(ctx, "stamp password change", "user_id", userID, "error", err)
	}
	if s.sessionStore != nil {
		_ = s.sessionStore.DeleteAllForUser(ctx, userID)
	}

	var pOrgID *int64
	if oID, _, _, err := s.repo.DefaultOrgInfoForUser(ctx, userID); err == nil && oID > 0 {
		pOrgID = &oID
	}
	_ = s.RecordAudit(ctx, database.AuditEntry{
		OrganizationID: pOrgID,
		ActorUserID:    userID,
		Action:         "user.password_changed",
		EntityType:     "user",
		EntityID:       strconv.FormatInt(userID, 10),
		Before:         map[string]any{"password_status": "previous"},
		After:          map[string]any{"password_status": "updated", "changed_at": time.Now().UTC().Format(time.RFC3339)},
	})

	s.log.InfoContext(ctx, "user password updated", "user_id", userID)
	return nil
}

// ErrResetTokenSpent means a password reset token was already used, or a
// password change was recorded after the token was issued.
var ErrResetTokenSpent = errors.New("identity: password reset token already used")

// ResetPassword resets a user's password after an OTP check in settings. The
// settings page has no reset token to consume.
func (s *Service) ResetPassword(ctx context.Context, email, newPassword string) error {
	return s.resetPassword(ctx, email, newPassword, nil)
}

// ResetPasswordWithToken resets a password for a reset token issued at
// tokenIssuedAt. The token works once: the reset records a password change, and
// a token issued before that change is refused.
func (s *Service) ResetPasswordWithToken(ctx context.Context, email, newPassword string, tokenIssuedAt time.Time) error {
	return s.resetPassword(ctx, email, newPassword, &tokenIssuedAt)
}

func (s *Service) resetPassword(ctx context.Context, email, newPassword string, tokenIssuedAt *time.Time) error {
	cleanEmail := NormalizeEmail(email)
	if cleanEmail == "" {
		return apperr.Validation("email.invalid", "A valid email address is required.", nil)
	}
	user, err := s.repo.GetUserByEmail(ctx, cleanEmail)
	if err != nil {
		return err
	}
	if len(newPassword) < 8 {
		return apperr.Validation("auth.weak_password", i18n.TDefault("w4_mod.8_183"), nil)
	}
	hash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	user.UpdatedAt = time.Now().UTC()
	if tokenIssuedAt != nil {
		stamped, err := s.repo.StampPasswordChange(ctx, user.ID, user.UpdatedAt, *tokenIssuedAt)
		if err != nil {
			return err
		}
		if !stamped {
			return ErrResetTokenSpent
		}
	}
	user.PasswordHash = hash
	user.MustChangePassword = false
	if err := s.repo.UpdateUser(ctx, user); err != nil {
		return err
	}
	_ = s.repo.UpsertSecurity(ctx, &UserSecurity{
		UserID:              user.ID,
		LoginAttempts:       0,
		LastPasswordChange:  &user.UpdatedAt,
		PasswordChangeCount: 1,
	})
	if s.sessionStore != nil {
		_ = s.sessionStore.DeleteAllForUser(ctx, user.ID)
	}

	var pOrgID *int64
	if oID, _, _, err := s.repo.DefaultOrgInfoForUser(ctx, user.ID); err == nil && oID > 0 {
		pOrgID = &oID
	}
	_ = s.RecordAudit(ctx, database.AuditEntry{
		OrganizationID: pOrgID,
		ActorUserID:    user.ID,
		Action:         "user.password_changed",
		EntityType:     "user",
		EntityID:       strconv.FormatInt(user.ID, 10),
		Before:         map[string]any{"password_status": "reset"},
		After:          map[string]any{"password_status": "updated", "changed_at": time.Now().UTC().Format(time.RFC3339)},
	})

	s.log.InfoContext(ctx, "user password reset successfully", "user_id", user.ID)
	return nil
}
