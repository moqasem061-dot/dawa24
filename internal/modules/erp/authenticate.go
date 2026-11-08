package erp

import (
	"context"
	"crypto/hmac"
	"encoding/hex"
	"time"

	"github.com/moqasem061-dot/dawa24/internal/platform/authctx"
)

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
	refuse := func(reason string, attrs ...any) (Principal, error) {
		if s.log != nil {
			s.log.WarnContext(ctx, "erp key refused", append([]any{"reason", reason, "prefix", prefix}, attrs...)...)
		}
		return invalid()
	}
	switch {
	case !valid:
		return refuse("malformed")
	case lookupErr != nil:
		return refuse("lookup_error", "error", lookupErr)
	case !found:
		return refuse("unknown_prefix")
	case !matches:
		return refuse("secret_mismatch")
	}
	if row.Status != "active" {
		return refuse("key_status", "status", row.Status)
	}
	if row.ExpiresAt != nil && !row.ExpiresAt.After(time.Now()) {
		return refuse("key_expired")
	}
	if row.IntegrationStatus != "active" {
		return refuse("integration_status", "status", row.IntegrationStatus)
	}
	status, orgType, err := s.repo.OrgStatusType(ctx, row.OrganizationID)
	if err != nil {
		return refuse("org_lookup_error", "organization_id", row.OrganizationID, "error", err)
	}
	if !authctx.IsApprovedOrgStatus(status) {
		return refuse("org_status", "organization_id", row.OrganizationID, "status", status)
	}
	if err = s.repo.TouchLastUsed(ctx, row.OrganizationID, row.ID, ip); err != nil {
		return refuse("touch_error", "organization_id", row.OrganizationID, "error", err)
	}
	return Principal{OrganizationID: row.OrganizationID, IntegrationID: row.IntegrationID, KeyID: row.ID, Provider: row.Provider, Scopes: row.Scopes, WarehouseID: row.WarehouseID, BranchID: row.BranchID, BuyingUserID: row.BuyingUserID, MaxBatchItems: row.MaxBatchItems, RatePerMinute: row.RateLimitPerMinute, OrgType: orgType, KeyPrefix: row.KeyPrefix}, nil
}
