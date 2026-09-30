package ingest

import (
	"context"
	"log/slog"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// TestSupplierCode_CatalogueCollision verifies that when a supplier code
// in a vendor row happens to equal an unrelated catalogue product's SKU,
// but the column is declared as `supplier_code` and there is no confirmed
// supplier-code knowledge:
// 1. The decision's basis is NOT `identifier`.
// 2. The unrelated product is NOT accepted.
func TestSupplierCode_CatalogueCollision(t *testing.T) {
	ctx := context.Background()

	// Catalogue has an unrelated product with SKU "COLLISION-999"
	unrelatedProd := productmatch.MasterProduct{
		ID:            101,
		NameAR:        "أموكسيسيلين 500 مجم كبسول",
		NameEN:        "Amoxicillin 500mg Capsules",
		SKU:           "COLLISION-999",
		Scientific:    "Amoxicillin",
		DosageForm:    "capsule",
		Concentration: "500 mg",
		Manufacturer:  "Pharco",
	}

	panadolProd := productmatch.MasterProduct{
		ID:            102,
		NameAR:        "بنادول اكسترا 500 مجم أقراص",
		NameEN:        "Panadol Extra 500mg Tablets",
		SKU:           "PAN-102",
		Scientific:    "Paracetamol / Caffeine",
		DosageForm:    "tablet",
		Concentration: "500 mg / 65 mg",
		Manufacturer:  "GSK",
	}

	idx := productmatch.NewIndex([]productmatch.MasterProduct{unrelatedProd, panadolProd})

	// Row has Panadol name, but vendor's internal code is "COLLISION-999"
	row := &productmatch.Row{
		Number: 1,
		Name:   "Panadol Extra 500mg",
		SKU:    "COLLISION-999",
	}

	// Policy where column is declared as supplier_code (AllowSupplierCode=true, AllowCatalogCode=false)
	pol := productmatch.DefaultPolicy()
	pol.Identifiers.AllowSupplierCode = true
	pol.Identifiers.AllowCatalogCode = false
	pol.Identifiers.AllowGTIN = false

	dec := idx.Decide(row, pol)

	// 1. Basis must NOT be identifier
	if dec.Basis == matchflow.BasisIdentifier {
		t.Fatalf("expected decision basis != identifier when code is supplier_code without knowledge, got %s", dec.Basis)
	}

	// 2. The unrelated product (Amoxicillin, ID 101) must NOT be accepted
	if dec.ProductID != nil && *dec.ProductID == unrelatedProd.ID && dec.State == matchflow.StateAccepted {
		t.Fatalf("COLLISION BUG: unrelated catalogue product %d (%s) was accepted for row %q!",
			unrelatedProd.ID, unrelatedProd.NameEN, row.Name)
	}

	// Also verify end-to-end through ingest staging
	products := []MatchProduct{
		{
			ID:          unrelatedProd.ID,
			NameAR:      unrelatedProd.NameAR,
			NameEN:      unrelatedProd.NameEN,
			SKU:         unrelatedProd.SKU,
			PublicPrice: "20.00",
		},
		{
			ID:          panadolProd.ID,
			NameAR:      panadolProd.NameAR,
			NameEN:      panadolProd.NameEN,
			SKU:         panadolProd.SKU,
			PublicPrice: "35.00",
		},
	}

	catMock := &mockCommitCatalogPort{
		products:      products,
		keys:          []VariantKey{},
		nextVariantID: 6000,
	}
	invMock := &mockCommitInventoryPort{
		warehouses: []Warehouse{{ID: 1}},
	}

	csvData := []byte("code,name,price,quantity\nCOLLISION-999,Panadol Extra 500mg,35,50\n")

	session := &Session{
		ID:             2,
		PublicID:       "imp-supp-collision",
		OrganizationID: 20,
		Filename:       "supplier.csv",
		Phase:          PhaseSettings,
		Settings: Settings{
			WarehouseID:       1,
			Mode:              ModeUpsert,
			StockMode:         StockReplace,
			UseAI:             false,
			MinMatchScore:     0.60,
			TrustSupplierCode: true,
			CodeIsCatalogCode: false, // Column is declared supplier_code, NOT catalog_code
		},
	}

	store := &aiTrackingStore{
		session:   session,
		fileBytes: csvData,
	}

	svc := NewService(nil, slog.Default())
	svc.SetImportStore(store)
	svc.SetCatalogPort(catMock)
	svc.SetInventoryPort(invMock)
	svc.SetEnhancer(nil)

	err := svc.StageImport(ctx, session)
	if err != nil {
		t.Fatalf("StageImport failed: %v", err)
	}

	if len(store.stagedRows) != 1 {
		t.Fatalf("expected 1 staged row, got %d", len(store.stagedRows))
	}

	stagedRow := store.stagedRows[0]

	// The staged row must not be matched to the unrelated Amoxicillin product via identifier
	if stagedRow.MatchLevel == "code" && stagedRow.ProductID != nil && *stagedRow.ProductID == unrelatedProd.ID {
		t.Fatalf("staged row matched unrelated product %d by code!", unrelatedProd.ID)
	}
	if stagedRow.ProductID != nil && *stagedRow.ProductID == unrelatedProd.ID {
		t.Fatalf("staged row resolved to unrelated product %d!", unrelatedProd.ID)
	}
}
