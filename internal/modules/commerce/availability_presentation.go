package commerce

import "github.com/muhiya/dawa24-store/internal/shared/i18n"

// Disposition is how a buying surface should present a refusal.
type Disposition int

const (
	DispositionOrderable Disposition = iota // show, enabled
	DispositionBlocked                      // show, disabled, with MessageAr
	DispositionHidden                       // do not list this offer for this buyer
)

// Disposition classifies a result for the UI. It is the ONLY place this
// mapping exists; three surfaces used to carry their own copy.
func (r AvailabilityResult) Disposition() Disposition {
	if r.Allowed {
		return DispositionOrderable
	}
	switch r.Reason {
	case ReasonOK, ReasonBelowMinimum:
		return DispositionOrderable
	case ReasonOutOfStock, ReasonInsufficientStock, ReasonQuotaExhausted, ReasonQuotaExceeded:
		return DispositionBlocked
	default:
		// Hidden includes: ReasonNotCovered, ReasonBranchNoLocation,
		// ReasonBranchNoInstitutionalWorks, ReasonBranchInstitutionalMismatch,
		// ReasonOwnOrganization, ReasonVendorInvalid, ReasonVendorUnapproved,
		// ReasonWrongVendor, ReasonVariantInvalid, ReasonVariantInactive,
		// ReasonBranchInvalid, ReasonBranchNotOwned, ReasonQuantityInvalid,
		// and any unexpected/probe error.
		return DispositionHidden
	}
}

// IsOrderable reports whether the result allows the item to be added to cart.
func (r AvailabilityResult) IsOrderable() bool {
	return r.Disposition() == DispositionOrderable
}

// IsBlocked reports whether the result should show the item disabled with an explanation.
func (r AvailabilityResult) IsBlocked() bool {
	return r.Disposition() == DispositionBlocked
}

// IsHidden reports whether the offer should be excluded from display entirely.
func (r AvailabilityResult) IsHidden() bool {
	return r.Disposition() == DispositionHidden
}

// DisplayReasonAr returns the Arabic reason to display, with fallback.
func (r AvailabilityResult) DisplayReasonAr() string {
	if r.MessageAr != "" {
		return r.MessageAr
	}
	if r.MessageEn != "" {
		return r.MessageEn
	}
	return i18n.T("ar", "offers.cov_reason_verify_failed")
}
