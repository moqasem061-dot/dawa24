package erp

import "time"

type Principal struct {
	OrganizationID, IntegrationID, KeyID int64
	Provider                             string
	Scopes                               []string
	WarehouseID, BranchID                int64
	BuyingUserID                         *int64
	MaxBatchItems, RatePerMinute         int
	OrgType, KeyPrefix                   string
}

func (p Principal) HasScope(s string) bool {
	for _, scope := range p.Scopes {
		if scope == s {
			return true
		}
	}
	return false
}

type Integration struct {
	ID, OrganizationID         int64
	PublicID, Provider, Status string
	CreatedBy                  *int64
	CreatedAt, UpdatedAt       time.Time
}

type APIKey struct {
	ID, OrganizationID, IntegrationID   int64
	PublicID, Name, KeyPrefix           string
	Scopes                              []string
	WarehouseID, BranchID               int64
	BuyingUserID                        *int64
	RateLimitPerMinute, MaxBatchItems   int
	Status                              string
	ExpiresAt, LastUsedAt, RevokedAt    *time.Time
	LastUsedIP                          *string
	RevokedBy, RotatedFromID, CreatedBy *int64
	RevokeReason                        string
	CreatedAt, UpdatedAt                time.Time
}

type CreateKeyInput struct {
	OrganizationID, ActorUserID       int64
	ActorIsOwner                      bool
	Provider, Name, Tag               string
	Scopes                            []string
	WarehouseID, BranchID             int64
	BuyingUserID                      *int64
	RateLimitPerMinute, MaxBatchItems int
	ExpiresAt                         *time.Time
}
type CreatedKey struct {
	Key     APIKey
	FullKey string
}
type KeyAuthRow struct {
	APIKey
	KeyHash, IntegrationStatus, Provider string
}
type CallRecord struct {
	ID                                  int64
	OrganizationID, APIKeyID            *int64
	KeyPrefix, RequestID, Method, Route string
	Status                              int
	ErrorCode                           string
	DurationMS, BytesIn                 int
	IP                                  *string
	SensitiveRead                       bool
	CreatedAt                           time.Time
}
