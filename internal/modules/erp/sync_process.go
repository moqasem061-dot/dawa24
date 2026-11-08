package erp

import (
	"context"
	"errors"
	"fmt"

	"github.com/moqasem061-dot/dawa24/internal/platform/database"
)

// ProcessChunk drains one chunk of one accepted batch. It is what the erpsync
// worker calls, and the flow tests call directly. The organization comes from
// the tenant context the worker set (database.WithTenant(args.OrganizationID)),
// never from the key: the batch was accepted, and a key revoked or rotated
// afterwards must not stop it (trap 25, plan S2 step 7).
//
// Order per plan step 7: (a) wait while an earlier batch of the same
// integration is queued or processing; (b) mark processing once; resolve
// (c)-(f), write (g)-(h); (i) finish the chunk — the increment that reaches
// chunk_count completes the batch and folds the counters.
func (s *Service) ProcessChunk(ctx context.Context, batchID int64, chunkNo int) error {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return database.ErrNoTenant
	}
	batch, err := s.repo.GetBatchByID(ctx, orgID, batchID)
	if errors.Is(err, ErrNotFound) {
		return NewError("ERP_SYNC_NOT_FOUND", 404, nil)
	}
	if err != nil {
		return s.processError(err)
	}
	earlier, err := s.repo.EarlierBatchActive(ctx, orgID, batch.IntegrationID, batchID)
	if err != nil {
		return s.processError(err)
	}
	if earlier {
		return ErrWaitForEarlierBatch
	}
	if err := s.repo.MarkBatchProcessing(ctx, orgID, batchID); err != nil {
		return s.processError(err)
	}

	cc := chunkCtx{
		OrganizationID: orgID,
		IntegrationID:  batch.IntegrationID,
		WarehouseID:    batch.WarehouseID,
		BranchID:       batch.BranchID,
		Provider:       batch.Provider,
	}
	pending, err := s.repo.LoadPendingItems(ctx, orgID, batchID, chunkNo)
	if err != nil {
		return s.processError(err)
	}
	plan, err := s.resolveItems(ctx, cc, batchID, chunkNo, pending)
	if err != nil {
		return s.processError(err)
	}
	if err := s.writeResolved(ctx, cc, plan); err != nil {
		return s.processError(err)
	}
	if _, err := s.repo.FinishChunk(ctx, orgID, batchID); err != nil {
		return s.processError(err)
	}
	return nil
}

// processError maps repository failures onto a wrapped error; the worker
// passes everything except ErrWaitForEarlierBatch through to River's retries.
func (s *Service) processError(err error) error {
	return fmt.Errorf("erp: process chunk: %w", err)
}
