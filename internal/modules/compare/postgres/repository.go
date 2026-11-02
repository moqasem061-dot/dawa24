package postgres

import (
	"sync/atomic"

	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// Repository implements compare.Repository backed by PostgreSQL.
type Repository struct {
	db *database.DB
	// legacyUntil is when to stop assuming compare.file_rows.file_uploaded_at
	// is missing (migration 246 not applied yet), as a unix nanosecond stamp.
	// Until then the market board orders by compare.files.created_at — the same
	// order at the cost of a sort. Zero means "use the denormalised column".
	//
	// A deadline rather than a flag so the board heals itself: the deploy's
	// migrate step can run seconds after the server starts, and a process that
	// latched the fallback would keep sorting every page view until the next
	// restart. See ListMarketDiscounts.
	legacyUntil atomic.Int64
}

// NewRepository creates a new compare repository.
func NewRepository(db *database.DB) *Repository {
	return &Repository{db: db}
}

var _ compare.Repository = (*Repository)(nil)
