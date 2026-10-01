package productmatch_test

import (
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

func TestDecide_BarcodeConflictNotAccepted(t *testing.T) {
	// Product in catalogue: Concor 5mg with valid GTIN barcode
	catProduct := productmatch.MasterProduct{
		ID:            100,
		NameAR:        "كونكور 5 مجم أقراص",
		Barcode:       "6223000550014",
		Concentration: "5 mg",
		DosageForm:    "tablets",
	}
	idx := productmatch.NewIndex([]productmatch.MasterProduct{catProduct})

	// Row from pharmacy has the exact same barcode, but states 10mg in description
	row := &productmatch.Row{
		Barcode: "6223000550014",
		Name:    "كونكور 10 مجم أقراص",
	}

	pol := productmatch.DefaultPolicy()
	pol.Identifiers.AllowGTIN = true

	// 1. Evaluate via Decide
	dec := idx.Decide(row, pol)

	if dec.State == matchflow.StateAccepted {
		t.Fatalf("expected row with strength conflict not to be accepted, got state %v", dec.State)
	}
	if dec.State != matchflow.StateNeedsReview {
		t.Errorf("expected StateNeedsReview, got %v", dec.State)
	}

	cols := matchflow.Columns(dec)
	if cols.MatchedProductID != nil {
		t.Errorf("expected MatchedProductID to be nil in column projection, got %d", *cols.MatchedProductID)
	}
	if cols.MatchLevel != "review" {
		t.Errorf("expected legacy MatchLevel 'review', got %q", cols.MatchLevel)
	}

	hasStrengthConflict := false
	hasIdentVsDesc := false
	for _, r := range dec.Reasons {
		if r.Code == string(productmatch.ReasonStrengthConflict) {
			hasStrengthConflict = true
		}
		if r.Code == string(productmatch.ReasonIdentifierVsDescription) {
			hasIdentVsDesc = true
		}
	}
	if !hasStrengthConflict {
		t.Errorf("expected ReasonStrengthConflict in decision reasons")
	}
	if !hasIdentVsDesc {
		t.Errorf("expected ReasonIdentifierVsDescription in decision reasons")
	}

	// 2. Evaluate via Index.Match (legacy caller compatibility)
	opts := productmatch.DefaultMatchOptions()
	opts.TrustBarcode = true
	res := idx.Match(row, opts)

	if res.Level == productmatch.MatchBarcode {
		t.Errorf("expected Index.Match not to return MatchBarcode on conflicting description, got %v", res.Level)
	}
	if res.Level != productmatch.MatchReview {
		t.Errorf("expected Index.Match to return MatchReview, got %v", res.Level)
	}
}

func TestDecide_BarcodeAgreedAccepted(t *testing.T) {
	catProduct := productmatch.MasterProduct{
		ID:            100,
		NameAR:        "كونكور 5 مجم أقراص",
		Barcode:       "6223000550014",
		Concentration: "5 mg",
		DosageForm:    "tablets",
	}
	idx := productmatch.NewIndex([]productmatch.MasterProduct{catProduct})

	// Row from pharmacy matches both barcode and 5mg strength
	row := &productmatch.Row{
		Barcode: "6223000550014",
		Name:    "كونكور 5 مجم أقراص",
	}

	pol := productmatch.DefaultPolicy()
	pol.Identifiers.AllowGTIN = true

	dec := idx.Decide(row, pol)

	if dec.State != matchflow.StateAccepted {
		t.Fatalf("expected accepted decision, got %v (reasons: %+v)", dec.State, dec.Reasons)
	}
	if dec.Basis != matchflow.BasisIdentifier {
		t.Errorf("expected BasisIdentifier, got %v", dec.Basis)
	}

	cols := matchflow.Columns(dec)
	if cols.MatchedProductID == nil || *cols.MatchedProductID != 100 {
		t.Errorf("expected MatchedProductID = 100, got %v", cols.MatchedProductID)
	}
	if cols.MatchLevel != "barcode" {
		t.Errorf("expected legacy MatchLevel 'barcode', got %q", cols.MatchLevel)
	}
}
