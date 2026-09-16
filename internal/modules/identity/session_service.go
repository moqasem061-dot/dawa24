package identity

import (
	"context"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// RevokeSession invalidates one of the user's own sessions.
func (s *Service) RevokeSession(ctx context.Context, token string, userID int64) error {
	if s.sessionStore == nil {
		return nil
	}
	sess, err := s.sessionStore.Get(ctx, token)
	if err != nil {
		return err
	}
	if sess.UserID != userID {
		return apperr.Forbidden("session.not_owner", "You can only revoke your own sessions.")
	}
	return s.sessionStore.Delete(ctx, token)
}

// RevokeOrgSession invalidates a session belonging to an organization.
func (s *Service) RevokeOrgSession(ctx context.Context, orgID int64, token string) error {
	if s.sessionStore == nil {
		return nil
	}
	sess, err := s.sessionStore.Get(ctx, token)
	if err != nil {
		return err
	}
	if sess.ActiveOrgID != orgID {
		return apperr.Forbidden("session.not_org_member", "You can only revoke sessions belonging to your organization.")
	}
	return s.sessionStore.Delete(ctx, token)
}

// RevokeAllOtherOrgSessions revokes all sessions belonging to the organization EXCEPT the current token.
func (s *Service) RevokeAllOtherOrgSessions(ctx context.Context, orgID int64, currentToken string) error {
	if s.sessionStore == nil {
		return nil
	}
	return s.sessionStore.DeleteAllOtherForOrg(ctx, orgID, currentToken)
}

// RevokeAllOtherUserSessions revokes all sessions belonging to the user EXCEPT the current token.
func (s *Service) RevokeAllOtherUserSessions(ctx context.Context, userID int64, currentToken string) error {
	if s.sessionStore == nil {
		return nil
	}
	return s.sessionStore.DeleteAllOtherForUser(ctx, userID, currentToken)
}

// GetOrgPlanLimits returns the concurrent session and device quotas for an organization.
func (s *Service) GetOrgPlanLimits(ctx context.Context, orgID int64) (maxSessions int, maxDevices int, planName string, err error) {
	if s == nil || s.repo == nil {
		return 3, 3, "الباقة الأساسية", nil
	}
	return s.repo.GetOrgPlanLimits(ctx, orgID)
}

// SetIdleTimeout configures the active idle timeout duration on the underlying session store.
func (s *Service) SetIdleTimeout(d time.Duration) {
	if s.sessionStore != nil {
		s.sessionStore.SetIdleTimeout(d)
	}
}

// GetIdleTimeout retrieves the active idle timeout duration.
func (s *Service) GetIdleTimeout() time.Duration {
	if s.sessionStore != nil {
		return s.sessionStore.GetIdleTimeout()
	}
	return 30 * time.Minute
}

// ValidateSession verifies a session token and confirms the user still exists and is active.
func (s *Service) ValidateSession(ctx context.Context, token string) (*Session, error) {
	if s.sessionStore == nil {
		return nil, apperr.Unauthorized()
	}
	sess, err := s.sessionStore.Get(ctx, token)
	if err != nil {
		return nil, err
	}
	if s.repo != nil {
		if !s.isUserActive(ctx, sess.UserID) {
			_ = s.sessionStore.Delete(ctx, token)
			return nil, apperr.Unauthorized()
		}
	}
	return sess, nil
}

// ValidateSessionWithoutTouch validates a session without recording the request
// as user activity, for requests the browser issues on its own. See
// SessionStore.GetWithoutTouch.
func (s *Service) ValidateSessionWithoutTouch(ctx context.Context, token string) (*Session, error) {
	if s.sessionStore == nil {
		return nil, apperr.Unauthorized()
	}
	sess, err := s.sessionStore.GetWithoutTouch(ctx, token)
	if err != nil {
		return nil, err
	}
	if s.repo != nil {
		if !s.isUserActive(ctx, sess.UserID) {
			_ = s.sessionStore.Delete(ctx, token)
			return nil, apperr.Unauthorized()
		}
	}
	return sess, nil
}

// GetUserByID looks up a user by ID.
func (s *Service) GetUserByID(ctx context.Context, id int64) (*User, error) {
	return s.repo.GetUserByID(ctx, id)
}

// GetUserByEmail looks up a user by email address.
func (s *Service) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	return s.repo.GetUserByEmail(ctx, email)
}

// UserBelongsToOrg reports whether a user is a member of an organization.
//
// This backs the tenant-switch check in ResolveTenant. Without it, a caller
// could name any organization in the X-Dawa-Org-ID header and row-level
// security would faithfully scope every query to that tenant's data — the
// isolation mechanism working perfectly against an attacker-supplied input.
func (s *Service) UserBelongsToOrg(ctx context.Context, userID, orgID int64) (bool, error) {
	return s.repo.UserBelongsToOrg(ctx, userID, orgID)
}

// ResolvePermissions computes effective permissions for a user within an org.
func (s *Service) ResolvePermissions(ctx context.Context, userID int64, orgID int64) ([]string, error) {
	return s.resolvePermissions(ctx, userID, orgID), nil
}

// ListUserOrganizations lists all organizations a user has active membership in.
func (s *Service) ListUserOrganizations(ctx context.Context, userID int64) ([]*UserOrgMembership, error) {
	return s.repo.ListUserOrganizations(ctx, userID)
}

// SwitchActiveOrg verifies membership in target organization and constructs an updated active Session.
func (s *Service) SwitchActiveOrg(ctx context.Context, userID, targetOrgID int64) (*Session, error) {
	belongs, err := s.repo.UserBelongsToOrg(ctx, userID, targetOrgID)
	if err != nil {
		return nil, err
	}
	if !belongs {
		return nil, apperr.Forbidden("auth.org_unauthorized", "User does not have access to this organization.")
	}

	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	perms := s.resolvePermissions(ctx, userID, targetOrgID)

	// Lookup target org info
	orgs, err := s.repo.ListUserOrganizations(ctx, userID)
	if err != nil {
		return nil, err
	}

	var orgType, orgStatus string
	for _, o := range orgs {
		if o.OrganizationID == targetOrgID {
			orgType = o.OrgType
			orgStatus = o.OrgStatus
			break
		}
	}

	normType, wasLegacy := NormalizeOrgType(orgType)
	if wasLegacy {
		s.log.WarnContext(ctx, "legacy organization type normalized on switch",
			"original", orgType, "normalized", normType, "user_id", userID, "org_id", targetOrgID)
	}
	orgType = normType

	sess := &Session{
		UserID:      user.ID,
		PublicID:    user.PublicID,
		Email:       user.Email,
		Role:        user.Role,
		ActiveOrgID: targetOrgID,
		OrgType:     orgType,
		OrgStatus:   orgStatus,
		Permissions: perms,
	}

	if s.sessionStore != nil {
		if err := s.sessionStore.Create(ctx, sess); err != nil {
			return nil, err
		}
	}

	s.log.InfoContext(ctx, "switched active organization", "user_id", userID, "org_id", targetOrgID, "org_type", orgType)
	return sess, nil
}

// GetOrgInfoForUser retrieves and normalizes organization details and permissions for a user membership.
func (s *Service) GetOrgInfoForUser(ctx context.Context, userID, orgID int64) (orgType, orgStatus string, perms []string, err error) {
	orgs, err := s.repo.ListUserOrganizations(ctx, userID)
	if err != nil {
		return "", "", nil, err
	}
	found := false
	for _, o := range orgs {
		if o.OrganizationID == orgID {
			orgType = o.OrgType
			orgStatus = o.OrgStatus
			found = true
			break
		}
	}
	if !found {
		return "", "", nil, apperr.Forbidden("org.not_a_member", "You are not a member of that organization.")
	}
	normType, wasLegacy := NormalizeOrgType(orgType)
	if wasLegacy {
		s.log.WarnContext(ctx, "legacy organization type normalized on membership resolution",
			"original", orgType, "normalized", normType, "user_id", userID, "org_id", orgID)
	}
	orgType = normType
	perms = s.resolvePermissions(ctx, userID, orgID)
	return orgType, orgStatus, perms, nil
}

func stringsContains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
