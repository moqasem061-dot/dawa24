package promo

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Reading a package's statement.

// CreditStatementFor returns one purchase's ledger with its totals.
//
// The purchase is fetched first and checked against the caller's tenant: the
// ledger table is RLS-protected, but a caller reading another company's
// purchase id would otherwise get an empty statement rather than a refusal, and
// "no movements" and "not yours" are different answers.
func (s *Service) CreditStatementFor(
	ctx context.Context, purchaseID int64, limit, offset int,
) (*CreditStatement, int, error) {
	if purchaseID <= 0 {
		return nil, 0, apperr.Validation("promo.credit_invalid_purchase",
			"A valid purchase is required.", nil)
	}
	if limit <= 0 {
		limit = 25
	}

	purchase, err := s.repo.GetSponsorshipPurchaseByID(database.AsSystem(ctx), purchaseID)
	if err != nil {
		return nil, 0, err
	}
	if orgID, ok := database.TenantFrom(ctx); ok && purchase.OrganizationID != orgID {
		return nil, 0, apperr.NotFound("sponsorship_purchase")
	}

	entries, total, err := s.repo.ListCreditEntries(
		database.AsSystem(ctx), purchaseID, limit, offset)
	if err != nil {
		return nil, 0, err
	}

	// The totals describe the whole purchase, not the page in hand.
	consumed, refunded, err := s.repo.CreditTotals(database.AsSystem(ctx), purchaseID)
	if err != nil {
		return nil, 0, err
	}

	return &CreditStatement{
		Purchase:  purchase,
		Entries:   entries,
		Total:     purchase.CreditsTotal,
		Consumed:  consumed,
		Refunded:  refunded,
		Remaining: purchase.CreditsRemainingInt(),
	}, total, nil
}

// OrgCreditEntries returns one company's movements across every package, for
// the administrator's per-organization view.
func (s *Service) OrgCreditEntries(
	ctx context.Context, orgID int64, limit, offset int,
) ([]*CreditEntry, int, error) {
	if orgID <= 0 {
		return nil, 0, apperr.Validation("promo.credit_invalid_org",
			"A valid organization is required.", nil)
	}
	if limit <= 0 {
		limit = 25
	}
	return s.repo.ListOrgCreditEntries(ctx, orgID, limit, offset)
}

// CreditAccounts lists every company holding packages, newest spend first.
func (s *Service) CreditAccounts(
	ctx context.Context, search string, limit, offset int,
) ([]*CreditAccount, int, error) {
	if limit <= 0 {
		limit = 25
	}
	return s.repo.ListCreditAccounts(ctx, search, limit, offset)
}

// PurchasesForOrg returns one company's purchases for the drill-down.
func (s *Service) PurchasesForOrg(
	ctx context.Context, orgID int64, limit, offset int,
) ([]*SponsorshipPurchase, int, error) {
	if orgID <= 0 {
		return nil, 0, apperr.Validation("promo.credit_invalid_org",
			"A valid organization is required.", nil)
	}
	if limit <= 0 {
		limit = 25
	}
	return s.repo.ListPurchasesForOrg(ctx, orgID, limit, offset)
}
