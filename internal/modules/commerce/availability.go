package commerce

import (
	"context"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// Availability is the single source of truth for "may this company buy this
// quantity of this variant from this supplier, right now?".
//
// Before this existed, AddToCartSubmit made the decision inline and got it
// wrong five ways: a missing vendor id silently became organization 1, a variant
// with StockQty == 0 skipped validation entirely because the check was guarded
// by StockQty > 0, an over-stock quantity was silently clamped instead of
// refused, the stock lookup's error was swallowed, and nothing checked coverage
// or branch eligibility at all. Every surface now calls CheckAvailability so
// those rules cannot drift apart again.

// Reason is a machine-readable refusal code. The UI maps it to a message; tests
// assert on it rather than on Arabic text.
type Reason string

const (
	ReasonOK                          Reason = ""
	ReasonVendorInvalid               Reason = "vendor_invalid"
	ReasonVendorUnapproved            Reason = "vendor_unapproved"
	ReasonVariantInvalid              Reason = "variant_invalid"
	ReasonVariantInactive             Reason = "variant_inactive"
	ReasonWrongVendor                 Reason = "wrong_vendor"
	ReasonOutOfStock                  Reason = "out_of_stock"
	ReasonInsufficientStock           Reason = "insufficient_stock"
	ReasonBelowMinimum                Reason = "below_minimum"
	ReasonBranchInvalid               Reason = "branch_invalid"
	ReasonBranchNotOwned              Reason = "branch_not_owned"
	ReasonBranchNoLocation            Reason = "branch_no_location"
	ReasonBranchNoInstitutionalWorks  Reason = "branch_no_institutional_works"
	ReasonBranchInstitutionalMismatch Reason = "branch_institutional_mismatch"
	ReasonNotCovered                  Reason = "not_covered"
	ReasonQuantityInvalid             Reason = "quantity_invalid"
	// ReasonQuotaExhausted and ReasonQuotaExceeded are the per-branch purchase
	// quota. They are separate reasons rather than one, because the two need
	// different screens: "this branch has taken its whole allowance" ends the
	// conversation, while "you asked for 8 and 3 remain" is a number the
	// pharmacy can act on. MaxQuantity carries that remainder.
	ReasonQuotaExhausted Reason = "quota_exhausted"
	ReasonQuotaExceeded  Reason = "quota_exceeded"
	// ReasonOwnOrganization refuses a company buying from itself. Smart
	// Ordering has always called this ReasonOwnOrg and refused it first; this
	// is the same invariant on the ordinary purchase path.
	ReasonOwnOrganization Reason = "own_organization"
)

// VariantAvailability is the slice of a catalog variant that the rule needs.
type VariantAvailability struct {
	ID             int64
	OrganizationID int64
	VendorBranchID int64
	StockQty       int
	MinOrderQty    int
	Active         bool
	// QuotaLimit is the supplier's per-branch cap on this variant, 0 meaning
	// unlimited. It is the cap alone; what a branch has already taken against
	// it is commerce's own arithmetic and is read here, not supplied by the
	// probe.
	QuotaLimit             int
	Price                  money.Amount
	Discount               money.Amount
	EffectivePrice         money.Amount
	IsNegotiable           bool
	CostPrice              *money.Amount
	CostDiscountPercentage float64
}

// VendorAvailability is the slice of a supplier organization that the rule needs.
type VendorAvailability struct {
	ID       int64
	IsVendor bool
	Approved bool
}

// BranchAvailability is the slice of the buyer's own branch that the rule needs.
type BranchAvailability struct {
	ID                 int64
	OrganizationID     int64
	CityID             *int64
	Latitude           *float64
	Longitude          *float64
	InstitutionalWorks []string
}

// AvailabilityProbe is what commerce needs from the catalog, org and workflow
// modules. Modules must not import each other (ADR 0002), so the composition
// root in cmd/server/routes.go implements this over the real services — the
// same shape as RequiredDocsChecker.
type AvailabilityProbe interface {
	Variant(ctx context.Context, variantID int64) (VariantAvailability, error)
	Vendor(ctx context.Context, orgID int64) (VendorAvailability, error)
	CustomerBranch(ctx context.Context, branchID int64) (BranchAvailability, error)
	VendorCovers(ctx context.Context, vendorOrgID, vendorBranchID int64, lat, lon float64, day time.Weekday, cityID *int64) (bool, error)
	VendorInstitutionalConnection(ctx context.Context, vendorOrgID int64, customerBranchID int64, variantID int64) (bool, error)

	VariantsByIDs(ctx context.Context, variantIDs []int64) (map[int64]VariantAvailability, error)
	VendorsByIDs(ctx context.Context, orgIDs []int64) (map[int64]VendorAvailability, error)
	VendorInstitutionalConnections(ctx context.Context, customerBranchID int64, lines []AvailabilityLine) (map[int64]bool, error)
}

// AvailabilityRequest describes one prospective purchase line.
type AvailabilityRequest struct {
	VariantID        int64
	VendorOrgID      int64
	CustomerOrgID    int64
	CustomerBranchID int64
	Quantity         int
	When             time.Time // decides which weekday's coverage applies
}

// AvailabilityResult is the verdict. MaxQuantity is filled whenever the caller
// asked for more than is available, so the UI can say how many there actually
// are instead of silently reducing the number the pharmacy typed.
type AvailabilityResult struct {
	Allowed     bool
	MaxQuantity int
	Reason      Reason
	MessageAr   string
	MessageEn   string
}

func denied(reason Reason, max int, ar, en string) AvailabilityResult {
	return AvailabilityResult{Allowed: false, MaxQuantity: max, Reason: reason, MessageAr: ar, MessageEn: en}
}

// Message returns the localized failure message for the given language.
func (r AvailabilityResult) Message(lang string) string {
	if lang == "en" && r.MessageEn != "" {
		return r.MessageEn
	}
	if r.MessageAr != "" {
		return r.MessageAr
	}
	return r.MessageEn
}


// CheckAvailability runs every purchase precondition in order and returns the
// first failure. It delegates to CheckAvailabilityBatch so there is exactly
// one implementation of the availability rule across the platform.
func (s *Service) CheckAvailability(ctx context.Context, req AvailabilityRequest) (AvailabilityResult, error) {
	resMap, err := s.CheckAvailabilityBatch(ctx, req.CustomerOrgID, req.CustomerBranchID, req.When, []AvailabilityLine{
		{
			VariantID:   req.VariantID,
			VendorOrgID: req.VendorOrgID,
			Quantity:    req.Quantity,
		},
	})
	if err != nil {
		return AvailabilityResult{}, err
	}
	return resMap[req.VariantID], nil
}

// revalidateCheckoutLines re-runs the availability rule over every line being
// ordered. It exists because availability at add-to-cart time is not a promise:
// a cart can sit open for hours while another pharmacy takes the last unit or
// the supplier drops the delivery day.
//
// A refusal fails the whole checkout with the line's own reason rather than
// silently dropping that line — a pharmacy must not discover at delivery that
// part of its order quietly vanished.
func (s *Service) revalidateCheckoutLines(ctx context.Context, input CheckoutInput) error {
	if s.availability == nil {
		return nil // no probe wired: CheckAvailability already fails closed at the surfaces
	}
	branchID := int64(0)
	if input.BranchID != nil {
		branchID = *input.BranchID
	}

	// One basket can name the same variant twice — the plain listing and an
	// offer both add it, and the cart keys on the cart item rather than on the
	// variant. Checking each line on its own would then measure 3 and 3
	// against a stock of 5 or a remaining quota of 4 and pass both. The whole
	// order's demand for a variant is what has to fit, so it is summed first.
	demand := make(map[int64]int, len(input.Items))
	for _, item := range input.Items {
		if item.ProductVariantID == nil || *item.ProductVariantID <= 0 {
			continue
		}
		demand[*item.ProductVariantID] += item.Quantity
	}

	checked := make(map[int64]bool, len(demand))
	for _, item := range input.Items {
		if item.ProductVariantID == nil || *item.ProductVariantID <= 0 {
			continue // a line with no variant carries no stock to check
		}
		if checked[*item.ProductVariantID] {
			continue
		}
		checked[*item.ProductVariantID] = true
		res, err := s.CheckAvailability(ctx, AvailabilityRequest{
			VariantID:        *item.ProductVariantID,
			VendorOrgID:      item.VendorOrgID,
			CustomerOrgID:    input.CustomerOrgID,
			CustomerBranchID: branchID,
			Quantity:         demand[*item.ProductVariantID],
			When:             time.Now(),
		})
		if err != nil {
			return err
		}
		if !res.Allowed {
			msg := res.MessageAr
			if msg == "" {
				msg = res.MessageEn
			}
			return apperr.Validation(
				"checkout.line_unavailable."+string(res.Reason),
				msg,
				map[string]string{"product": msg},
			)
		}
	}
	return nil
}
