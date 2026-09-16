package promo

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func TestPromoServiceLifecycle(t *testing.T) {
	ctx := database.WithTenant(context.Background(), 42)
	repo := newMockPromoRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(repo, logger)

	// 1. Create Offer
	discVal, _ := money.Parse("10.00")
	minOrder, _ := money.Parse("100.00")
	now := time.Now().UTC()

	o := &Offer{
		Title:          i18n.New("خصم الصيف", "Summer Discount"),
		DiscountType:   DiscountPercentage,
		DiscountValue:  discVal,
		MinOrderAmount: minOrder,
		StartsAt:       now,
		ExpiresAt:      now.Add(7 * 24 * time.Hour),
		IsActive:       true,
	}

	createdOffer, err := svc.CreateOffer(ctx, o)
	if err != nil {
		t.Fatalf("CreateOffer failed: %v", err)
	}
	if createdOffer.ID <= 0 || createdOffer.OrganizationID != 42 {
		t.Errorf("Offer metadata wrong: ID=%d Org=%d", createdOffer.ID, createdOffer.OrganizationID)
	}

	// 2. Get Offer & Engagement
	gotOffer, err := svc.GetOffer(ctx, createdOffer.ID)
	if err != nil || gotOffer.ID != createdOffer.ID {
		t.Fatalf("GetOffer failed: %v", err)
	}

	if err := svc.RecordOfferView(ctx, createdOffer.ID); err != nil {
		t.Fatalf("RecordOfferView failed: %v", err)
	}
	if err := svc.RecordOfferClick(ctx, createdOffer.ID); err != nil {
		t.Fatalf("RecordOfferClick failed: %v", err)
	}

	// 3. Calculate Discount
	subtotal, _ := money.Parse("200.00")
	disc, err := createdOffer.CalculateDiscount(subtotal)
	if err != nil {
		t.Fatalf("CalculateDiscount failed: %v", err)
	}
	if disc.IsZero() {
		t.Errorf("expected positive discount, got %v", disc)
	}

	// 4. Packages & Sponsorship
	pkgPrice, _ := money.Parse("50.00")
	pkg := &OfferPackage{
		Name:         i18n.New("باقة برونزية", "Bronze Tier"),
		Price:        pkgPrice,
		DurationDays: 14,
		MaxOffers:    3,
		IsActive:     true,
	}
	createdPkg, err := svc.CreatePackage(ctx, pkg)
	if err != nil {
		t.Fatalf("CreatePackage failed: %v", err)
	}

	pkgs, err := svc.ListPackages(ctx)
	if err != nil || len(pkgs) != 1 {
		t.Fatalf("ListPackages failed: %v", err)
	}

	spons, err := svc.SponsorOffer(ctx, createdOffer.ID, createdPkg.ID, 30)
	if err != nil {
		t.Fatalf("SponsorOffer failed: %v", err)
	}
	if spons.OfferID != createdOffer.ID {
		t.Errorf("got spons offer id %d, want %d", spons.OfferID, createdOffer.ID)
	}

	// 5. Active Offers and Ads
	activeOffers, err := svc.ListActiveOffers(ctx, 10, 0)
	if err != nil || len(activeOffers) != 1 {
		t.Fatalf("ListActiveOffers failed: %v", err)
	}

	repo.ads[1] = &Ad{
		ID:       1,
		Title:    "Top Banner",
		Position: "homepage_top",
		IsActive: true,
	}
	ads, err := svc.ListActiveAds(ctx, "homepage_top")
	if err != nil || len(ads) != 1 {
		t.Fatalf("ListActiveAds failed: %v", err)
	}
	if err := svc.RecordAdClick(ctx, 1, nil, "127.0.0.1", "agent"); err != nil {
		t.Fatalf("RecordAdClick failed: %v", err)
	}

	// 6. Highlight sections & Expire
	h := &HighlightSection{
		Title: i18n.New("عروض مميزة", "Featured Deals"),
		Slug:  "featured-deals",
	}
	createdSec, err := svc.CreateHighlightSection(ctx, h)
	if err != nil || createdSec.ID <= 0 {
		t.Fatalf("CreateHighlightSection failed: %v", err)
	}
	secs, err := svc.ListHighlightSections(ctx)
	if err != nil || len(secs) != 1 {
		t.Fatalf("ListHighlightSections failed: %v", err)
	}

	// 6b. Organization-owned highlight sections (066)
	orgSec, err := svc.CreateOrganizationHighlightSection(ctx, 7, i18n.New("الأكثر مبيعاً", "Best sellers"), "best")
	if err != nil || orgSec.ID <= 0 {
		t.Fatalf("CreateOrganizationHighlightSection failed: %v", err)
	}
	if orgSec.OwnerType != "organization" || orgSec.OrganizationID == nil || *orgSec.OrganizationID != 7 {
		t.Errorf("org section owner mismatch: %+v", orgSec)
	}
	if _, err := svc.CreateOrganizationHighlightSection(ctx, 7, i18n.Text{}, ""); err == nil {
		t.Error("empty-title org section should be rejected")
	}
	orgSecs, err := svc.ListHighlightSectionsByOrg(ctx, 7)
	if err != nil || len(orgSecs) != 1 {
		t.Fatalf("ListHighlightSectionsByOrg failed: %v", err)
	}
	pid := int64(42)
	if err := svc.AddHighlightItem(ctx, orgSec.ID, &pid, nil); err != nil {
		t.Fatalf("AddHighlightItem failed: %v", err)
	}
	items, err := svc.ListHighlightItems(ctx, orgSec.ID)
	if err != nil || len(items) != 0 {
		t.Fatalf("ListHighlightItems failed: %v", err)
	}

	_, err = svc.ExpirePromotions(ctx)
	if err != nil {
		t.Fatalf("ExpirePromotions failed: %v", err)
	}
}

func TestPurchaseSponsorshipPackage_WalletDebit(t *testing.T) {
	repo := newMockPromoRepo()
	pkg := &OfferPackage{
		ID:           1,
		Name:         i18n.New("الباقة الذهبية", "Gold Package"),
		Price:        money.FromMinor(150000),
		Credits:      10,
		DurationDays: 30,
		IsActive:     true,
	}
	repo.packages[1] = pkg

	var savedPurchase *SponsorshipPurchase
	repo.CreateSponsorshipPurchaseFunc = func(_ context.Context, p *SponsorshipPurchase) error {
		savedPurchase = p
		p.ID = 100
		return nil
	}

	svc := NewService(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))

	var debitedOrgID int64
	var debitedAmount money.Amount
	fakeTxID := int64(777)
	svc.SetWalletDebiter(func(ctx context.Context, orgID int64, amount money.Amount, description string) (*int64, error) {
		debitedOrgID = orgID
		debitedAmount = amount
		return &fakeTxID, nil
	})

	ctx := database.WithTenant(context.Background(), 55)
	purchase, err := svc.PurchaseSponsorshipPackage(ctx, 1, true, "monthly")
	if err != nil {
		t.Fatalf("PurchaseSponsorshipPackage failed: %v", err)
	}

	if debitedOrgID != 55 {
		t.Errorf("debitedOrgID = %d, want 55", debitedOrgID)
	}
	if debitedAmount.Minor() != 150000 {
		t.Errorf("debitedAmount = %d, want 150000", debitedAmount.Minor())
	}
	if savedPurchase == nil {
		t.Fatal("expected purchase to be saved")
	}
	if savedPurchase.PaymentID != nil {
		t.Errorf("savedPurchase.PaymentID = %v, want nil (must not violate foreign key)", *savedPurchase.PaymentID)
	}
	if savedPurchase.SourceID == nil || *savedPurchase.SourceID != 777 {
		t.Errorf("savedPurchase.SourceID = %v, want 777", savedPurchase.SourceID)
	}
	if savedPurchase.SourceSystem != "wallet_checkout" {
		t.Errorf("savedPurchase.SourceSystem = %q, want 'wallet_checkout'", savedPurchase.SourceSystem)
	}
	if purchase.CreditsRemaining != 10 {
		t.Errorf("purchase.CreditsRemaining = %d, want 10", purchase.CreditsRemaining)
	}
}

func TestExpirePromotionsWithMediaPurge(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("UPLOAD_DIR", tempDir)

	adsDir := filepath.Join(tempDir, "ads")
	if err := os.MkdirAll(adsDir, 0755); err != nil {
		t.Fatalf("failed to create ads dir: %v", err)
	}

	mainFile := filepath.Join(adsDir, "ad_exp_01.jpg")
	thumbFile := filepath.Join(adsDir, "ad_exp_01_thumb.jpg")
	cardFile := filepath.Join(adsDir, "ad_exp_01_card.jpg")
	otherFile := filepath.Join(adsDir, "ad_keep_02.jpg")

	for _, f := range []string{mainFile, thumbFile, cardFile, otherFile} {
		if err := os.WriteFile(f, []byte("test image data"), 0644); err != nil {
			t.Fatalf("failed to write test file %s: %v", f, err)
		}
	}

	repo := newMockPromoRepo()
	repo.ExpirePromotionsFunc = func(ctx context.Context) ([]string, int64, error) {
		return []string{"/uploads/ads/ad_exp_01.jpg"}, 1, nil
	}

	svc := NewService(repo, slog.Default())
	expCount, purgedCount, err := svc.ExpirePromotionsWithMediaPurge(context.Background(), nil)
	if err != nil {
		t.Fatalf("ExpirePromotionsWithMediaPurge failed: %v", err)
	}

	if expCount != 1 {
		t.Errorf("expCount = %d, want 1", expCount)
	}
	if purgedCount != 1 {
		t.Errorf("purgedCount = %d, want 1", purgedCount)
	}

	// Verify main file and derivative renditions are deleted
	for _, f := range []string{mainFile, thumbFile, cardFile} {
		if _, err := os.Stat(f); !os.IsNotExist(err) {
			t.Errorf("expected file %s to be deleted, but it still exists", f)
		}
	}

	// Verify other unrelated file is untouched
	if _, err := os.Stat(otherFile); err != nil {
		t.Errorf("expected unrelated file %s to be preserved, got error: %v", otherFile, err)
	}
}
