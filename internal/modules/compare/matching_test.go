package compare_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/compare"
)

func TestMatchLadder_AllStrategies(t *testing.T) {
	ctx := context.Background()
	repo := newMockCompareRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := compare.NewService(repo, logger)

	candidates := []*compare.CandidateProduct{
		{
			ID:                     10,
			SKU:                    "SKU-PANADOL-EXTRA",
			NameAr:                 "بنادول اكسترا 24 قرص",
			NameEn:                 "Panadol Extra 24 Tablets",
			ScientificName:         "Paracetamol 500mg + Caffeine 65mg",
			ManufacturingCompanies: "GSK",
			Pharmacology:           "Analgesic / Antipyretic",
		},
		{
			ID:                     20,
			SKU:                    "SKU-CATAFLAM-50",
			NameAr:                 "كتافلام 50 مجم 20 قرص",
			NameEn:                 "Cataflam 50mg 20 Tablets",
			ScientificName:         "Diclofenac Potassium",
			ManufacturingCompanies: "Novartis",
			Pharmacology:           "NSAID",
		},
		{
			ID:                     30,
			SKU:                    "SKU-AUGMENTIN-1G",
			NameAr:                 "اوجمنتين 1 جم 14 قرص",
			NameEn:                 "Augmentin 1g 14 Tablets",
			ScientificName:         "Amoxicillin + Clavulanate",
			ManufacturingCompanies: "GlaxoSmithKline",
			Pharmacology:           "Antibiotic",
		},
	}

	orgID := int64(100)

	// Strategy 0: Saved Customer Product Mapping
	_ = repo.SaveCustomerProductMapping(ctx, &orgID, "بنادول الاحمر اكسترا", 10, "manual")
	match, err := svc.MatchLadder(ctx, &orgID, "بنادول الاحمر اكسترا", "", "", candidates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if match.ProductID == nil || *match.ProductID != 10 {
		t.Errorf("expected Strategy 0 (Saved Mapping) to match product 10, got %v", match.ProductID)
	}
	if match.Method != compare.MatchMethodSavedMapping {
		t.Errorf("expected method saved_mapping, got %s", match.Method)
	}
	if match.Confidence != 100.0 {
		t.Errorf("expected 100%% confidence, got %f", match.Confidence)
	}

	// Strategy 1: SKU Match, seconded by the name.
	//
	// The code tier is not authoritative in this tool, and that is deliberate:
	// a compare upload is a SUPPLIER's price list, so its code column holds the
	// supplier's own numbering, which collides with a دوا 24 code by accident
	// more often than by design. productmatch therefore requires a unique
	// catalogue hit AND a name that agrees before a code settles anything.
	match, err = svc.MatchLadder(ctx, &orgID, "كتافلام 50 مجم", "SKU-CATAFLAM-50", "", candidates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if match.ProductID == nil || *match.ProductID != 20 {
		t.Errorf("expected Strategy 1 (SKU) to match product 20, got %v", match.ProductID)
	}
	if match.Method != compare.MatchMethodSKU {
		t.Errorf("expected method sku, got %s", match.Method)
	}

	// And the other half of that rule: a code that hits while the name says
	// something else entirely must NOT settle the row. This tool used to return
	// it at confidence 100, which is the failure mode identifiers.go exists to
	// prevent — a wrongly linked row prices the wrong medicine and nothing
	// downstream catches it.
	match, err = svc.MatchLadder(ctx, &orgID, "عنصر غير معروف الاسم", "SKU-CATAFLAM-50", "", candidates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if match.ProductID != nil {
		t.Errorf("a bare code collision must not settle a row; got product %v at %v%%",
			*match.ProductID, match.Confidence)
	}

	// Strategy 2: Exact Name Match
	match, err = svc.MatchLadder(ctx, &orgID, "Augmentin 1g 14 Tablets", "", "", candidates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if match.ProductID == nil || *match.ProductID != 30 {
		t.Errorf("expected Strategy 2 (Exact Name) to match product 30, got %v", match.ProductID)
	}
	if match.Method != compare.MatchMethodExactName {
		t.Errorf("expected method exact_name, got %s", match.Method)
	}

	// Strategy 3: Trigram / Fuzzy Match with Arabic Normalization (أ vs ا, ة vs ه, etc.)
	match, err = svc.MatchLadder(ctx, &orgID, "أوجمنتين ١ جم ١٤ قرص", "", "", candidates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if match.ProductID == nil || *match.ProductID != 30 {
		t.Errorf("expected Strategy 3 (Fuzzy) to match product 30, got %v", match.ProductID)
	}
	if match.Confidence < 90.0 {
		t.Errorf("expected high confidence >= 90 for normalized match, got %f", match.Confidence)
	}

	// A terser line still resolves: the brand and the strength agree and nothing
	// contradicts, so the missing pack count is a detail rather than a
	// difference.
	match, err = svc.MatchLadder(ctx, &orgID, "كتافلام 50 مجم", "", "", candidates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if match.ProductID == nil || *match.ProductID != 20 {
		t.Errorf("expected the terse Cataflam line to match product 20, got %v", match.ProductID)
	}

	// 🚦 Sharing a brand is not being the same product.
	//
	// "كتافلام شراب أطفال" is a paediatric syrup and product 20 is the
	// fifty-milligram tablet. They share the brand and nothing else, and the
	// ladder used to join them on exactly that — its "first meaningful word"
	// strategy matched on the brand alone at 55%, with no form check and no
	// strength check.
	//
	// This is the class of wrong match the shared engine exists to refuse: a
	// syrup is not a tablet, and a price comparison built on that tells a
	// pharmacy the wrong medicine is cheaper elsewhere.
	match, err = svc.MatchLadder(ctx, &orgID, "كتافلام شراب أطفال غير مسجل بالجرام", "", "", candidates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if match.ProductID != nil {
		t.Errorf("a paediatric syrup was matched to the 50mg tablet of the same brand: %d",
			*match.ProductID)
	}

	// Strategy 5: Unmatched (< 55%)
	match, err = svc.MatchLadder(ctx, &orgID, "منتج شوكولاتة وبسكويت أطفال", "", "", candidates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if match.ProductID != nil {
		t.Errorf("expected nil product for unmatched item, got %d", *match.ProductID)
	}
	if match.Method != compare.MatchMethodUnmatched {
		t.Errorf("expected method unmatched, got %s", match.Method)
	}
	if match.Confidence != 0.0 {
		t.Errorf("expected 0 confidence, got %f", match.Confidence)
	}
}

func TestManualCorrectionPersistenceAndReuse(t *testing.T) {
	ctx := context.Background()
	repo := newMockCompareRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := compare.NewService(repo, logger)

	candidates := []*compare.CandidateProduct{
		{
			ID:     55,
			SKU:    "SKU-CONCOR-5",
			NameAr: "كونكور 5 مجم 30 قرص",
			NameEn: "Concor 5mg 30 Tablets",
		},
	}

	orgID := int64(202)
	rowID := int64(1001)
	rawName := "كونكور خمسة بلس معدل"

	// 1. Initial match fails or is partial
	match, _ := svc.MatchLadder(ctx, &orgID, rawName, "", "", candidates)
	if match.Method == compare.MatchMethodSavedMapping {
		t.Errorf("expected no saved mapping before manual correction")
	}

	// 2. User manually corrects row to product 55
	err := svc.SaveManualCorrection(ctx, &orgID, rowID, rawName, 55)
	if err != nil {
		t.Fatalf("failed to save manual correction: %v", err)
	}

	// 3. Next time matching the exact same raw name, Strategy 0 picks it up automatically!
	nextMatch, err := svc.MatchLadder(ctx, &orgID, rawName, "", "", candidates)
	if err != nil {
		t.Fatalf("unexpected error on second match: %v", err)
	}
	if nextMatch.ProductID == nil || *nextMatch.ProductID != 55 {
		t.Errorf("expected reused manual mapping to match product 55, got %v", nextMatch.ProductID)
	}
	if nextMatch.Method != compare.MatchMethodSavedMapping {
		t.Errorf("expected method saved_mapping on reused correction, got %s", nextMatch.Method)
	}
	if nextMatch.Confidence != 100.0 {
		t.Errorf("expected 100%% confidence on reused correction, got %f", nextMatch.Confidence)
	}
}
