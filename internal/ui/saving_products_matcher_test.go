package ui

import (
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

func TestSavingProductMatchEngine(t *testing.T) {
	catalogItems := []catalog.MatchProduct{
		{
			ID: 101, SKU: "PAN-24", Barcode: "6221234567890",
			NameAR: "بانادول إكسترا 24 قرص", NameEN: "Panadol Extra 24 Tablets",
			DosageForm: "أقراص",
		},
		{
			ID: 102, SKU: "20203380", Barcode: "20203380",
			NameAR: "ليدي سبيد ستيك مزيل عرق 65 جرام", NameEN: "Lady Speed Stick Deodorant 65g",
		},
		{
			ID: 103, SKU: "AUG-1G", Barcode: "6229876543210",
			NameAR: "أوجمنتين 1 جم 14 قرص", NameEN: "Augmentin 1g 14 Tablets",
			DosageForm: "أقراص", Concentration: "1g",
		},
		{
			ID: 104, SKU: "FEM-250", Barcode: "5010724524221",
			NameAR: "فيم فريش غسول يومي للمناطق الحساسة 250 مل", NameEN: "Femfresh Daily Intimate Wash 250ml",
		},
	}

	// The engine starts with every identifier tier off, so the tests that
	// exercise a tier have to declare what the user mapped and chose — which is
	// the contract every caller now has.
	codeEngine := NewSavingProductMatchEngine(catalogItems)
	codeEngine.UseIdentifiers(
		productmatch.MappedColumns{Code: true},
		productmatch.IdentifierChoices{ByCode: true, CodeIsCatalogCode: true},
	)

	// A separate engine for the rows whose column really does hold GTINs. It
	// used to be the same one: a single column was fed to the code tier AND the
	// barcode tier, so an internal item number got a free pass through a tier
	// that never looks at the name.
	barcodeEngine := NewSavingProductMatchEngine(catalogItems)
	barcodeEngine.UseIdentifiers(
		productmatch.MappedColumns{Barcode: true},
		productmatch.IdentifierChoices{ByBarcode: true},
	)

	engine := NewSavingProductMatchEngine(catalogItems)

	t.Run("Exact SKU Match", func(t *testing.T) {
		res := codeEngine.Match(StrategySmartAuto, nil, "PAN-24", "اسم عشوائي مختلف")
		if res.ProductID == nil || *res.ProductID != 101 {
			t.Fatalf("expected product 101, got %v", res.ProductID)
		}
		if res.MatchType != "exact_sku" {
			t.Errorf("expected exact_sku, got %s", res.MatchType)
		}
	})

	t.Run("Cleaned Barcode with .0 and spaces", func(t *testing.T) {
		res := barcodeEngine.Match(StrategySmartAuto, nil, " 6221234567890.0 ", "")
		if res.ProductID == nil || *res.ProductID != 101 {
			t.Fatalf("expected product 101, got %v", res.ProductID)
		}
		if res.MatchType != "barcode" {
			t.Errorf("expected barcode, got %s", res.MatchType)
		}
	})

	t.Run("Scientific notation barcode expansion", func(t *testing.T) {
		sanitized := SanitizeSKUCode("2.020338e+07")
		if sanitized != "20203380" {
			t.Fatalf("expected 20203380, got %s", sanitized)
		}
		res := barcodeEngine.Match(StrategySmartAuto, nil, "2.020338e+07", "")
		if res.ProductID == nil || *res.ProductID != 102 {
			t.Fatalf("expected product 102, got %v", res.ProductID)
		}
	})

	t.Run("Normalized Arabic Name Match (Hamza variation)", func(t *testing.T) {
		// Catalog has "أوجمنتين 1 جم 14 قرص" (with Alif Hamza), input has "اوجمنتين 1 جم 14 قرص" (without Hamza)
		res := engine.Match(StrategySmartAuto, nil, "", "اوجمنتين 1 جم 14 قرص")
		if res.ProductID == nil || *res.ProductID != 103 {
			t.Fatalf("expected product 103, got %v", res.ProductID)
		}
		if res.MatchType != "exact_name" {
			t.Errorf("expected exact_name, got %s", res.MatchType)
		}
	})

	t.Run("Normalized Yaa / Alif Maqsura Match", func(t *testing.T) {
		// Catalog has "ليدي سبيد", input has "ليدى سبيد"
		res := engine.Match(StrategySmartAuto, nil, "", "ليدى سبيد ستيك مزيل عرق 65 جرام")
		if res.ProductID == nil || *res.ProductID != 102 {
			t.Fatalf("expected product 102, got %v", res.ProductID)
		}
	})

	t.Run("Core Drug Name Match without dosage suffix noise", func(t *testing.T) {
		// Input has "بانادول إكسترا" without "24 قرص"
		res := engine.Match(StrategySmartAuto, nil, "", "بانادول اكسترا")
		if res.ProductID == nil || *res.ProductID != 101 {
			t.Fatalf("expected product 101, got %v", res.ProductID)
		}
	})

	t.Run("Borderline description is held for review, not applied", func(t *testing.T) {
		// Input:   "فيم فريش غسول انتميت سكير 250 مل"
		// Catalog: "فيم فريش غسول يومي للمناطق الحساسة 250 مل"
		//
		// Three of six identifying words shared, no concentration and no dosage
		// form to corroborate either way. The shared scorer puts it at 0.41,
		// which is exactly the band the platform's 50% default exists to keep
		// out of a pharmacy's list without a human looking at it: plausible,
		// and not the same product often enough to apply silently.
		//
		// It used to be applied, because the default was 40%. The row is still
		// scored, still shown and still one click from being accepted.
		res := engine.MatchUnified(DefaultMatchChoice(), nil, "INTERNAL-1234", "فيم فريش غسول انتميت سكير 250 مل")
		if res.ProductID != nil {
			t.Fatalf("borderline row was applied at the default threshold: %v (%.4f)",
				*res.ProductID, res.Confidence)
		}

		// A pharmacy that wants the wider net can have it: the threshold is the
		// one control, and lowering it changes this row and nothing else.
		wide := DefaultMatchChoice()
		wide.MinScore = 0.35
		res = engine.MatchUnified(wide, nil, "INTERNAL-1234", "فيم فريش غسول انتميت سكير 250 مل")
		if res.ProductID == nil || *res.ProductID != 104 {
			t.Fatalf("expected product 104 at a 35%% threshold, got %v", res.ProductID)
		}
	})

	t.Run("Identifier tiers are off unless switched on", func(t *testing.T) {
		// The default choice consults the name and nothing else. A pharmacy's
		// internal item number in the code column must not settle a match.
		if res := engine.MatchUnified(DefaultMatchChoice(), nil, "PAN-24", "اسم عشوائي مختلف"); res.ProductID != nil {
			t.Fatalf("code tier ran without being switched on: %v", *res.ProductID)
		}
		byCode := DefaultMatchChoice()
		byCode.ByCode, byCode.CodeIsCatalogCode = true, true
		if res := engine.MatchUnified(byCode, nil, "PAN-24", "اسم عشوائي مختلف"); res.ProductID == nil || *res.ProductID != 101 {
			t.Fatalf("code tier did not run once switched on: %v", res.ProductID)
		}
		byBarcode := DefaultMatchChoice()
		byBarcode.ByBarcode = true
		if res := engine.MatchUnified(byBarcode, nil, "6221234567890", ""); res.ProductID == nil || *res.ProductID != 101 {
			t.Fatalf("barcode tier did not run once switched on: %v", res.ProductID)
		}
	})

	t.Run("Strategy: SKU Only mode", func(t *testing.T) {
		// Correct name but unknown SKU
		res := codeEngine.Match(StrategySKUOnly, nil, "UNKNOWN-SKU", "بانادول إكسترا 24 قرص")
		if res.ProductID != nil {
			t.Fatalf("expected no match in SKUOnly mode with unknown SKU, got %v", res.ProductID)
		}

		// Known SKU
		res2 := codeEngine.Match(StrategySKUOnly, nil, "PAN-24", "اسم مختلف تماماً")
		if res2.ProductID == nil || *res2.ProductID != 101 {
			t.Fatalf("expected product 101, got %v", res2.ProductID)
		}
	})

	t.Run("Strategy: Name Only mode (Internal SKU ignored)", func(t *testing.T) {
		// Internal pharmacy cashier SKU that does NOT match catalog, but name matches
		res := codeEngine.Match(StrategyNameOnly, nil, "MY-POS-998877", "بانادول اكسترا 24 قرص")
		if res.ProductID == nil || *res.ProductID != 101 {
			t.Fatalf("expected product 101 in NameOnly mode, got %v", res.ProductID)
		}
	})

	t.Run("Strategy: Default when empty is Name Only", func(t *testing.T) {
		res := codeEngine.Match("", nil, "MY-POS-998877", "بانادول اكسترا 24 قرص")
		if res.ProductID == nil || *res.ProductID != 101 {
			t.Fatalf("expected product 101 with empty strategy defaulting to NameOnly, got %v", res.ProductID)
		}
	})

	t.Run("Strategy: Barcode Only mode", func(t *testing.T) {
		res := barcodeEngine.Match(StrategyBarcodeOnly, nil, "6221234567890", "اسم مختلف غير مطابق")
		if res.ProductID == nil || *res.ProductID != 101 {
			t.Fatalf("expected product 101 in BarcodeOnly mode, got %v", res.ProductID)
		}
		// When barcode does not match, does not match by name
		resFail := barcodeEngine.Match(StrategyBarcodeOnly, nil, "0000000000000", "بانادول اكسترا 24 قرص")
		if resFail.ProductID != nil {
			t.Fatalf("expected no match for mismatched barcode in BarcodeOnly mode, got %v", resFail.ProductID)
		}
	})

	t.Run("Strategy: SKU and Barcode mode", func(t *testing.T) {
		res := codeEngine.Match(StrategySKUBarcode, nil, "PAN-24", "اسم مختلف تماماً")
		if res.ProductID == nil || *res.ProductID != 101 {
			t.Fatalf("expected product 101 in SKUBarcode mode, got %v", res.ProductID)
		}
	})

	t.Run("Strategy: Dawa24 ID Only mode", func(t *testing.T) {
		validID := int64(101)
		res := engine.Match(StrategyIDOnly, &validID, "", "")
		if res.ProductID == nil || *res.ProductID != 101 {
			t.Fatalf("expected product 101 in IDOnly mode, got %v", res.ProductID)
		}
		invalidID := int64(999999)
		resFail := engine.Match(StrategyIDOnly, &invalidID, "", "بانادول اكسترا 24 قرص")
		if resFail.ProductID != nil {
			t.Fatalf("expected no match in IDOnly mode for unknown ID, got %v", resFail.ProductID)
		}
	})

	// The default engine is the one a caller gets before saying anything about
	// the file. It must not settle a row on an identifier alone.
	t.Run("Identifier tiers are off until the user maps and chooses", func(t *testing.T) {
		// A catalogue SKU, with a name that matches nothing.
		if res := engine.Match(StrategySmartAuto, nil, "PAN-24", "اسم عشوائي مختلف"); res.ProductID != nil {
			t.Errorf("code settled a match with the tier off: product %v", *res.ProductID)
		}
		// A real GTIN, with no name at all.
		if res := engine.Match(StrategySmartAuto, nil, "6221234567890", ""); res.ProductID != nil {
			t.Errorf("barcode settled a match with the tier off: product %v", *res.ProductID)
		}
	})

	// The bug this replaced, pinned so it cannot come back: one mapped column
	// must reach one tier. A pharmacy's internal item number that happens to be
	// eight digits must not be tried as a GTIN.
	t.Run("A code column is not also offered to the barcode tier", func(t *testing.T) {
		// 20203380 is product 102's barcode AND its SKU in this fixture, so the
		// two tiers would agree — use a value that is only ever a barcode.
		res := codeEngine.Match(StrategySmartAuto, nil, "6221234567890", "")
		if res.ProductID != nil && res.MatchType == "barcode" {
			t.Errorf("a mapped CODE column reached the barcode tier and matched product %v", *res.ProductID)
		}
	})
}

func TestExcelSanitizers(t *testing.T) {
	t.Run("Summary and Total Row Detection", func(t *testing.T) {
		if !IsSummaryOrTotalRow([]string{"الإجمالي", "1500", "50000"}) {
			t.Errorf("expected true for الإجمالي")
		}
		if !IsSummaryOrTotalRow([]string{"Total Amount", "100", "200"}) {
			t.Errorf("expected true for Total Amount")
		}
		if !IsSummaryOrTotalRow([]string{"المجموع الكلي:", "50"}) {
			t.Errorf("expected true for المجموع الكلي:")
		}
		if IsSummaryOrTotalRow([]string{"بانادول اكسترا", "10", "50"}) {
			t.Errorf("expected false for standard product row")
		}
	})

	t.Run("ParseFlexibleQuantity", func(t *testing.T) {
		qty, ok := ParseFlexibleQuantity(" 1,500.00 ")
		if !ok || qty != 1500.0 {
			t.Errorf("expected 1500, got %f (ok=%v)", qty, ok)
		}

		qty2, ok := ParseFlexibleQuantity("25")
		if !ok || qty2 != 25.0 {
			t.Errorf("expected 25, got %f", qty2)
		}

		qty3, ok := ParseFlexibleQuantity("١٠") // Arabic-Indic digits
		if !ok || qty3 != 10.0 {
			t.Errorf("expected 10, got %f", qty3)
		}
	})

	t.Run("ParseFlexibleMoney", func(t *testing.T) {
		m1, ok := ParseFlexibleMoney(" 1,250.50 ج.م ")
		if !ok || m1.Minor() != 125050 {
			t.Errorf("expected 125050 minor, got %d (ok=%v)", m1.Minor(), ok)
		}

		m2, ok := ParseFlexibleMoney("45.00 EGP")
		if !ok || m2.Minor() != 4500 {
			t.Errorf("expected 4500 minor, got %d", m2.Minor())
		}

		m3, ok := ParseFlexibleMoney("٥٠.٢٥ جنيه")
		if !ok || m3.Minor() != 5025 {
			t.Errorf("expected 5025 minor, got %d", m3.Minor())
		}

		_, ok4 := ParseFlexibleMoney("-10")
		if ok4 {
			t.Errorf("expected negative price to be rejected")
		}
	})
}
