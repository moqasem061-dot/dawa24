package promo

import (
	"context"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// WalletDebiter is injected from the composition root so the promo module can
// charge a vendor's wallet for a sponsorship package purchase without
// importing the billing module (ADR 0002). It debits amount from the org's
// wallet and returns the wallet transaction reference.
type WalletDebiter func(ctx context.Context, orgID int64, amount money.Amount, description string) (paymentID *int64, err error)

// SetWalletDebiter installs the wallet payment hook.
func (s *Service) SetWalletDebiter(fn WalletDebiter) { s.walletDebit = fn }

// PurchaseSponsorshipPackage buys a sponsorship package for the vendor's
// organization, charging their wallet and creating a purchase record with the
// package's credits. Returns the purchase with its remaining credits.
func (s *Service) PurchaseSponsorshipPackage(ctx context.Context, packageID int64, autoRenew bool, billingCycle string) (*SponsorshipPurchase, error) {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return nil, database.ErrNoTenant
	}

	pkg, err := s.repo.GetPackageByID(ctx, packageID)
	if err != nil {
		return nil, err
	}
	if pkg == nil {
		return nil, apperr.NotFound("package")
	}
	if !pkg.IsActive {
		return nil, apperr.Validation("package.inactive", i18n.TDefault("w4_mod.w4str_247_247"), nil)
	}

	cost := pkg.Price
	if billingCycle != "annual" && billingCycle != "monthly" {
		billingCycle = "monthly"
	}
	durationDays := pkg.DurationDays
	if durationDays <= 0 {
		durationDays = 30
	}

	// Charge wallet if the package has a price and a debiter is wired.
	var sourceID *int64
	if s.walletDebit != nil && cost.IsPositive() {
		desc := i18n.TDefault("w4_mod.w4str_248_248") + pkg.Name.Get("ar")
		txID, err := s.walletDebit(ctx, orgID, cost, desc)
		if err != nil {
			return nil, err
		}
		sourceID = txID
	}

	now := time.Now().UTC()
	purchase := &SponsorshipPurchase{
		OrganizationID: orgID,
		PackageID:      packageID,
		CreditsTotal:   pkg.Credits,
		CreditsUsed:    0,
		StartsAt:       now,
		ExpiresAt:      now.Add(time.Duration(durationDays) * 24 * time.Hour),
		Status:         PurchaseActive,
		AutoRenew:      autoRenew,
		BillingCycle:   billingCycle,
		Amount:         cost,
		PaymentID:      nil,
		SourceSystem:   "wallet_checkout",
		SourceID:       sourceID,
	}

	if err := s.repo.CreateSponsorshipPurchase(ctx, purchase); err != nil {
		return nil, err
	}
	purchase.CreditsRemaining = purchase.CreditsRemainingInt()

	s.log.InfoContext(ctx, "sponsorship package purchased", "org_id", orgID, "package_id", packageID, "credits", pkg.Credits, "cost", cost.String())
	return purchase, nil
}

// ListSponsorshipPurchases returns all purchases for the active tenant.
func (s *Service) ListSponsorshipPurchases(ctx context.Context) ([]*SponsorshipPurchase, error) {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return nil, database.ErrNoTenant
	}
	purchases, err := s.repo.ListSponsorshipPurchasesByOrg(ctx, orgID)
	if err != nil {
		return nil, err
	}
	for _, p := range purchases {
		if p != nil {
			p.CreditsRemaining = p.CreditsRemainingInt()
		}
	}
	return purchases, nil
}

// ListActiveSponsorshipPurchases returns only active purchases.
func (s *Service) ListActiveSponsorshipPurchases(ctx context.Context) ([]*SponsorshipPurchase, error) {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return nil, database.ErrNoTenant
	}
	purchases, err := s.repo.ListActiveSponsorshipPurchasesByOrg(ctx, orgID)
	if err != nil {
		return nil, err
	}
	for _, p := range purchases {
		if p != nil {
			p.CreditsRemaining = p.CreditsRemainingInt()
		}
	}
	return purchases, nil
}

// SubmitSponsorshipRequest creates a sponsorship request for a product or
// offer, consuming one credit from the vendor's active purchase. The request
// starts in pending status and must be admin-approved before it becomes active.
func (s *Service) SubmitSponsorshipRequest(ctx context.Context, itemType SponsorshipItemType, itemID, packageID int64) (*SponsorshipRequest, error) {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return nil, database.ErrNoTenant
	}

	pkg, err := s.repo.GetPackageByID(ctx, packageID)
	if err != nil {
		return nil, err
	}

	// Find an active purchase with remaining credits for this package.
	purchases, err := s.repo.ListActiveSponsorshipPurchasesByOrg(ctx, orgID)
	if err != nil {
		return nil, err
	}
	var purchase *SponsorshipPurchase
	for _, p := range purchases {
		if p != nil && p.PackageID == packageID && p.CreditsRemainingInt() > 0 {
			purchase = p
			break
		}
	}
	if purchase == nil {
		return nil, apperr.Conflict("sponsorship.no_credits", i18n.TDefault("w4_mod.w4str_249_249"))
	}

	now := time.Now().UTC()
	expiresAt := now.Add(time.Duration(pkg.DurationDays) * 24 * time.Hour)
	if expiresAt.After(purchase.ExpiresAt) {
		expiresAt = purchase.ExpiresAt
	}

	sr := &SponsorshipRequest{
		OrganizationID: orgID,
		PurchaseID:     &purchase.ID,
		PackageID:      packageID,
		ItemType:       itemType,
		ItemID:         itemID,
		CreditsUsed:    1,
		AdminStatus:    AdminPending,
		Status:         SRSPending,
		StartsAt:       now,
		ExpiresAt:      expiresAt,
	}

	if err := s.repo.CreateSponsorshipRequest(ctx, sr); err != nil {
		return nil, err
	}

	// Reserve the credit immediately so concurrent submissions don't overspend.
	// The credit is refunded if the admin rejects the request (in the admin
	// activation path the credit was already reserved at submission time, so
	// rejection releases it).
	if _, err := s.repo.ConsumeSponsorshipCredits(ctx, ConsumeCredits{
		OrganizationID: orgID,
		PurchaseID:     purchase.ID,
		Credits:        1,
		Reason:         CreditSponsorshipRequested,
		EntityType:     string(itemType),
		EntityID:       &sr.ID,
	}); err != nil {
		// The request exists and the reservation did not happen. Silence here
		// let a vendor submit more requests than they had credits for.
		s.log.ErrorContext(ctx, "sponsorship request: credit reservation failed",
			"error", err, "request_id", sr.ID, "purchase_id", purchase.ID)
	}

	s.log.InfoContext(ctx, "sponsorship request submitted", "request_id", sr.ID, "item_type", string(itemType), "item_id", itemID, "package_id", packageID)
	return sr, nil
}

// SubmitBatchSponsorshipRequests creates sponsorship requests for multiple products/offers,
// consuming 1 credit per item from the vendor's active purchase.
func (s *Service) SubmitBatchSponsorshipRequests(ctx context.Context, itemType SponsorshipItemType, itemIDs []int64, packageID int64) ([]*SponsorshipRequest, error) {
	if len(itemIDs) == 0 {
		return nil, apperr.Validation("sponsorship.items_required", "يرجى تحديد عنصر واحد على الأقل للرعاية.", nil)
	}

	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return nil, database.ErrNoTenant
	}

	pkg, err := s.repo.GetPackageByID(ctx, packageID)
	if err != nil {
		return nil, err
	}

	totalCreditsNeeded := len(itemIDs) * SponsorshipCreditCost

	purchases, err := s.repo.ListActiveSponsorshipPurchasesByOrg(ctx, orgID)
	if err != nil {
		return nil, err
	}
	var purchase *SponsorshipPurchase
	for _, p := range purchases {
		if p != nil && p.PackageID == packageID && p.CreditsRemainingInt() >= totalCreditsNeeded {
			purchase = p
			break
		}
	}
	if purchase == nil {
		return nil, apperr.Conflict("sponsorship.no_credits", "رصيد الرعاية في الباقة المختارة غير كافٍ لعدد الأصناف المحددة.")
	}

	now := time.Now().UTC()
	expiresAt := now.Add(time.Duration(pkg.DurationDays) * 24 * time.Hour)
	if expiresAt.After(purchase.ExpiresAt) {
		expiresAt = purchase.ExpiresAt
	}

	var created []*SponsorshipRequest
	for _, itemID := range itemIDs {
		if itemID <= 0 {
			continue
		}
		sr := &SponsorshipRequest{
			OrganizationID: orgID,
			PurchaseID:     &purchase.ID,
			PackageID:      packageID,
			ItemType:       itemType,
			ItemID:         itemID,
			CreditsUsed:    SponsorshipCreditCost,
			AdminStatus:    AdminPending,
			Status:         SRSPending,
			StartsAt:       now,
			ExpiresAt:      expiresAt,
		}
		if err := s.repo.CreateSponsorshipRequest(ctx, sr); err != nil {
			return created, err
		}
		created = append(created, sr)
	}

	if len(created) > 0 {
		if _, err := s.repo.ConsumeSponsorshipCredits(ctx, ConsumeCredits{
			OrganizationID: orgID,
			PurchaseID:     purchase.ID,
			Credits:        len(created) * SponsorshipCreditCost,
			Reason:         CreditSponsorshipBatch,
			EntityType:     string(itemType),
		}); err != nil {
			s.log.ErrorContext(ctx, "batch sponsorship: credit reservation failed",
				"error", err, "count", len(created), "purchase_id", purchase.ID)
		}
	}

	s.log.InfoContext(ctx, "batch sponsorship requests submitted", "count", len(created), "org_id", orgID, "package_id", packageID)
	return created, nil
}

// ListSponsorshipRequestsByOrg returns the vendor's sponsorship requests.
func (s *Service) ListSponsorshipRequestsByOrg(ctx context.Context, limit, offset int) ([]*SponsorshipRequest, error) {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return nil, database.ErrNoTenant
	}
	return s.repo.ListSponsorshipRequestsByOrg(ctx, orgID, limit, offset)
}

// GetSponsorshipRequestByID retrieves a request by ID (tenant-scoped).
func (s *Service) GetSponsorshipRequestByID(ctx context.Context, id int64) (*SponsorshipRequest, error) {
	sr, err := s.repo.GetSponsorshipRequestByID(ctx, id)
	if err != nil {
		return nil, err
	}
	orgID, ok := database.TenantFrom(ctx)
	if ok && sr.OrganizationID != orgID {
		return nil, apperr.NotFound("sponsorship_request")
	}
	return sr, nil
}

// CancelSponsorshipRequest lets a vendor withdraw their own pending request and
// gives the reserved credit back.
//
// The credit is taken when the request is submitted, so cancelling has to
// return it — exactly as rejection does. Without the refund a vendor who
// changed their mind simply lost the credit, with nothing in the UI to say so.
func (s *Service) CancelSponsorshipRequest(ctx context.Context, id int64) error {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return database.ErrNoTenant
	}

	// Read it before cancelling: afterwards it is no longer pending and the
	// reservation it held cannot be identified.
	sr, err := s.repo.GetSponsorshipRequestByID(database.AsSystem(ctx), id)
	if err != nil {
		return err
	}
	if sr.OrganizationID != orgID {
		return apperr.NotFound("sponsorship_request")
	}

	if err := s.repo.CancelSponsorshipRequest(ctx, id, orgID); err != nil {
		return err
	}

	if sr.PurchaseID != nil && sr.CreditsUsed > 0 {
		if _, err := s.repo.ConsumeSponsorshipCredits(database.AsSystem(ctx), ConsumeCredits{
			OrganizationID: sr.OrganizationID,
			PurchaseID:     *sr.PurchaseID,
			Credits:        sr.CreditsUsed,
			Refund:         true,
			Reason:         CreditSponsorshipRejected,
			EntityType:     string(sr.ItemType),
			EntityID:       &sr.ID,
			Note:           i18n.TDefault("promo.credits.note.request_cancelled"),
		}); err != nil {
			// The request is already cancelled; a failed refund is a balance
			// discrepancy someone has to see, not a silent loss.
			s.log.ErrorContext(ctx, "sponsorship cancel: credit refund failed",
				"error", err, "request_id", id, "purchase_id", *sr.PurchaseID)
		}
	}
	return nil
}

// RankedSponsorshipsForProducts returns sponsored rankings for product IDs.
func (s *Service) RankedSponsorshipsForProducts(ctx context.Context, productIDs []int64) ([]*RankedSponsorship, error) {
	return s.repo.RankedSponsorshipsForProducts(database.AsSystem(ctx), productIDs)
}

// RankedSponsorshipsForOffers returns sponsored rankings for offer IDs.
func (s *Service) RankedSponsorshipsForOffers(ctx context.Context, offerIDs []int64) ([]*RankedSponsorship, error) {
	return s.repo.RankedSponsorshipsForOffers(database.AsSystem(ctx), offerIDs)
}

// ListActiveRankedSponsorships returns all active, approved sponsorships for an item type,
// ordered by package tier level (highest first), breaking ties at the same tier randomly.
func (s *Service) ListActiveRankedSponsorships(ctx context.Context, itemType SponsorshipItemType) ([]*RankedSponsorship, error) {
	return s.repo.ListActiveRankedSponsorships(database.AsSystem(ctx), itemType)
}

// IsSponsored returns the highest-tier sponsorship for a single item.
func (s *Service) IsSponsored(ctx context.Context, itemType SponsorshipItemType, itemID int64) (*RankedSponsorship, error) {
	return s.repo.IsSponsored(database.AsSystem(ctx), itemType, itemID)
}

// ExpireSponsorships runs the expiry sweep for purchases and requests.
func (s *Service) ExpireSponsorships(ctx context.Context) (int64, error) {
	n1, err := s.repo.ExpireSponsorshipPurchases(ctx)
	if err != nil {
		return 0, err
	}
	n2, err := s.repo.ExpireSponsorshipRequests(ctx)
	if err != nil {
		return 0, err
	}
	return n1 + n2, nil
}

// AdminListSponsorshipRequestsWithTotal returns all requests for admin moderation with total count.
func (s *Service) AdminListSponsorshipRequestsWithTotal(ctx context.Context, limit, offset int) ([]*SponsorshipRequest, int, error) {
	return s.repo.ListAllSponsorshipRequestsWithTotal(database.AsSystem(ctx), limit, offset)
}

// ListSponsorshipRequestsByOrgWithTotal returns the vendor's sponsorship requests with total count.
func (s *Service) ListSponsorshipRequestsByOrgWithTotal(ctx context.Context, limit, offset int) ([]*SponsorshipRequest, int, error) {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return nil, 0, database.ErrNoTenant
	}
	return s.repo.ListSponsorshipRequestsByOrgWithTotal(ctx, orgID, limit, offset)
}
