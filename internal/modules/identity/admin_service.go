package identity

import (
	"context"
)

func (s *Service) AdminListUsers(ctx context.Context, role, status string) ([]*User, error) {
	return s.repo.AdminListUsers(ctx, role, status)
}

func (s *Service) AdminListUsersWithTotal(ctx context.Context, filter AdminUserFilter, limit, offset int) ([]*User, int, error) {
	return s.repo.AdminListUsersWithTotal(ctx, filter, limit, offset)
}

func (s *Service) AdminUserStats(ctx context.Context) (AdminUserStatsResult, error) {
	return s.repo.AdminUserStats(ctx)
}

func (s *Service) SearchUsers(ctx context.Context, query, role string, limit int) ([]*User, error) {
	return s.repo.SearchUsers(ctx, query, role, limit)
}

// AdminCountUsers returns the total number of accounts, for dashboards that
// need a figure rather than a page of rows.
func (s *Service) AdminCountUsers(ctx context.Context) (int, error) {
	return s.repo.AdminCountUsers(ctx)
}

func (s *Service) AdminGetUser(ctx context.Context, id int64) (*User, error) {
	return s.repo.GetUserByID(ctx, id)
}

// AdminSuspendUser blocks an account and ends the sessions it already holds.
//
// Changing the stored status alone is not enough: sessions live in Redis and
// are checked against the token, not re-read from the row, so a suspended user
// would keep working from an already-open tab until their cookie expired. That
// window is exactly what suspending an account is meant to close.
func (s *Service) AdminSuspendUser(ctx context.Context, id, actorID int64) error {
	s.InvalidateUserCache(id)
	if err := s.repo.AdminUpdateUserStatus(ctx, id, string(StatusSuspended), actorID); err != nil {
		return err
	}
	if s.sessionStore == nil {
		return nil
	}
	if err := s.sessionStore.DeleteAllForUser(ctx, id); err != nil {
		// The account is already suspended in the database, which is the part
		// that must not be rolled back. A failure to reach Redis is logged and
		// surfaced rather than swallowed, because it leaves live sessions
		// behind and an operator needs to know that.
		s.log.ErrorContext(ctx, "suspended user but could not revoke sessions",
			"error", err, "user_id", id, "actor_id", actorID)
		return err
	}
	return nil
}

func (s *Service) AdminReactivateUser(ctx context.Context, id, actorID int64) error {
	s.InvalidateUserCache(id)
	return s.repo.AdminUpdateUserStatus(ctx, id, string(StatusActive), actorID)
}

// AdminResetMFA clears a user's second factor and ends their sessions, so a
// session established with the old factor cannot outlive it.
func (s *Service) AdminResetMFA(ctx context.Context, id, actorID int64) error {
	if err := s.repo.AdminResetMFA(ctx, id, actorID); err != nil {
		return err
	}
	if s.sessionStore == nil {
		return nil
	}
	if err := s.sessionStore.DeleteAllForUser(ctx, id); err != nil {
		s.log.ErrorContext(ctx, "reset MFA but could not revoke sessions",
			"error", err, "user_id", id, "actor_id", actorID)
		return err
	}
	return nil
}

// AdminAssignRole changes a user's role and ends their sessions.
//
// Permissions are resolved from the role when a request is authorised, so a
// user demoted mid-session could otherwise keep acting with the permissions
// they held when they signed in.
func (s *Service) AdminAssignRole(ctx context.Context, id int64, role string, actorID int64) error {
	if err := s.repo.AdminAssignRole(ctx, id, role, actorID); err != nil {
		return err
	}
	if s.sessionStore == nil {
		return nil
	}
	if err := s.sessionStore.DeleteAllForUser(ctx, id); err != nil {
		s.log.ErrorContext(ctx, "assigned role but could not revoke sessions",
			"error", err, "user_id", id, "actor_id", actorID)
		return err
	}
	return nil
}

// AdminSetPassword validates password strength, hashes with bcrypt, updates password_hash, revokes all sessions, and logs audit.
func (s *Service) AdminSetPassword(ctx context.Context, userID, actorID int64, newPassword string) error {
	if err := ValidatePassword(newPassword); err != nil {
		return err
	}
	hash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	if err := s.repo.AdminSetPassword(ctx, userID, hash, actorID); err != nil {
		return err
	}
	if s.sessionStore != nil {
		if err := s.sessionStore.DeleteAllForUser(ctx, userID); err != nil {
			s.log.ErrorContext(ctx, "set password but could not revoke sessions",
				"error", err, "user_id", userID, "actor_id", actorID)
			return err
		}
	}
	// Clear any lockout on the account so the user can immediately log in with the new password
	if s.repo != nil {
		sec, _ := s.repo.GetSecurity(ctx, userID)
		if sec != nil && (sec.LockedUntil != nil || sec.LoginAttempts > 0) {
			sec.LockedUntil = nil
			sec.LoginAttempts = 0
			_ = s.repo.UpsertSecurity(ctx, sec)
		}
	}
	return nil
}

// AdminUpdateUserDetails updates user profile information and national ID.
func (s *Service) AdminUpdateUserDetails(ctx context.Context, userID int64, in AdminEditUserInput, actorID int64) error {
	if err := s.repo.AdminUpdateUserDetails(ctx, userID, in, actorID); err != nil {
		return err
	}
	// If status changed to suspended, revoke all sessions
	if in.Status == StatusSuspended && s.sessionStore != nil {
		_ = s.sessionStore.DeleteAllForUser(ctx, userID)
	}
	return nil
}

// GetNationalID retrieves the national ID for a user.
func (s *Service) GetNationalID(ctx context.Context, userID int64) (string, error) {
	return s.repo.GetNationalID(ctx, userID)
}

// RequestAccountDeletion allows a user to submit a formal request to delete their account.
func (s *Service) RequestAccountDeletion(ctx context.Context, userID int64, orgID *int64, reason string) error {
	req := &AccountDeletionRequest{
		UserID:         userID,
		OrganizationID: orgID,
		Reason:         reason,
		Status:         "pending",
	}
	return s.repo.CreateAccountDeletionRequest(ctx, req)
}

// GetPendingAccountDeletion retrieves any open deletion request for the given user.
func (s *Service) GetPendingAccountDeletion(ctx context.Context, userID int64) (*AccountDeletionRequest, error) {
	return s.repo.GetPendingAccountDeletionRequest(ctx, userID)
}

// CancelAccountDeletion cancels a pending deletion request by the user.
func (s *Service) CancelAccountDeletion(ctx context.Context, userID, requestID int64) error {
	return s.repo.CancelAccountDeletionRequest(ctx, userID, requestID)
}

// AdminListDeletionRequests lists all pending or past account deletion requests.
func (s *Service) AdminListDeletionRequests(ctx context.Context, status string) ([]*AccountDeletionRequest, error) {
	return s.repo.ListAccountDeletionRequests(ctx, status)
}

// AdminListDeletionRequestsWithTotal lists paginated account deletion requests with total count.
func (s *Service) AdminListDeletionRequestsWithTotal(ctx context.Context, status string, limit, offset int) ([]*AccountDeletionRequest, int, error) {
	return s.repo.ListAccountDeletionRequestsWithTotal(ctx, status, limit, offset)
}

// AdminReviewDeletionRequest approves or rejects an account deletion request.
func (s *Service) AdminReviewDeletionRequest(ctx context.Context, requestID, reviewerID int64, approve bool, adminNotes string) error {
	return s.repo.ReviewAccountDeletionRequest(ctx, requestID, reviewerID, approve, adminNotes)
}
