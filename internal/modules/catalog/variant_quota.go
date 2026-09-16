package catalog

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// The per-branch purchase quota a supplier may put on one of its variants.
//
// The cap lives here because it is a property of the thing being sold. What has
// been consumed against it does not: that is the sum of the orders placed, and
// it belongs to commerce. Keeping the two apart is what stops a counter in this
// table drifting away from the orders it is supposed to describe.
//
// A quota is *per buying branch*, not per company and not per order. Two
// branches of the same pharmacy chain each get the full allowance, because the
// allocation is about where the stock physically lands.

// MaxQuotaLimit is the largest cap a supplier may set. It is not a business
// rule so much as a guard: a hand-edited form field asking for two billion
// would overflow nothing here but would make the column's meaning ("a real
// restriction") a lie, and no pharmacy branch orders a million packs.
const MaxQuotaLimit = 1_000_000

// NormalizeQuotaLimit folds every way of saying "no quota" into nil.
//
// Zero and negatives arrive from form parsing, from JSON that sent 0 rather
// than omitting the field, and from a supplier clearing the box. All three mean
// the same thing, and the column's CHECK constraint refuses anything but NULL
// or a positive number — so the normalisation happens once, here, rather than
// at each of the six call sites that write a variant.
func NormalizeQuotaLimit(limit *int) *int {
	if limit == nil || *limit <= 0 {
		return nil
	}
	return limit
}

// ValidateQuotaLimit checks a supplier-supplied cap before it is written.
func ValidateQuotaLimit(limit *int) error {
	if limit == nil {
		return nil
	}
	if *limit < 0 {
		return apperr.Validation("variant.quota_negative",
			"The quota limit cannot be negative.", nil)
	}
	if *limit > MaxQuotaLimit {
		return apperr.Validation("variant.quota_too_large",
			"The quota limit is unreasonably large.", nil)
	}
	return nil
}

// HasQuota reports whether this variant restricts how much one branch may take.
func (v *ProductVariant) HasQuota() bool {
	return v != nil && v.QuotaLimit != nil && *v.QuotaLimit > 0
}

// QuotaLimitOrZero is the cap as a plain integer, 0 meaning unlimited. It exists
// for the boundaries that carry the fact across a module edge, where a pointer
// would only invite a nil dereference.
func (v *ProductVariant) QuotaLimitOrZero() int {
	if !v.HasQuota() {
		return 0
	}
	return *v.QuotaLimit
}

// VariantQuotaBackend is the persistence the quota screen needs beyond the
// ordinary variant writes.
type VariantQuotaBackend interface {
	SetVariantQuotaLimit(ctx context.Context, orgID, variantID int64, limit *int) error
	CountVariantsWithQuota(ctx context.Context, orgID int64) (int, error)
}

// SetVariantQuotaLimit sets or removes the per-branch cap on one variant.
//
// A nil limit removes the quota entirely, which is the remove-quota action on
// the supplier's quota screen: every branch may buy freely again, and the
// release rows recorded against it become inert rather than being deleted — a
// supplier who re-imposes a quota tomorrow should not silently resurrect a
// branch's consumption from before they lifted it.
func (s *Service) SetVariantQuotaLimit(ctx context.Context, variantID int64, limit *int) error {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return database.ErrNoTenant
	}
	if variantID <= 0 {
		return apperr.Validation("variant.id_required", "A product variant is required.", nil)
	}
	limit = NormalizeQuotaLimit(limit)
	if err := ValidateQuotaLimit(limit); err != nil {
		return err
	}
	backend, ok := s.repo.(VariantQuotaBackend)
	if !ok {
		return ErrBulkVariantsUnavailable
	}
	return backend.SetVariantQuotaLimit(ctx, orgID, variantID, limit)
}

// CountVariantsWithQuota is the headline figure on the quota screen: how many of
// this supplier's items are restricted at all.
func (s *Service) CountVariantsWithQuota(ctx context.Context, orgID int64) (int, error) {
	backend, ok := s.repo.(VariantQuotaBackend)
	if !ok {
		return 0, nil
	}
	return backend.CountVariantsWithQuota(ctx, orgID)
}
