package pages

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

// CheckoutBranchItem holds a customer's branch alongside its cart availability and refusal reason.
type CheckoutBranchItem struct {
	Branch      *org.Branch
	IsAvailable bool
	Reason      string
}

// HasAvailableCheckoutBranch returns true if at least one branch in the list can receive the order.
func HasAvailableCheckoutBranch(branches []*CheckoutBranchItem) bool {
	for _, b := range branches {
		if b != nil && b.IsAvailable {
			return true
		}
	}
	return false
}

// IsCheckoutBranchChecked determines if a branch should be selected by default.
// An unavailable branch is NEVER selected by default.
func IsCheckoutBranchChecked(ctx context.Context, b *CheckoutBranchItem, branches []*CheckoutBranchItem) bool {
	if b == nil || !b.IsAvailable {
		return false
	}
	// 1. If an active branch is stored in buying context, prefer it IF it is available:
	if buying, has := authctx.BuyingBranchFrom(ctx); has && buying.Active != nil {
		if b.Branch != nil && b.Branch.ID == *buying.Active {
			return true
		}
		// Check if that active branch is available in our list:
		activeFoundAndAvail := false
		for _, other := range branches {
			if other != nil && other.Branch != nil && other.Branch.ID == *buying.Active && other.IsAvailable {
				activeFoundAndAvail = true
				break
			}
		}
		// If the active branch was indeed available, we don't select this other branch:
		if activeFoundAndAvail {
			return false
		}
	}

	// 2. If no available active branch, pick the main branch if available:
	if b.Branch != nil && b.Branch.IsMain {
		return true
	}

	// 3. Otherwise pick the first available branch in the list:
	for _, other := range branches {
		if other != nil && other.IsAvailable {
			return other.Branch != nil && b.Branch != nil && other.Branch.ID == b.Branch.ID
		}
	}
	return false
}
