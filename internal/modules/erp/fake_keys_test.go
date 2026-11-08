package erp

import (
	"context"
	"time"
)

type fakeRepo struct {
	*fakeSync
	*fakeOrders
	*fakeCode
	*fakePurchaseRepo
	keys                                     map[int64]NewKey
	integrations                             map[int64]Integration
	audits                                   []AuditRecord
	orgStatus, orgType                       string
	validWarehouse, validBranch, validMember bool
	nextID                                   int64
	touches                                  int
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{fakeSync: newFakeSync(), fakeOrders: newFakeOrders(), fakeCode: newFakeCode(), fakePurchaseRepo: newFakePurchaseRepo(), keys: map[int64]NewKey{}, integrations: map[int64]Integration{}, orgStatus: "approved", orgType: "vendor", validWarehouse: true, validBranch: true, validMember: true}
}
func (f *fakeRepo) FindKeyByPrefixAsSystem(_ context.Context, prefix string) (KeyAuthRow, bool, error) {
	for _, k := range f.keys {
		if k.KeyPrefix == prefix {
			i := f.integrations[k.IntegrationID]
			return KeyAuthRow{APIKey: k.APIKey, KeyHash: k.KeyHash, IntegrationStatus: i.Status, Provider: i.Provider}, true, nil
		}
	}
	return KeyAuthRow{}, false, nil
}
func (f *fakeRepo) OrgStatusType(context.Context, int64) (string, string, error) {
	return f.orgStatus, f.orgType, nil
}
func (f *fakeRepo) TouchLastUsed(context.Context, int64, int64, string) error {
	f.touches++
	return nil
}
func (f *fakeRepo) InsertCallsAsSystem(context.Context, []CallRecord) error { return nil }
func (f *fakeRepo) EnsureIntegration(_ context.Context, orgID int64, provider string, createdBy int64, a AuditRecord) (Integration, error) {
	for _, i := range f.integrations {
		if i.OrganizationID == orgID && i.Provider == provider && i.Status != "closed" {
			return i, nil
		}
	}
	f.nextID++
	i := Integration{ID: f.nextID, OrganizationID: orgID, Provider: provider, Status: "active"}
	f.integrations[i.ID] = i
	f.audits = append(f.audits, a)
	return i, nil
}
func (f *fakeRepo) InsertKey(_ context.Context, orgID int64, k NewKey, a AuditRecord) (APIKey, error) {
	f.nextID++
	k.ID = f.nextID
	k.OrganizationID = orgID
	k.Status = "active"
	f.keys[k.ID] = k
	f.audits = append(f.audits, a)
	return k.APIKey, nil
}
func (f *fakeRepo) GetKey(_ context.Context, orgID, keyID int64) (APIKey, error) {
	k, ok := f.keys[keyID]
	if !ok || k.OrganizationID != orgID {
		return APIKey{}, ErrNotFound
	}
	return k.APIKey, nil
}
func (f *fakeRepo) ListKeys(_ context.Context, orgID int64) ([]APIKey, error) {
	var out []APIKey
	for _, k := range f.keys {
		if k.OrganizationID == orgID {
			out = append(out, k.APIKey)
		}
	}
	return out, nil
}
func (f *fakeRepo) SetKeyExpiry(_ context.Context, orgID, keyID int64, at time.Time, a AuditRecord) error {
	k, ok := f.keys[keyID]
	if !ok || k.OrganizationID != orgID {
		return ErrNotFound
	}
	k.ExpiresAt = &at
	f.keys[keyID] = k
	f.audits = append(f.audits, a)
	return nil
}
func (f *fakeRepo) SetKeyRateLimit(_ context.Context, orgID, keyID int64, rate int, a AuditRecord) error {
	k, ok := f.keys[keyID]
	if !ok || k.OrganizationID != orgID {
		return ErrNotFound
	}
	k.RateLimitPerMinute = rate
	f.keys[keyID] = k
	f.audits = append(f.audits, a)
	return nil
}
func (f *fakeRepo) RevokeKey(_ context.Context, orgID, keyID int64, reason string, by int64, a AuditRecord) error {
	k, ok := f.keys[keyID]
	if !ok || k.OrganizationID != orgID {
		return ErrNotFound
	}
	k.Status = "revoked"
	k.RevokeReason = reason
	f.keys[keyID] = k
	f.audits = append(f.audits, a)
	return nil
}
func (f *fakeRepo) SetIntegrationStatus(_ context.Context, orgID, id int64, status string, a AuditRecord) error {
	i, ok := f.integrations[id]
	if !ok || i.OrganizationID != orgID {
		return ErrNotFound
	}
	i.Status = status
	f.integrations[id] = i
	f.audits = append(f.audits, a)
	return nil
}
func (f *fakeRepo) WarehouseInOrg(context.Context, int64, int64) (bool, error) {
	return f.validWarehouse, nil
}
func (f *fakeRepo) BranchInOrg(context.Context, int64, int64) (bool, error) {
	return f.validBranch, nil
}
func (f *fakeRepo) KeyTargets(context.Context, int64) ([]KeyTarget, []KeyTarget, error) {
	return nil, nil, nil
}
func (f *fakeRepo) ActiveMember(context.Context, int64, int64) (bool, error) {
	return f.validMember, nil
}
func (f *fakeRepo) ListKeysAsStaff(context.Context) ([]StaffKeyRow, error) {
	var out []StaffKeyRow
	for _, k := range f.keys {
		out = append(out, StaffKeyRow{Key: k.APIKey})
	}
	return out, nil
}
func (f *fakeRepo) RevokeKeyAsStaff(ctx context.Context, id int64, reason string, by int64, a AuditRecord) error {
	k, ok := f.keys[id]
	if !ok {
		return ErrNotFound
	}
	return f.RevokeKey(ctx, k.OrganizationID, id, reason, by, a)
}

var _ Repository = (*fakeRepo)(nil)

func (f *fakeRepo) HealthData(_ context.Context, p Principal) (HealthInfo, error) {
	k, ok := f.keys[p.KeyID]
	if !ok || k.OrganizationID != p.OrganizationID {
		return HealthInfo{}, ErrNotFound
	}
	return HealthInfo{OrganizationName: map[string]string{"ar": "منشأة", "en": "Organization"}, WarehouseName: "Warehouse", ExpiresAt: k.ExpiresAt, IntegrationStatus: f.integrations[k.IntegrationID].Status}, nil
}
