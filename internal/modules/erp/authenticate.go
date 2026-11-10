package erp

import (
	"context"
	"crypto/hmac"
	"encoding/hex"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
)

// AuthRefusal is a refused key whose prefix matched a row. The caller still gets
// the same ERP_AUTH_INVALID answer (Err, unchanged); the HTTP layer reads the
// organization, key and reason from here so the refusal shows in the tenant's
// own activity log. Refusals with no matching row stay a bare *Error.
type AuthRefusal struct {
	OrganizationID, KeyID int64
	KeyPrefix, Reason     string
	Err                   *Error
}

func (e *AuthRefusal) Error() string { return e.Err.Error() }

// Unwrap keeps errors.As(err, *Error) working for the response writer.
func (e *AuthRefusal) Unwrap() error { return e.Err }

func (s *Service) Authenticate(ctx context.Context, raw, ip string) (Principal, error) {
	invalid := func() (Principal, error) { return Principal{}, NewError("ERP_AUTH_INVALID", 401, nil) }
	prefix, secret, valid := ParseKey(raw)
	var row KeyAuthRow
	var found bool
	var lookupErr error
	if valid {
		row, found, lookupErr = s.repo.FindKeyByPrefixAsSystem(ctx, prefix)
	}
	expected := make([]byte, 32)
	if found {
		decoded, err := hex.DecodeString(row.KeyHash)
		if err == nil && len(decoded) == len(expected) {
			expected = decoded
		} else {
			found = false
		}
	}
	actual := s.hashSecret(secret)
	matches := hmac.Equal(actual, expected)
	// Every refusal answers the same code (a caller learns nothing about which
	// keys exist); the reason goes to the server log so an operator can tell a
	// mistyped key from a paused integration or a database error.
	logRefusal := func(reason string, attrs ...any) {
		if s.log != nil {
			s.log.WarnContext(ctx, "erp key refused", append([]any{"reason", reason, "prefix", prefix}, attrs...)...)
		}
	}
	refuse := func(reason string, attrs ...any) (Principal, error) {
		logRefusal(reason, attrs...)
		return invalid()
	}
	// refuseKnown is for a refusal of a key whose row was found: the response
	// is the same as refuse's, but the caller can read who was refused and why.
	refuseKnown := func(reason string, attrs ...any) (Principal, error) {
		logRefusal(reason, attrs...)
		return Principal{}, &AuthRefusal{OrganizationID: row.OrganizationID, KeyID: row.ID, KeyPrefix: prefix, Reason: reason, Err: NewError("ERP_AUTH_INVALID", 401, nil)}
	}
	switch {
	case !valid:
		return refuse("malformed")
	case lookupErr != nil:
		return refuse("lookup_error", "error", lookupErr)
	case !found:
		return refuse("unknown_prefix")
	case !matches:
		return refuseKnown("secret_mismatch")
	}
	if row.Status != "active" {
		return refuseKnown("key_status", "status", row.Status)
	}
	if row.ExpiresAt != nil && !row.ExpiresAt.After(time.Now()) {
		return refuseKnown("key_expired")
	}
	if row.IntegrationStatus != "active" {
		return refuseKnown("integration_status", "status", row.IntegrationStatus)
	}
	status, orgType, err := s.repo.OrgStatusType(ctx, row.OrganizationID)
	if err != nil {
		return refuseKnown("org_lookup_error", "organization_id", row.OrganizationID, "error", err)
	}
	if !authctx.IsApprovedOrgStatus(status) {
		return refuseKnown("org_status", "organization_id", row.OrganizationID, "status", status)
	}
	if err = s.repo.TouchLastUsed(ctx, row.OrganizationID, row.ID, ip); err != nil {
		return refuseKnown("touch_error", "organization_id", row.OrganizationID, "error", err)
	}
	return Principal{OrganizationID: row.OrganizationID, IntegrationID: row.IntegrationID, KeyID: row.ID, Provider: row.Provider, Scopes: row.Scopes, WarehouseID: row.WarehouseID, BranchID: row.BranchID, BuyingUserID: row.BuyingUserID, MaxBatchItems: row.MaxBatchItems, RatePerMinute: row.RateLimitPerMinute, OrgType: orgType, KeyPrefix: row.KeyPrefix}, nil
}
