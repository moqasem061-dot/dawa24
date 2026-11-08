package erp

import (
	"context"
	"time"
)

// Cleanup runs the ERP retention sweep. It is a maintenance job: fixed
// cut-offs from the Retention* constants, bounded by CleanupBatchRows rows per
// statement so a backlog drains over runs instead of in one giant delete.
// order_imports and idempotency_records are never touched (they are the
// ledger); the ERP rows in catalog.match_runs are purged by their
// consumer_ref prefix (plan.md E3's named scope).
func (s *Service) Cleanup(ctx context.Context, now time.Time) error {
	cut := Cutoffs{
		APICalls:    now.Add(-RetentionAPICalls),
		SyncItems:   now.Add(-RetentionSyncItems),
		SyncBatches: now.Add(-RetentionSyncBatches),
		Review:      now.Add(-RetentionReview),
		MatchRuns:   now.Add(-RetentionMatchRuns),
		BatchRows:   CleanupBatchRows,
	}
	return s.repo.PurgeExpiredAsSystem(ctx, cut)
}
