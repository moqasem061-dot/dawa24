package catalog

import (
	"context"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

func TestSavingMatchingEngine(t *testing.T) {
	catalogItems := []MatchProduct{
		{
			ID: 101, SKU: "PAN-24", Barcode: "6221234567891",
			NameAR: "بانادول إكسترا 24 قرص", NameEN: "Panadol Extra 24 Tablets",
			DosageForm: "أقراص",
		},
		{
			ID: 102, SKU: "20203382", Barcode: "20203382",
			NameAR: "ليدي سبيد ستيك مزيل عرق 65 جرام", NameEN: "Lady Speed Stick Deodorant 65g",
		},
		{
			ID: 103, SKU: "AUG-1G", Barcode: "6229876543211",
			NameAR: "أوجمنتين 1 جم 14 قرص", NameEN: "Augmentin 1g 14 Tablets",
			DosageForm: "أقراص", Concentration: "1g",
		},
	}

	engine := NewSavingProductMatchEngine(catalogItems)

	t.Run("Direct ID match", func(t *testing.T) {
		validID := int64(101)
		res := engine.MatchUnified(DefaultSavingMatchChoice(), &validID, "", "")
		if res.ProductID == nil || *res.ProductID != 101 {
			t.Fatalf("expected direct ID match for 101, got %v", res.ProductID)
		}
		if res.MatchType != "direct_id" {
			t.Errorf("expected direct_id, got %s", res.MatchType)
		}
	})

	t.Run("Exact SKU Match when enabled", func(t *testing.T) {
		choice := DefaultSavingMatchChoice()
		choice.ByCode = true
		choice.CodeIsCatalogCode = true

		res := engine.MatchUnified(choice, nil, "PAN-24", "اسم عشوائي مختلف")
		t.Logf("res: %+v", res)
		if res.ProductID == nil || *res.ProductID != 101 {
			t.Fatalf("expected SKU match 101, got %v", res.ProductID)
		}
		if res.MatchType != "exact_sku" {
			t.Errorf("expected exact_sku, got %s", res.MatchType)
		}
	})

	t.Run("Barcode Match when enabled", func(t *testing.T) {
		choice := DefaultSavingMatchChoice()
		choice.ByBarcode = true
		res := engine.MatchUnified(choice, nil, "6221234567891", "")
		if res.ProductID == nil || *res.ProductID != 101 {
			t.Fatalf("expected Barcode match 101, got %v", res.ProductID)
		}
		if res.MatchType != "barcode" {
			t.Errorf("expected barcode, got %s", res.MatchType)
		}
	})

	t.Run("Single column not fed to both code and barcode", func(t *testing.T) {
		// T066: If both ByBarcode and ByCode are true, a valid GTIN binds to barcode only
		choice := DefaultSavingMatchChoice()
		choice.ByBarcode = true
		choice.ByCode = true
		choice.CodeIsCatalogCode = true

		// GTIN binds to barcode
		resGTIN := engine.MatchUnified(choice, nil, "6221234567891", "")
		if resGTIN.ProductID == nil || *resGTIN.ProductID != 101 || resGTIN.MatchType != "barcode" {
			t.Errorf("expected GTIN to bind to barcode, got %v (%s)", resGTIN.ProductID, resGTIN.MatchType)
		}

		// Non-GTIN code binds to SKU
		resSKU := engine.MatchUnified(choice, nil, "AUG-1G", "")
		if resSKU.ProductID == nil || *resSKU.ProductID != 103 || resSKU.MatchType != "exact_sku" {
			t.Errorf("expected non-GTIN to bind to SKU, got %v (%s)", resSKU.ProductID, resSKU.MatchType)
		}
	})
}

type stubImportBackend struct {
	Repository
	ImportBackend
	products []MatchProduct
}

func (s *stubImportBackend) ListMatchProducts(_ context.Context) ([]MatchProduct, error) {
	return s.products, nil
}

func TestMatchSavingsRows_Fallback(t *testing.T) {
	svc := &Service{
		repo: &stubImportBackend{},
	}
	scope := productmatch.Scope{
		EffectiveOrg: 10,
		Workflow:     productmatch.WorkflowSavings,
		EntryPoint:   "savings_matching",
	}

	rows := []SavingMatchRow{
		{
			Index: 1,
			Name:  "منتج تجريبي",
			SKU:   "SKU-1",
		},
	}

	items, engine, err := svc.MatchSavingsRows(context.Background(), scope, DefaultSavingMatchChoice(), rows)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].MatchType != "unlinked" {
		t.Errorf("expected unlinked, got %s", items[0].MatchType)
	}
	if engine == nil {
		t.Errorf("expected non-nil engine")
	}
}
