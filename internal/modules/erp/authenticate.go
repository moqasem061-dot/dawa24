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
	if !valid || !found || lookupErr != nil || !matches {
		return invalid()
	}
	if row.Status != "active" || (row.ExpiresAt != nil && !row.ExpiresAt.After(time.Now())) || row.IntegrationStatus != "active" {
		return invalid()
	}
	status, orgType, err := s.repo.OrgStatusType(ctx, row.OrganizationID)
	if err != nil || !authctx.IsApprovedOrgStatus(status) {
		return invalid()
	}
	if err = s.repo.TouchLastUsed(ctx, row.OrganizationID, row.ID, ip); err != nil {
		return invalid()
	}
	return Principal{OrganizationID: row.OrganizationID, IntegrationID: row.IntegrationID, KeyID: row.ID, Provider: row.Provider, Scopes: row.Scopes, WarehouseID: row.WarehouseID, BranchID: row.BranchID, BuyingUserID: row.BuyingUserID, MaxBatchItems: row.MaxBatchItems, RatePerMinute: row.RateLimitPerMinute, OrgType: orgType, KeyPrefix: row.KeyPrefix}, nil
}
