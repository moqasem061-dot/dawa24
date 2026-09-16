package compare

import (
	"context"
)

// Repository defines the persistence operations for compare plans, subscriptions, and sessions.
type Repository interface {
	// Plans & Features
	ListPlans(ctx context.Context, onlyPublic bool) ([]*Plan, error)
	GetPlanByID(ctx context.Context, id int64) (*Plan, error)
	GetPlanBySlug(ctx context.Context, slug string) (*Plan, error)
	CreatePlan(ctx context.Context, plan *Plan) error
	UpdatePlan(ctx context.Context, plan *Plan) error
	DeletePlan(ctx context.Context, id int64) error

	// Plan Features
	ListPlanFeatures(ctx context.Context, planID int64) ([]*PlanFeature, error)
	SetPlanFeature(ctx context.Context, feature *PlanFeature) error
	DeletePlanFeature(ctx context.Context, id int64) error

	// Plan Requests
	CreatePlanRequest(ctx context.Context, req *PlanRequest) error
	GetPlanRequestByID(ctx context.Context, id int64) (*PlanRequest, error)
	ListPlanRequestsByOrg(ctx context.Context, orgID int64) ([]*PlanRequest, error)
	ListPendingPlanRequests(ctx context.Context) ([]*PlanRequest, error)
	ReviewPlanRequest(ctx context.Context, id int64, status PlanRequestStatus, reviewerID int64, reason string) error

	// Subscriptions
	CreateSubscription(ctx context.Context, sub *Subscription) error
	GetSubscriptionByID(ctx context.Context, id int64) (*Subscription, error)
	GetActiveSubscription(ctx context.Context, userID int64, orgID *int64) (*Subscription, error)
	ListSubscriptionsByOrg(ctx context.Context, orgID int64) ([]*Subscription, error)
	UpdateSubscriptionStatus(ctx context.Context, id int64, status SubscriptionStatus) error

	// Subscription Users (Seats)
	AssignSubscriptionUser(ctx context.Context, subID int64, userID int64) error
	RemoveSubscriptionUser(ctx context.Context, subID int64, userID int64) error
	ListSubscriptionUsers(ctx context.Context, subID int64) ([]*SubscriptionUser, error)
	IsUserAssignedToSubscription(ctx context.Context, subID int64, userID int64) (bool, error)

	// User Sessions & Device Caps
	UpsertUserSession(ctx context.Context, sess *UserSession) error
	TouchUserSession(ctx context.Context, sessionID string) error
	CountActiveUserSessions(ctx context.Context, userID int64) (int, error)
	ListActiveUserSessions(ctx context.Context, userID int64) ([]*UserSession, error)
	EvictOldestSessions(ctx context.Context, userID int64, keepCount int) error
	DeactivateUserSession(ctx context.Context, sessionID string) error

	// Compare Files & Archive Management
	CreateFile(ctx context.Context, f *CompareFile) error
	GetFileByID(ctx context.Context, id int64) (*CompareFile, error)
	GetFileByPublicID(ctx context.Context, publicID string) (*CompareFile, error)
	ListFiles(ctx context.Context, userID int64, orgID *int64, status *CompareFileStatus) ([]*CompareFile, error)
	ListAllFiles(ctx context.Context, search string, status *CompareFileStatus) ([]*CompareFile, error)
	ListAdminTempWarehouses(ctx context.Context, filter AdminTempWarehouseFilter) ([]*AdminTempWarehouse, error)
	ListAdminTempWarehousesWithTotal(ctx context.Context, filter AdminTempWarehouseFilter, limit, offset int) ([]*AdminTempWarehouse, int, error)
	AdminTempWarehouseStats(ctx context.Context, filter AdminTempWarehouseFilter) (totalRows int64, activeCount, archivedCount int, err error)
	ListTempWarehouseUploaders(ctx context.Context) ([]FileUploader, error)
	CountActiveFiles(ctx context.Context, userID int64, orgID *int64) (int, error)
	UpdateFile(ctx context.Context, f *CompareFile) error
	RenameFile(ctx context.Context, id int64, newSupplierName string) error
	ArchiveOldestFiles(ctx context.Context, userID int64, orgID *int64, keepCount int, reason string) ([]string, error)
	ArchiveActiveFiles(ctx context.Context, userID int64, orgID *int64, reason string) ([]string, error)
	ArchiveFile(ctx context.Context, id int64, reason string) error
	UnarchiveFile(ctx context.Context, id int64) error
	DeleteFile(ctx context.Context, id int64) error
	BulkDeleteFiles(ctx context.Context, ids []int64, ownerID *int64) (int64, error)
	BulkArchiveFiles(ctx context.Context, ids []int64, ownerID *int64, reason string) (int64, error)
	BulkUnarchiveFiles(ctx context.Context, ids []int64, ownerID *int64) (int64, error)
	PurgeExpiredCompareFiles(ctx context.Context, defaultRetentionDays int) (int64, error)
	AutoArchiveTempWarehouses(ctx context.Context, olderThanHours int) (int64, error)
	PurgeArchivedTempWarehouses(ctx context.Context, olderThanDays int) (storageKeys []string, purgedFiles int64, purgedRows int64, err error)

	// Compare File Rows
	InsertFileRows(ctx context.Context, rows []*CompareFileRow) error
	ListFileRows(ctx context.Context, fileID int64, limit, offset int) ([]*CompareFileRow, error)
	GetFileRowsPaginated(ctx context.Context, fileID int64, page, limit int) ([]*CompareFileRow, int64, error)
	DeleteFileRows(ctx context.Context, fileID int64) error
	DeleteFileRow(ctx context.Context, rowID int64) error
	// DeleteFileRowOwnedBy deletes a row only if its parent file's user_id
	// matches ownerUserID. Returns apperr.NotFound when nothing matched.
	DeleteFileRowOwnedBy(ctx context.Context, rowID int64, ownerUserID int64) error
	UpdateFileRowMatch(ctx context.Context, rowID int64, matchedProductID *int64, method MatchMethod, confidence float64) error
	// BulkUpdateFileRowMatches writes a whole matching run in one statement.
	// A price list is tens of thousands of rows; a round trip per row would
	// move the bottleneck rather than remove it.
	BulkUpdateFileRowMatches(ctx context.Context, fileID int64, matches []RowMatch) error

	// Deterministic Product Matching & Saved Mappings (Plan V5 Phase 2 §2.4)
	SaveCustomerProductMapping(ctx context.Context, orgID *int64, rawName string, productID int64, source string) error
	GetSavedProductMapping(ctx context.Context, orgID *int64, rawName string) (*int64, error)
	FindCandidateProducts(ctx context.Context, orgID *int64, query, sku string, limit int) ([]*CandidateProduct, error)

	// Search across files
	SearchFileRows(ctx context.Context, userID int64, orgID *int64, query string, limit int) ([]*CompareFileRowWithSupplier, error)

	// Market Discounts (Public & Platform Wide)
	ListMarketDiscounts(ctx context.Context, filter MarketDiscountsFilter) (*MarketDiscountsResult, error)
	// CountMarketDiscounts is the pager's total, split from the listing so it
	// can be cached. See market_board_service.go.
	CountMarketDiscounts(ctx context.Context, filter MarketDiscountsFilter) (int64, error)
	ListDistinctSuppliers(ctx context.Context) ([]string, error)

	// LoadMarketOffers reads the whole comparable market for aggregation.
	// ListMarketDiscounts is the paginated feed and clamps to 100 rows; this is
	// the analytical read the two market screens need. See market_dataset.go.
	LoadMarketOffers(ctx context.Context, opts MarketScanOptions) ([]MarketOffer, error)
}

// MarketScanOptions narrows an analytical market read.
type MarketScanOptions struct {
	// OrganizationID admits the caller's own private files alongside the public
	// market, so a supplier comparing their list against the market sees their
	// own uploads treated as theirs rather than as absent.
	OrganizationID *int64
	// ExcludeFileID drops one file from the market — the file being compared
	// against it, which would otherwise be its own best and worst offer.
	ExcludeFileID int64
	// ExcludeSupplierName drops any offers from the compared supplier itself,
	// ensuring a supplier is never benchmarked against itself.
	ExcludeSupplierName string
}
