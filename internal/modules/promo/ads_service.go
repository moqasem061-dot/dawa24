package promo

import (
	"context"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// CreateAd creates a new advertisement for the vendor's organization.
// It deducts AdCreditCost (2 credits) from an active sponsorship purchase.
// The ad starts in pending admin_status and must be approved before display.
func (s *Service) CreateAd(ctx context.Context, a *Ad) (*Ad, error) {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return nil, database.ErrNoTenant
	}
	a.OrganizationID = &orgID
	a.AdminStatus = AdminPending
	a.IsActive = false
	if a.Title == "" && a.TitleAr != "" {
		a.Title = a.TitleAr
	}
	if a.Title == "" && a.TitleEn != "" {
		a.Title = a.TitleEn
	}
	if a.Title == "" {
		return nil, apperr.Validation("ad.title_required", "عنوان الإعلان مطلوب.", nil)
	}
	if a.MediaURL == "" {
		return nil, apperr.Validation("ad.media_required", "ملف أو رابط وسائط الإعلان مطلوب.", nil)
	}

	// 1. Verify and deduct 2 sponsorship credits
	purchases, err := s.repo.ListActiveSponsorshipPurchasesByOrg(ctx, orgID)
	if err != nil {
		return nil, err
	}
	var selectedPurchase *SponsorshipPurchase
	for _, p := range purchases {
		if p != nil && p.CreditsRemainingInt() >= AdCreditCost {
			selectedPurchase = p
			break
		}
	}
	if selectedPurchase == nil {
		return nil, apperr.Conflict("ad.insufficient_credits", "رصيد الرعاية غير كافٍ. يتطلب إنشاء الإعلان 2 رصيد رعاية على الأقل.")
	}

	if _, err := s.repo.ConsumeSponsorshipCredits(ctx, ConsumeCredits{
		OrganizationID: orgID,
		PurchaseID:     selectedPurchase.ID,
		Credits:        AdCreditCost,
		Reason:         CreditAdCreated,
		EntityType:     "ad",
	}); err != nil {
		return nil, err
	}
	a.AdPlanID = nil

	if a.DurationDays <= 0 {
		a.DurationDays = 30
	}
	if a.ExpiresAt.IsZero() {
		a.ExpiresAt = time.Now().UTC().Add(time.Duration(a.DurationDays) * 24 * time.Hour)
	}
	if a.StartsAt.IsZero() {
		a.StartsAt = time.Now().UTC()
	}
	if err := s.repo.CreateAd(ctx, a); err != nil {
		// The credit was reserved before the ad was written, so a failed
		// write has to give it back. A refund that itself fails leaves the
		// vendor short by two credits with nothing to show for it, so it is
		// logged rather than discarded.
		if _, refundErr := s.repo.ConsumeSponsorshipCredits(ctx, ConsumeCredits{
			OrganizationID: orgID,
			PurchaseID:     selectedPurchase.ID,
			Credits:        AdCreditCost,
			Refund:         true,
			Reason:         CreditAdRefunded,
			EntityType:     "ad",
			Note:           i18n.TDefault("promo.credits.note.ad_failed"),
		}); refundErr != nil {
			s.log.ErrorContext(ctx, "ad create: credit refund failed",
				"error", refundErr, "purchase_id", selectedPurchase.ID)
		}
		return nil, err
	}
	s.log.InfoContext(ctx, "ad created with 2 credits deducted", "ad_id", a.ID, "org_id", orgID, "position", a.Position, "purchase_id", selectedPurchase.ID)
	return a, nil
}

// UpdateAd updates an existing ad owned by the vendor's organization.
func (s *Service) UpdateAd(ctx context.Context, a *Ad) error {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return database.ErrNoTenant
	}
	existing, err := s.repo.GetAdByID(ctx, a.ID)
	if err != nil {
		return err
	}
	if existing.OrganizationID == nil || *existing.OrganizationID != orgID {
		return apperr.NotFound("ad")
	}
	if a.Title == "" && a.TitleAr != "" {
		a.Title = a.TitleAr
	}
	if a.Title == "" && a.TitleEn != "" {
		a.Title = a.TitleEn
	}
	if a.Title == "" {
		return apperr.Validation("ad.title_required", i18n.TDefault("w4_mod.w4str_242_242"), nil)
	}
	return s.repo.UpdateAd(ctx, a)
}

// AdminToggleAd allows platform staff to toggle an ad's active state without tenant constraints.
func (s *Service) AdminToggleAd(ctx context.Context, id int64) (*Ad, error) {
	return s.repo.AdminToggleAd(ctx, id)
}

// GetAd retrieves an ad by ID.
func (s *Service) GetAd(ctx context.Context, id int64) (*Ad, error) {
	return s.repo.GetAdByID(ctx, id)
}

// ListAdsByOrg returns ads for the active tenant.
func (s *Service) ListAdsByOrg(ctx context.Context, limit, offset int) ([]*Ad, error) {
	ads, _, err := s.ListAdsByOrgWithTotal(ctx, limit, offset)
	return ads, err
}

// ListAdsByOrgWithTotal returns paginated ads for the active tenant with total count.
func (s *Service) ListAdsByOrgWithTotal(ctx context.Context, limit, offset int) ([]*Ad, int, error) {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return nil, 0, database.ErrNoTenant
	}
	ads, total, err := s.repo.ListAdsByOrgWithTotal(ctx, orgID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	for _, a := range ads {
		if a != nil {
			a.CTR = ComputeCTR(a.Impressions, a.Clicks)
		}
	}
	return ads, total, nil
}

// AdminListAds returns all ads for admin moderation.
func (s *Service) AdminListAds(ctx context.Context, limit, offset int) ([]*Ad, error) {
	ads, _, err := s.AdminListAdsWithTotal(ctx, limit, offset)
	return ads, err
}

// AdminListAdsWithTotal returns all ads for admin moderation with total count.
func (s *Service) AdminListAdsWithTotal(ctx context.Context, limit, offset int) ([]*Ad, int, error) {
	ads, total, err := s.repo.ListAllAdsWithTotal(database.AsSystem(ctx), limit, offset)
	if err != nil {
		return nil, 0, err
	}
	for _, a := range ads {
		if a != nil {
			a.CTR = ComputeCTR(a.Impressions, a.Clicks)
		}
	}
	return ads, total, nil
}

// AdminApproveAd approves an ad for display.
func (s *Service) AdminApproveAd(ctx context.Context, id int64, notes string) error {
	reviewerID, _ := authctx.UserID(ctx)
	return s.repo.UpdateAdAdminStatus(database.AsSystem(ctx), id, AdminApproved, notes, reviewerID)
}

// AdminRejectAd rejects an ad, keeping it invisible.
func (s *Service) AdminRejectAd(ctx context.Context, id int64, notes string) error {
	reviewerID, _ := authctx.UserID(ctx)
	return s.repo.UpdateAdAdminStatus(database.AsSystem(ctx), id, AdminRejected, notes, reviewerID)
}

// SubmitAdEditRequest stores vendor-proposed ad edits for admin review without taking down the live ad.
func (s *Service) SubmitAdEditRequest(ctx context.Context, id int64, changes *AdPendingChanges) error {
	if id <= 0 {
		return apperr.Validation("ad_id.invalid", "Invalid ad ID", nil)
	}
	if changes == nil {
		return apperr.Validation("changes.required", "Changes payload is required", nil)
	}
	changes.SubmittedAt = time.Now()
	return s.repo.SubmitAdEditRequest(database.AsSystem(ctx), id, changes)
}

// ApproveAdEditRequest applies vendor-proposed changes to the live ad and clears pending changes.
func (s *Service) ApproveAdEditRequest(ctx context.Context, id int64) error {
	if id <= 0 {
		return apperr.Validation("ad_id.invalid", "Invalid ad ID", nil)
	}
	reviewerID, _ := authctx.UserID(ctx)
	return s.repo.ApproveAdEditRequest(database.AsSystem(ctx), id, reviewerID)
}

// RejectAdEditRequest rejects vendor-proposed changes and keeps the live ad intact.
func (s *Service) RejectAdEditRequest(ctx context.Context, id int64, notes string) error {
	if id <= 0 {
		return apperr.Validation("ad_id.invalid", "Invalid ad ID", nil)
	}
	reviewerID, _ := authctx.UserID(ctx)
	return s.repo.RejectAdEditRequest(database.AsSystem(ctx), id, reviewerID, notes)
}

// RecordAdImpression logs an ad view.
func (s *Service) RecordAdImpression(ctx context.Context, adID int64, userID *int64, ip, ua string) error {
	if adID <= 0 {
		return apperr.Validation("ad_id.invalid", "Invalid ad ID", nil)
	}
	return s.repo.RecordAdImpression(database.AsSystem(ctx), adID, userID, ip, ua)
}

// AdminListPackages returns all sponsorship packages for admin management.
func (s *Service) AdminListPackages(ctx context.Context) ([]*OfferPackage, error) {
	return s.repo.AdminListPackages(database.AsSystem(ctx))
}

// AdminCreatePackage creates a sponsorship package (admin).
func (s *Service) AdminCreatePackage(ctx context.Context, p *OfferPackage) (*OfferPackage, error) {
	if p.Name.IsEmpty() {
		return nil, apperr.Validation("package.name_required", "Package name is required.", nil)
	}
	p.IsActive = true
	if err := s.repo.CreatePackage(database.AsSystem(ctx), p); err != nil {
		return nil, err
	}
	return p, nil
}

// AdminUpdatePackage updates a sponsorship package (admin).
func (s *Service) AdminUpdatePackage(ctx context.Context, p *OfferPackage) (*OfferPackage, error) {
	if p.ID <= 0 || p.Name.IsEmpty() {
		return nil, apperr.Validation("package.invalid", "Package ID and name are required.", nil)
	}
	if err := s.repo.UpdatePackage(database.AsSystem(ctx), p); err != nil {
		return nil, err
	}
	return p, nil
}

// AdminTogglePackageActive toggles a package's active state (admin).
func (s *Service) AdminTogglePackageActive(ctx context.Context, id int64, active bool) error {
	return s.repo.TogglePackageActive(database.AsSystem(ctx), id, active)
}

// AdminGetPackageByID retrieves a single package by ID for admin.
func (s *Service) AdminGetPackageByID(ctx context.Context, id int64) (*OfferPackage, error) {
	return s.repo.GetPackageByID(database.AsSystem(ctx), id)
}
