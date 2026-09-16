package inventory

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// ReceiveTransfer credits the destination warehouse and completes the transfer.
//
// This is the second half of the two-phase move started by TransferStock. The
// source was already deducted at dispatch; between the two calls the goods
// belong to neither warehouse, which is what the physical world looks like.
func (s *Service) ReceiveTransfer(ctx context.Context, transferID int64) (*WarehouseTransfer, error) {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return nil, database.ErrNoTenant
	}

	transfer, err := s.repo.GetTransferByID(ctx, transferID)
	if err != nil {
		return nil, err
	}

	// The in-memory state machine rejects an obviously invalid transition and
	// gives a clear error message.
	if err := transfer.TransitionTo(TransferCompleted); err != nil {
		return nil, err
	}

	// Claim the transfer BEFORE crediting any stock.
	//
	// Each repository call runs in its own transaction, so these two writes are
	// not atomic. Ordering therefore decides which way an interleaving fails.
	// Claiming first means the loser of a concurrent receive credits nothing,
	// which is safe. Crediting first would let both callers add stock and only
	// then discover one of them had lost — stock created from nothing.
	//
	// Known limitation: if the credit below fails after a successful claim, the
	// transfer reads `completed` with no matching movement. That is detectable
	// (a completed transfer with no inbound movement referencing it) and
	// recoverable, unlike invented inventory. Closing it properly needs a
	// repository method that spans both writes in one transaction.
	if err := s.repo.UpdateTransferStatus(ctx, transfer.ID, TransferInTransit, TransferCompleted); err != nil {
		return nil, err
	}

	// The destination row may not exist yet — this can be the first time that
	// variant has ever been held here.
	toStock, err := s.repo.GetStock(ctx, transfer.ToWarehouseID, transfer.ProductVariantID)
	if (err != nil && apperr.KindOf(err) == apperr.KindNotFound) || (err == nil && toStock == nil) {
		toStock = &Stock{
			OrganizationID:   orgID,
			WarehouseID:      transfer.ToWarehouseID,
			ProductID:        transfer.ProductID,
			ProductVariantID: transfer.ProductVariantID,
			Quantity:         0,
			MinThreshold:     1,
		}
		if err := s.repo.UpsertStock(ctx, toStock); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}

	if _, err := s.repo.AdjustStock(ctx, toStock.ID, transfer.Quantity, StockMovement{
		Type:          MovementTransfer,
		QuantityDelta: transfer.Quantity,
		Details:       "Inbound transfer received",
		ReferenceID:   &transfer.ID,
		UserID:        transfer.InitiatedBy,
	}); err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "warehouse transfer received",
		"transfer_id", transfer.ID, "quantity", transfer.Quantity)
	return transfer, nil
}

// CancelTransfer aborts a transfer and returns the goods to the source.
//
// Cancelling an in-transit transfer must restore the source quantity, otherwise
// the deduction made at dispatch silently destroys stock.
func (s *Service) CancelTransfer(ctx context.Context, transferID int64, reason string) (*WarehouseTransfer, error) {
	if _, ok := database.TenantFrom(ctx); !ok {
		return nil, database.ErrNoTenant
	}

	transfer, err := s.repo.GetTransferByID(ctx, transferID)
	if err != nil {
		return nil, err
	}

	priorStatus := transfer.Status
	wasDispatched := priorStatus == TransferInTransit

	if err := transfer.TransitionTo(TransferCancelled); err != nil {
		return nil, err
	}

	// Claim before restoring, for the same reason as ReceiveTransfer: a lost
	// race must not return stock to the source twice.
	if err := s.repo.UpdateTransferStatus(ctx, transfer.ID, priorStatus, TransferCancelled); err != nil {
		return nil, err
	}

	if wasDispatched {
		fromStock, err := s.repo.GetStock(ctx, transfer.FromWarehouseID, transfer.ProductVariantID)
		if err != nil {
			return nil, err
		}
		if _, err := s.repo.AdjustStock(ctx, fromStock.ID, transfer.Quantity, StockMovement{
			Type:          MovementTransfer,
			QuantityDelta: transfer.Quantity,
			Details:       "Transfer cancelled, stock returned to source: " + reason,
			ReferenceID:   &transfer.ID,
			UserID:        transfer.InitiatedBy,
		}); err != nil {
			return nil, err
		}
	}

	s.log.InfoContext(ctx, "warehouse transfer cancelled",
		"transfer_id", transfer.ID, "restored", wasDispatched)
	return transfer, nil
}

// ListTransfers returns transfers for the active tenant, newest first.
func (s *Service) ListTransfers(ctx context.Context, status string, limit, offset int) ([]*WarehouseTransfer, error) {
	if _, ok := database.TenantFrom(ctx); !ok {
		return nil, database.ErrNoTenant
	}

	switch TransferStatus(status) {
	case "", TransferPending, TransferInTransit, TransferCompleted, TransferCancelled:
	default:
		return nil, apperr.Validation("transfer.unknown_status",
			"Unknown transfer status filter.", map[string]string{"status": status})
	}

	return s.repo.ListTransfers(ctx, status, limit, offset)
}

// ListTransfersWithTotal returns transfers for the active tenant with total count.
func (s *Service) ListTransfersWithTotal(ctx context.Context, status string, limit, offset int) ([]*WarehouseTransfer, int, error) {
	if _, ok := database.TenantFrom(ctx); !ok {
		return nil, 0, database.ErrNoTenant
	}

	switch TransferStatus(status) {
	case "", TransferPending, TransferInTransit, TransferCompleted, TransferCancelled:
	default:
		return nil, 0, apperr.Validation("transfer.unknown_status",
			"Unknown transfer status filter.", map[string]string{"status": status})
	}

	return s.repo.ListTransfersWithTotal(ctx, status, limit, offset)
}

// GetTransfer returns a single transfer.
func (s *Service) GetTransfer(ctx context.Context, id int64) (*WarehouseTransfer, error) {
	if _, ok := database.TenantFrom(ctx); !ok {
		return nil, database.ErrNoTenant
	}
	return s.repo.GetTransferByID(ctx, id)
}
