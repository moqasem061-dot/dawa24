package erp

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"
	"unicode/utf8"
)

func validation(field string) error {
	return NewError("ERP_VALIDATION", 422, map[string]any{"fields": []string{field}})
}
func keyError(err error) error {
	if errors.Is(err, ErrNotFound) {
		return NewError("ERP_NOT_FOUND", 404, nil)
	}
	return err
}
func keyAudit(action string, id int64, actor int64) AuditRecord {
	return AuditRecord{Action: action, EntityType: "erp.api_key", EntityID: fmt.Sprint(id), ActorUserID: actor}
}

func (s *Service) CreateKey(ctx context.Context, in CreateKeyInput) (CreatedKey, error) {
	if !in.ActorIsOwner {
		return CreatedKey{}, validation("actor")
	}
	if n := utf8.RuneCountInString(in.Name); n == 0 || n > 80 {
		return CreatedKey{}, validation("name")
	}
	if len(in.Scopes) == 0 {
		_, orgType, err := s.repo.OrgStatusType(ctx, in.OrganizationID)
		if err != nil {
			return CreatedKey{}, err
		}
		in.Scopes = DefaultScopes(orgType == "vendor" || orgType == "supplier" || orgType == "company" || orgType == "agency")
	}
	// The warehouse is where product uploads write stock. A key that may upload
	// products needs one; any other key may leave it out.
	ok := true
	var err error
	if in.WarehouseID != 0 || slices.Contains(in.Scopes, ScopeProductsSync) {
		ok, err = s.repo.WarehouseInOrg(ctx, in.OrganizationID, in.WarehouseID)
		if err != nil {
			return CreatedKey{}, err
		}
	}
	if !ok {
		return CreatedKey{}, validation("warehouseId")
	}
	ok, err = s.repo.BranchInOrg(ctx, in.OrganizationID, in.BranchID)
	if err != nil {
		return CreatedKey{}, err
	}
	if !ok {
		return CreatedKey{}, validation("branchId")
	}
	if in.BuyingUserID != nil {
		ok, err = s.repo.ActiveMember(ctx, in.OrganizationID, *in.BuyingUserID)
		if err != nil {
			return CreatedKey{}, err
		}
		if !ok {
			return CreatedKey{}, validation("buyingUserId")
		}
	}
	for _, scope := range in.Scopes {
		if !ValidScope(scope) {
			return CreatedKey{}, validation("scopes")
		}
	}
	if in.RateLimitPerMinute == 0 {
		in.RateLimitPerMinute = s.cfg.DefaultRate
	}
	if in.RateLimitPerMinute < MinRatePerMinute || in.RateLimitPerMinute > MaxRatePerMinute {
		return CreatedKey{}, validation("ratePerMinute")
	}
	if in.MaxBatchItems == 0 {
		in.MaxBatchItems = s.cfg.MaxBatchItems
	}
	if in.MaxBatchItems < 1 || in.MaxBatchItems > MaxBatchItems {
		return CreatedKey{}, validation("maxBatchItems")
	}
	provider := s.cfg.Provider
	integration, err := s.repo.EnsureIntegration(ctx, in.OrganizationID, provider, in.ActorUserID, keyAudit("erp.integration.create", 0, in.ActorUserID))
	if err != nil {
		return CreatedKey{}, err
	}
	creator := in.ActorUserID
	key := APIKey{IntegrationID: integration.ID, Name: in.Name, Scopes: in.Scopes, WarehouseID: in.WarehouseID, BranchID: in.BranchID, BuyingUserID: in.BuyingUserID, RateLimitPerMinute: in.RateLimitPerMinute, MaxBatchItems: in.MaxBatchItems, ExpiresAt: in.ExpiresAt, CreatedBy: &creator}
	return s.insertGeneratedKey(ctx, in.OrganizationID, key, in.ActorUserID)
}
func (s *Service) insertGeneratedKey(ctx context.Context, orgID int64, key APIKey, actor int64) (CreatedKey, error) {
	full, prefix, hash, err := GenerateKey(s.cfg.Tag, s.cfg.Pepper)
	if err != nil {
		return CreatedKey{}, err
	}
	key.KeyPrefix = prefix
	stored, err := s.repo.InsertKey(ctx, orgID, NewKey{APIKey: key, KeyHash: hash}, keyAudit("erp.key.create", 0, actor))
	if err != nil {
		return CreatedKey{}, err
	}
	return CreatedKey{Key: stored, FullKey: full}, nil
}
func (s *Service) RotateKey(ctx context.Context, orgID, keyID int64, grace time.Duration, actorUserID int64, actorIsOwner bool) (CreatedKey, error) {
	if !actorIsOwner {
		return CreatedKey{}, validation("actor")
	}
	if grace < 0 || grace > RotateGraceMax {
		return CreatedKey{}, validation("graceHours")
	}
	old, err := s.repo.GetKey(ctx, orgID, keyID)
	if err != nil {
		return CreatedKey{}, keyError(err)
	}
	old.ID = 0
	old.PublicID = ""
	old.RotatedFromID = &keyID
	old.ExpiresAt = nil
	old.LastUsedAt = nil
	old.LastUsedIP = nil
	old.RevokedAt = nil
	old.RevokedBy = nil
	old.RevokeReason = ""
	old.Status = "active"
	old.CreatedBy = &actorUserID
	result, err := s.insertGeneratedKey(ctx, orgID, old, actorUserID)
	if err != nil {
		return CreatedKey{}, err
	}
	if err = s.repo.SetKeyExpiry(ctx, orgID, keyID, time.Now().Add(grace), keyAudit("erp.key.rotate", keyID, actorUserID)); err != nil {
		return CreatedKey{}, keyError(err)
	}
	return result, nil
}
func (s *Service) UpdateKeyLimits(ctx context.Context, orgID, keyID int64, ratePerMinute int, actorUserID int64, actorIsOwner bool) error {
	if !actorIsOwner {
		return validation("actor")
	}
	if ratePerMinute < MinRatePerMinute || ratePerMinute > MaxRatePerMinute {
		return validation("ratePerMinute")
	}
	return keyError(s.repo.SetKeyRateLimit(ctx, orgID, keyID, ratePerMinute, keyAudit("erp.key.limit", keyID, actorUserID)))
}
func (s *Service) RevokeKey(ctx context.Context, orgID, keyID int64, reason string, actorUserID int64) error {
	return keyError(s.repo.RevokeKey(ctx, orgID, keyID, reason, actorUserID, keyAudit("erp.key.revoke", keyID, actorUserID)))
}
func (s *Service) PauseIntegration(ctx context.Context, orgID int64, pause bool, actorUserID int64) error {
	integration, err := s.repo.EnsureIntegration(ctx, orgID, s.cfg.Provider, actorUserID, keyAudit("erp.integration.create", 0, actorUserID))
	if err != nil {
		return err
	}
	status := "active"
	if pause {
		status = "paused"
	}
	return keyError(s.repo.SetIntegrationStatus(ctx, orgID, integration.ID, status, keyAudit("erp.integration.status", integration.ID, actorUserID)))
}
func (s *Service) ListKeys(ctx context.Context, orgID int64) ([]APIKey, error) {
	return s.repo.ListKeys(ctx, orgID)
}

// KeyTargets lists the organization's own warehouses, branches and active
// members, so the create-key form offers names instead of raw ids.
func (s *Service) KeyTargets(ctx context.Context, orgID int64) (KeyTargetSet, error) {
	return s.repo.KeyTargets(ctx, orgID)
}
func (s *Service) ListKeysAsStaff(ctx context.Context) ([]StaffKeyRow, error) {
	return s.repo.ListKeysAsStaff(ctx)
}
func (s *Service) RevokeKeyAsStaff(ctx context.Context, keyID int64, reason string, actorUserID int64) error {
	return keyError(s.repo.RevokeKeyAsStaff(ctx, keyID, reason, actorUserID, keyAudit("erp.key.revoke", keyID, actorUserID)))
}
