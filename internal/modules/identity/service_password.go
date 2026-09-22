package identity

import (
	"context"
	"strconv"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

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
	user.UpdatedAt = time.Now()
	if err := s.repo.UpdateUser(ctx, user); err != nil {
		return err
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

// ResetPassword resets a user's password directly after OTP verification.
func (s *Service) ResetPassword(ctx context.Context, email, newPassword string) error {
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
	user.PasswordHash = hash
	user.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpdateUser(ctx, user); err != nil {
		return err
	}
	maxSessions := 10
	_ = s.repo.UpsertSecurity(ctx, &UserSecurity{
		UserID:              user.ID,
		LoginAttempts:       0,
		LastPasswordChange:  &user.UpdatedAt,
		PasswordChangeCount: 1,
		MaxLoginSessions:    &maxSessions,
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
