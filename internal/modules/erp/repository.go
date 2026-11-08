package erp

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("erp record not found")

type AuditRecord struct {
	Action, EntityType, EntityID string
	Before, After                any
	ActorUserID                  int64
	ActorKeyID                   *int64
	ActorKind                    string
}
type NewKey struct {
	APIKey
	KeyHash string
}
type StaffKeyRow struct {
	Key         APIKey
	Integration Integration
	// OrganizationName is the owning organization's bilingual display name,
	// straight from org.organizations.name (an {"ar":…,"en":…} JSON object),
	// so the staff screen can attribute a key before revoking it.
	OrganizationName map[string]string
}

type HealthInfo struct {
	OrganizationName  map[string]string
	WarehouseName     string
	ExpiresAt         *time.Time
	IntegrationStatus string
}

type Repository interface {
	SyncRepository
	OrderRepository
	HealthData(context.Context, Principal) (HealthInfo, error)
	FindKeyByPrefixAsSystem(context.Context, string) (KeyAuthRow, bool, error)
	OrgStatusType(context.Context, int64) (status, orgType string, err error)
	TouchLastUsed(context.Context, int64, int64, string) error
	InsertCallsAsSystem(context.Context, []CallRecord) error
	EnsureIntegration(context.Context, int64, string, int64, AuditRecord) (Integration, error)
	InsertKey(context.Context, int64, NewKey, AuditRecord) (APIKey, error)
	GetKey(context.Context, int64, int64) (APIKey, error)
	ListKeys(context.Context, int64) ([]APIKey, error)
	SetKeyExpiry(context.Context, int64, int64, time.Time, AuditRecord) error
	SetKeyRateLimit(context.Context, int64, int64, int, AuditRecord) error
	RevokeKey(context.Context, int64, int64, string, int64, AuditRecord) error
	SetIntegrationStatus(context.Context, int64, int64, string, AuditRecord) error
	WarehouseInOrg(context.Context, int64, int64) (bool, error)
	BranchInOrg(context.Context, int64, int64) (bool, error)
	ActiveMember(context.Context, int64, int64) (bool, error)
	ListKeysAsStaff(context.Context) ([]StaffKeyRow, error)
	RevokeKeyAsStaff(context.Context, int64, string, int64, AuditRecord) error
}
