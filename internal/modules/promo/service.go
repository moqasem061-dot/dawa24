package promo

import (
	"context"
	"log/slog"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/storage"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// Service coordinates vendor promotions, sponsorships, and advertising campaigns.
type Service struct {
	repo Repository
	log  *slog.Logger

	reqDocs     RequiredDocsChecker
	walletDebit WalletDebiter
}

// RequiredDocsChecker is injected from composition root (Rebuild V2 §4.2): it
// must return an error when the organization cannot trade (missing mandatory
// documents). The promo module cannot import the attachments module, so the
// checker is a plain function set by cmd/server.
type RequiredDocsChecker func(ctx context.Context, orgID int64, orgType string) error

// SetRequiredDocsChecker installs the §4.2 documents gate. When set, offer
// creation refuses vendors with missing mandatory documents.
func (s *Service) SetRequiredDocsChecker(fn RequiredDocsChecker) {
	s.reqDocs = fn
}

// NewService creates a new promo service.
func NewService(repo Repository, log *slog.Logger) *Service {
	return &Service{
		repo: repo,
		log:  log,
	}
}

// CreateOffer registers a new promotional discount campaign under the tenant.
func (s *Service) CreateOffer(ctx context.Context, o *Offer) (*Offer, error) {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return nil, database.ErrNoTenant
	}
	if s.reqDocs != nil {
		if err := s.reqDocs(ctx, orgID, "vendor"); err != nil {
			return nil, err
		}
	}
	o.OrganizationID = orgID

	if err := o.Validate(); err != nil {
		return nil, err
	}
	o.IsActive = true

	if err := s.repo.CreateOffer(ctx, o); err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "offer created", "offer_id", o.ID, "org_id", o.OrganizationID)
	return o, nil
}

// GetOffer retrieves an offer by ID.
func (s *Service) GetOffer(ctx context.Context, id int64) (*Offer, error) {
	return s.repo.GetOfferByID(ctx, id)
}

// ListActiveOffers lists all running promotions across the marketplace.
func (s *Service) ListActiveOffers(ctx context.Context, limit, offset int) ([]*Offer, error) {
	return s.repo.ListActiveOffers(ctx, limit, offset)
}

// ListOffersForProduct lists the approved, running offers selling a product.
func (s *Service) ListOffersForProduct(ctx context.Context, productID int64) ([]*OfferProductWithOffer, error) {
	return s.repo.ListOffersForProduct(ctx, productID)
}

// ListOffersForProducts lists the approved, running offers selling any of the
// given products in one query, grouped by product ID.
func (s *Service) ListOffersForProducts(ctx context.Context, productIDs []int64) (map[int64][]*OfferProductWithOffer, error) {
	out := make(map[int64][]*OfferProductWithOffer)
	if len(productIDs) == 0 {
		return out, nil
	}
	rows, err := s.repo.ListOffersForProducts(ctx, productIDs)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if row == nil || row.Product == nil || row.Product.ProductID <= 0 {
			continue
		}
		out[row.Product.ProductID] = append(out[row.Product.ProductID], row)
	}
	return out, nil
}

// ListOffers returns all offers for admin moderation.
func (s *Service) ListOffers(ctx context.Context, limit, offset int) ([]*Offer, error) {
	return s.repo.ListOffers(ctx, limit, offset)
}

// SetOfferActive activates or deactivates an offer.
func (s *Service) SetOfferActive(ctx context.Context, id int64, active bool) error {
	return s.repo.SetOfferActive(ctx, id, active)
}

// RecordOfferView tracks an impression for an offer.
func (s *Service) RecordOfferView(ctx context.Context, offerID int64) error {
	return s.repo.IncrementOfferEngagement(ctx, offerID, false)
}

// RecordOfferClick tracks a click event on an offer.
func (s *Service) RecordOfferClick(ctx context.Context, offerID int64) error {
	return s.repo.IncrementOfferEngagement(ctx, offerID, true)
}

// ListPackages returns all available sponsorship packages.
func (s *Service) ListPackages(ctx context.Context) ([]*OfferPackage, error) {
	return s.repo.ListPackages(ctx)
}

// CreatePackage registers a new sponsorship package plan.
func (s *Service) CreatePackage(ctx context.Context, p *OfferPackage) (*OfferPackage, error) {
	if p.Name.IsEmpty() {
		return nil, apperr.Validation("package.name_required", "Package name is required.", nil)
	}
	p.IsActive = true
	if err := s.repo.CreatePackage(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// SponsorOffer sponsors an offer using a package tier.
func (s *Service) SponsorOffer(ctx context.Context, offerID int64, packageID int64, durationDays int) (*OfferSponsorship, error) {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return nil, database.ErrNoTenant
	}

	now := time.Now().UTC()
	if durationDays <= 0 {
		durationDays = 30
	}

	sponsorship := &OfferSponsorship{
		OrganizationID: orgID,
		OfferID:        offerID,
		PackageID:      packageID,
		StartsAt:       now,
		ExpiresAt:      now.Add(time.Duration(durationDays) * 24 * time.Hour),
		Status:         "active",
	}

	if err := s.repo.CreateSponsorship(ctx, sponsorship); err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "offer sponsored", "offer_id", offerID, "package_id", packageID)
	return sponsorship, nil
}

// ListActiveAds returns display banners for a designated position.
func (s *Service) ListActiveAds(ctx context.Context, position string) ([]*Ad, error) {
	return s.repo.ListActiveAds(ctx, position)
}

// RecordAdClick logs ad click analytics.
func (s *Service) RecordAdClick(ctx context.Context, adID int64, userID *int64, ip, ua string) error {
	if adID <= 0 {
		return apperr.Validation("ad_id.invalid", "Invalid ad ID", nil)
	}
	return s.repo.RecordAdClick(ctx, adID, userID, ip, ua)
}

// CreateHighlightSection creates a homepage curated highlight section.
func (s *Service) CreateHighlightSection(ctx context.Context, h *HighlightSection) (*HighlightSection, error) {
	if h.Title.IsEmpty() || h.Slug == "" {
		return nil, apperr.Validation("section.invalid", "Title and slug are required.", nil)
	}
	if h.OwnerType == "" {
		h.OwnerType = "platform"
	}
	if err := s.repo.CreateHighlightSection(ctx, h); err != nil {
		return nil, err
	}
	return h, nil
}

// ListHighlightSections returns all active curated sections.
func (s *Service) ListHighlightSections(ctx context.Context) ([]*HighlightSection, error) {
	return s.repo.ListHighlightSections(ctx)
}

// CreateOrganizationHighlightSection adds an organization-owned merchandising/featured section.
func (s *Service) CreateOrganizationHighlightSection(ctx context.Context, orgID int64, title i18n.Text, slug string) (*HighlightSection, error) {
	return s.CreateFeaturedSection(ctx, orgID, title, i18n.Text{}, "about", "#0284c7", slug, 0, true, true)
}

// CreateFeaturedSection creates an informational or branding section for the supplier.
func (s *Service) CreateFeaturedSection(ctx context.Context, orgID int64, title, description i18n.Text, sectionType, color, slug string, order int, active, showInHeader bool) (*HighlightSection, error) {
	if title.IsEmpty() {
		return nil, apperr.Validation("highlight.title_required", i18n.TDefault("w4_mod.w4str_245_245"), nil)
	}
	if sectionType == "" {
		sectionType = "about"
	}
	if color == "" {
		color = "#0284c7"
	}
	sec := &HighlightSection{
		OwnerType:      "organization",
		OrganizationID: &orgID,
		Title:          title,
		Description:    description,
		SectionType:    sectionType,
		Color:          color,
		Slug:           slug,
		DisplayOrder:   order,
		IsActive:       active,
		ShowInHeader:   showInHeader,
	}
	if err := s.repo.CreateHighlightSection(ctx, sec); err != nil {
		return nil, err
	}
	return sec, nil
}

// UpdateFeaturedSection updates an existing featured section.
func (s *Service) UpdateFeaturedSection(ctx context.Context, sec *HighlightSection) error {
	if sec == nil || sec.ID <= 0 {
		return apperr.Validation("highlight.id_required", i18n.TDefault("w4_mod.w4str_246_246"), nil)
	}
	if sec.Title.IsEmpty() {
		return apperr.Validation("highlight.title_required", i18n.TDefault("w4_mod.w4str_245_245"), nil)
	}
	return s.repo.UpdateHighlightSection(ctx, sec)
}

// DeleteFeaturedSection removes a supplier featured section.
func (s *Service) DeleteFeaturedSection(ctx context.Context, id, orgID int64) error {
	return s.repo.DeleteHighlightSection(ctx, id, orgID)
}

// GetFeaturedSection retrieves a featured section by ID.
func (s *Service) GetFeaturedSection(ctx context.Context, id int64) (*HighlightSection, error) {
	return s.repo.GetHighlightSectionByID(ctx, id)
}

// ListHighlightSectionsByOrg returns an organization's merchandising rows.
func (s *Service) ListHighlightSectionsByOrg(ctx context.Context, orgID int64) ([]*HighlightSection, error) {
	return s.repo.ListHighlightSectionsByOrg(ctx, orgID)
}

// AddHighlightItem adds a product or offer to a highlight section.
func (s *Service) AddHighlightItem(ctx context.Context, sectionID int64, productID, offerID *int64) error {
	item := &HighlightSectionItem{SectionID: sectionID, ProductID: productID, OfferID: offerID}
	return s.repo.AddHighlightItem(ctx, item)
}

// ListHighlightItems returns a section's items.
func (s *Service) ListHighlightItems(ctx context.Context, sectionID int64) ([]*HighlightSectionItem, error) {
	return s.repo.ListHighlightItems(ctx, sectionID)
}

// ExpirePromotionsWithMediaPurge expires promotions, sponsorship requests, and ads, and purges all associated media.
func (s *Service) ExpirePromotionsWithMediaPurge(ctx context.Context, s3Client *storage.Client) (int64, int, error) {
	mediaURLs, expiredCount, err := s.repo.ExpirePromotionsAndCollectMedia(ctx)
	if err != nil {
		return 0, 0, err
	}
	purged := 0
	for _, u := range mediaURLs {
		if err := storage.DeleteUploadedMedia(ctx, u, s3Client); err == nil {
			purged++
		}
	}
	if expiredCount > 0 || purged > 0 {
		s.log.InfoContext(ctx, "expired promotions and purged media",
			"expired_count", expiredCount, "purged_media_count", purged)
	}
	return expiredCount, purged, nil
}

// ExpirePromotions runs the automated expiry sweeps with local media purge.
func (s *Service) ExpirePromotions(ctx context.Context) (int64, error) {
	expiredCount, _, err := s.ExpirePromotionsWithMediaPurge(ctx, nil)
	return expiredCount, err
}

// CreateSpecialOffer creates a special offer under tenant organization.
func (s *Service) CreateSpecialOffer(ctx context.Context, o *SpecialOffer) (*SpecialOffer, error) {
	if o.OrganizationID <= 0 {
		orgID, ok := database.TenantFrom(ctx)
		if !ok {
			return nil, database.ErrNoTenant
		}
		o.OrganizationID = orgID
	}
	if err := s.repo.CreateSpecialOffer(ctx, o); err != nil {
		return nil, err
	}
	return o, nil
}

// UpdateSpecialOffer updates a vendor's special offer.
func (s *Service) UpdateSpecialOffer(ctx context.Context, o *SpecialOffer) error {
	if o.OrganizationID <= 0 {
		orgID, ok := database.TenantFrom(ctx)
		if !ok {
			return database.ErrNoTenant
		}
		o.OrganizationID = orgID
	}
	return s.repo.UpdateSpecialOffer(ctx, o)
}

// GetSpecialOffer retrieves a special offer by ID with details.
func (s *Service) GetSpecialOffer(ctx context.Context, id int64) (*SpecialOffer, error) {
	return s.repo.GetSpecialOfferByID(ctx, id)
}

// ListSpecialOffersByOrg lists all special offers for an organization.
func (s *Service) ListSpecialOffersByOrg(ctx context.Context, orgID int64) ([]*SpecialOffer, error) {
	return s.repo.ListSpecialOffersByOrg(ctx, orgID)
}

// DeleteSpecialOffer deletes a special offer.
func (s *Service) DeleteSpecialOffer(ctx context.Context, id, orgID int64) error {
	return s.repo.DeleteSpecialOffer(ctx, id, orgID)
}

// AddSpecialOfferLocation adds a geographic coverage record.
func (s *Service) AddSpecialOfferLocation(ctx context.Context, loc *SpecialOfferLocation) error {
	return s.repo.AddSpecialOfferLocation(ctx, loc)
}

// ListSpecialOfferLocations lists coverage locations for an offer.
func (s *Service) ListSpecialOfferLocations(ctx context.Context, offerID int64) ([]*SpecialOfferLocation, error) {
	return s.repo.ListSpecialOfferLocations(ctx, offerID)
}

// DeleteSpecialOfferLocation removes a geographic coverage record.
func (s *Service) DeleteSpecialOfferLocation(ctx context.Context, id, offerID, orgID int64) error {
	return s.repo.DeleteSpecialOfferLocation(ctx, id, offerID, orgID)
}

// ListAllSpecialOffers lists all special offers across the platform for admin review.
func (s *Service) ListAllSpecialOffers(ctx context.Context, limit, offset int) ([]*SpecialOffer, error) {
	return s.repo.ListAllSpecialOffers(ctx, limit, offset)
}

// ListAllSpecialOffersWithTotal lists paginated special offers across the platform with status filter.
func (s *Service) ListAllSpecialOffersWithTotal(ctx context.Context, statusFilter string, limit, offset int) ([]*SpecialOffer, int, error) {
	return s.repo.ListAllSpecialOffersWithTotal(ctx, statusFilter, limit, offset)
}

// UpdateSpecialOfferAdminStatus handles approval/rejection moderation by platform staff.
func (s *Service) UpdateSpecialOfferAdminStatus(ctx context.Context, id int64, adminStatus, notes string, approvedBy int64) error {
	return s.repo.UpdateSpecialOfferAdminStatus(ctx, id, adminStatus, notes, approvedBy)
}

// ToggleSpecialOfferStatus activates or deactivates a special offer.
func (s *Service) ToggleSpecialOfferStatus(ctx context.Context, id int64, isActive bool) error {
	return s.repo.ToggleSpecialOfferStatus(ctx, id, isActive)
}

// ListAdminOfferLocations retrieves geographic coverage records and stats for admin review.
func (s *Service) ListAdminOfferLocations(ctx context.Context, filter OfferLocationsFilter) ([]*OfferLocationAdminRow, OfferLocationsStats, int, error) {
	return s.repo.ListAdminOfferLocations(ctx, filter)
}
