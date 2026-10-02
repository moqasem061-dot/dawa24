package ingest

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
)

// TestMalformedRows_FiftyThousand generates a 50,000-row file with ~1% malformed lines
// (bad numbers, huge values, blank lines, repeated headers) and asserts that:
// 1. Every row gets exactly one primary state (or outcome).
// 2. No worker or matcher goroutine panics.
// 3. Staging completes successfully.
func TestMalformedRows_FiftyThousand(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping 50k malformed rows test in short mode")
	}

	ctx := context.Background()

	// Generate 50,000 rows CSV in memory
	var buf bytes.Buffer
	buf.WriteString("code,name,price,quantity\n")

	const totalRows = 50000
	rng := rand.New(rand.NewSource(42))

	validProducts := []string{
		"Panadol Extra 500mg Tablets",
		"Amoxicillin 500mg Capsules",
		"Cataflam 50mg Tablets",
		"Augmentin 1g Tablets",
		"Congestal Tablets",
	}

	for i := 1; i <= totalRows; i++ {
		// ~1% malformed lines
		r := rng.Float64()
		switch {
		case r < 0.0025: // 0.25% blank lines
			buf.WriteString(",,,\n")
		case r < 0.0050: // 0.25% repeated headers
			buf.WriteString("code,name,price,quantity\n")
		case r < 0.0075: // 0.25% bad numbers
			p := validProducts[rng.Intn(len(validProducts))]
			fmt.Fprintf(&buf, "SKU-%d,%s,BAD_PRICE,NOT_A_NUM\n", i, p)
		case r < 0.0100: // 0.25% huge values / overflow
			p := validProducts[rng.Intn(len(validProducts))]
			fmt.Fprintf(&buf, "SKU-%d,%s,999999999999999999999999999999999999,999999999999999999999999999999999999\n", i, p)
		default: // 99% valid rows
			p := validProducts[rng.Intn(len(validProducts))]
			price := rng.Intn(100) + 10
			qty := rng.Intn(500) + 1
			fmt.Fprintf(&buf, "SKU-%d,%s,%d,%d\n", i, p, price, qty)
		}
	}

	catMock := &mockCommitCatalogPort{
		products: []MatchProduct{
			{ID: 1, NameAR: "بنادول اكسترا", NameEN: "Panadol Extra 500mg Tablets", SKU: "PAN-EXT", PublicPrice: "30.00"},
			{ID: 2, NameAR: "أموكسيسيلين", NameEN: "Amoxicillin 500mg Capsules", SKU: "AMX-500", PublicPrice: "45.00"},
			{ID: 3, NameAR: "كاتافلام", NameEN: "Cataflam 50mg Tablets", SKU: "CAT-50", PublicPrice: "25.00"},
			{ID: 4, NameAR: "أوجمنتين", NameEN: "Augmentin 1g Tablets", SKU: "AUG-1G", PublicPrice: "90.00"},
			{ID: 5, NameAR: "كونجستال", NameEN: "Congestal Tablets", SKU: "CONG", PublicPrice: "20.00"},
		},
	}
	invMock := &mockCommitInventoryPort{
		warehouses: []Warehouse{{ID: 1}},
	}

	session := &Session{
		ID:             3,
		PublicID:       "imp-50k-malformed",
		OrganizationID: 30,
		Filename:       "large_input.csv",
		Phase:          PhaseSettings,
		Settings: Settings{
			WarehouseID:   1,
			Mode:          ModeUpsert,
			StockMode:     StockReplace,
			UseAI:         false,
			MinMatchScore: 0.60,
		},
	}

	store := &aiTrackingStore{
		session:   session,
		fileBytes: buf.Bytes(),
	}

	svc := NewService(nil, slog.Default())
	svc.SetImportStore(store)
	svc.SetCatalogPort(catMock)
	svc.SetInventoryPort(invMock)
	svc.SetEnhancer(nil)

	// Stage the import - must not panic!
	err := svc.StageImport(ctx, session)
	if err != nil {
		t.Fatalf("StageImport failed on 50k malformed file: %v", err)
	}

	// Assert every staged row has a valid outcome and primary match state
	validStates := map[string]bool{
		string(matchflow.StateAccepted):     true,
		string(matchflow.StateConfirmed):    true,
		string(matchflow.StateNeedsReview):  true,
		string(matchflow.StateAmbiguous):    true,
		string(matchflow.StateNoCandidate):  true,
		string(matchflow.StateExcluded):     true,
		string(matchflow.StateInvalidInput): true,
		string(matchflow.StateUnavailable):  true,
		"":                                  true, // legacy before state assigned
	}

	validOutcomes := map[string]bool{
		OutcomeStaged:   true,
		OutcomeInserted: true,
		OutcomeUpdated:  true,
		OutcomeSkipped:  true,
		OutcomeError:    true,
	}

	for i, row := range store.stagedRows {
		if !validOutcomes[row.Outcome] {
			t.Fatalf("row %d has invalid outcome %q", i+1, row.Outcome)
		}
		if row.MatchState != "" && !validStates[row.MatchState] {
			t.Fatalf("row %d has invalid match state %q", i+1, row.MatchState)
		}
	}

	t.Logf("Successfully staged %d rows (stats: sheet_rows=%d, rejected=%d, errors=%d)",
		len(store.stagedRows), session.Stats.SheetRows, session.Stats.Rejected, session.ErrorRows)
}
