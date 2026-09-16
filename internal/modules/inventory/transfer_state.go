package inventory

import "github.com/muhiya/dawa24-store/internal/shared/apperr"

// Transfers between warehouses are two-phase, not atomic.
//
// The obvious implementation deducts the source and credits the destination in
// one step. It is wrong for physical goods: the destination would show stock it
// has not received yet, and a buyer could order against it. In pharmaceutical
// distribution that means promising a pharmacy medicine that is still on a van.
//
// So a transfer moves through:
//
//	in_transit  — source deducted, goods dispatched, destination NOT credited
//	completed   — goods received, destination credited
//	cancelled   — goods never left or came back, source restored
//
// Stock in transit belongs to neither warehouse's sellable quantity, which is
// the honest representation of where it actually is.
//
// `pending` exists for a future approval step (a manager authorising a transfer
// before it is dispatched). It is reachable in the state machine but nothing
// creates it yet.
var allowedTransferTransitions = map[TransferStatus][]TransferStatus{
	TransferPending:   {TransferInTransit, TransferCancelled},
	TransferInTransit: {TransferCompleted, TransferCancelled},
	TransferCompleted: {},
	TransferCancelled: {},
}

// CanTransitionTo reports whether a transfer may move from its current status
// to the requested one.
func (t *WarehouseTransfer) CanTransitionTo(next TransferStatus) bool {
	for _, allowed := range allowedTransferTransitions[t.Status] {
		if allowed == next {
			return true
		}
	}
	return false
}

// TransitionTo validates and applies a status change.
//
// Returning a conflict rather than silently ignoring the change matters: two
// warehouse staff clicking "receive" at the same moment must not both credit
// the destination. The second call fails loudly.
func (t *WarehouseTransfer) TransitionTo(next TransferStatus) error {
	if t.Status == next {
		return apperr.Conflict("transfer.already_"+string(next),
			"This transfer is already in that state.")
	}
	if !t.CanTransitionTo(next) {
		return apperr.Conflict("transfer.invalid_transition",
			"A transfer cannot move from "+string(t.Status)+" to "+string(next)+".")
	}
	t.Status = next
	return nil
}

// IsTerminal reports whether the transfer can no longer change state.
func (t *WarehouseTransfer) IsTerminal() bool {
	return len(allowedTransferTransitions[t.Status]) == 0
}
