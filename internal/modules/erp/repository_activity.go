package erp

import (
	"context"
)

// ActivityRepository is the persistence of the activity screen's reads. Every
// method takes the organization and filters by it.
type ActivityRepository interface {
	ActivityCounts(ctx context.Context, orgID int64) (ActivityCounts, error)
	ListCalls(ctx context.Context, orgID int64, f CallFilter) ([]CallRow, error)
	ListBatches(ctx context.Context, orgID, beforeID int64, limit int) ([]BatchRow, error)
	ListActiveBatches(ctx context.Context, orgID int64, limit int) ([]BatchRow, error)
	ListOrderImports(ctx context.Context, orgID int64, limit int) ([]OrderImportRow, error)
}
