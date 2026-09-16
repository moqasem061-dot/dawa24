package catalog_test

import (
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
)

// Whether the AI tier is reached at all.
//
// The tier only ever sees rows the deterministic engine left in a narrow band —
// plausible, but not settled — and with an empty catalogue there are no
// candidates to choose between, so there is nothing to ask about. Neither is a
// bug in the AI, and both are invisible without a test that says out loud when
// the tier is and is not reached.

// The switch is on, and the two that create rows are off.
//
// It was off for a while, on the argument that an unaudited import must be
// judged on the deterministic engine's own result. Sound in principle; what it
// produced in practice was a catalogue of nineteen thousand products with a
// null category, because a switch whose absence is invisible is a switch nobody
// turns on. The tier cannot invent a product — it only ever picks among
// candidates already retrieved, and every answer is re-checked against the
// catalogue before it is written — so the risk it carries is not the risk that
// justified the default.
//
// What must stay off is anything that mints a row rather than filling a column.
// Linking to a category that exists is reversible; creating one from a
// supplier's spelling is how a category tree becomes a drug index.
func TestAIDefaultsOffAndCreationDefaultsOff(t *testing.T) {
	opts := catalog.DefaultImportOptions()
	if opts.UseAI {
		t.Error("UseAI defaults to on; expected off by default")
	}
	if !opts.AssignCategory {
		t.Error("AssignCategory defaults to off; the catalogue's organising column would stay null")
	}
	if opts.AutoCreateCategories {
		t.Error("AutoCreateCategories defaults to on; an import must not mint categories unasked")
	}
	if opts.AutoCreateBrands {
		t.Error("AutoCreateBrands defaults to on; an import must not mint brands unasked")
	}
	if opts.MinMatchScore != 0.60 {
		t.Errorf("MinMatchScore = %v, want the platform-wide 0.60", opts.MinMatchScore)
	}
}

// A row the catalogue has nothing like produces no request. Asking a model to
// choose from an empty shortlist is asking it to invent a product.
func TestNoCandidatesMeansNoRequest(t *testing.T) {
	store := newMemoryStore()
	store.catalogue = catalogueOf(catalog.MatchProduct{
		ID: 1, NameAR: "شامبو للشعر الجاف", DosageForm: "شامبو",
	})
	svc, _ := newImportService(t, store)
	ai := &stubAdjudicator{}
	svc.SetMatchAdjudicator(ai)

	prepareWithAI(t, svc, store)

	if ai.calls != 0 {
		t.Errorf("the model was asked about a row with no plausible candidate (%d calls)", ai.calls)
	}
}

// A row in the band does produce a request — batched, and carrying its
// shortlist. This is the case the whole tier exists for.
func TestAnUnsettledRowReachesTheModel(t *testing.T) {
	store := newMemoryStore()
	store.catalogue = uncertainCatalogue()
	svc, _ := newImportService(t, store)
	ai := &stubAdjudicator{}
	svc.SetMatchAdjudicator(ai)

	prepareWithAI(t, svc, store)

	if ai.calls != 1 {
		t.Fatalf("requests = %d, want exactly 1", ai.calls)
	}
	if len(ai.batches) != 1 || ai.batches[0] != 1 {
		t.Errorf("batch sizes = %v, want one batch of one row", ai.batches)
	}
}

// The request carries the candidates and nothing else. A model that can be
// given a catalogue to search will search it; one handed a shortlist can only
// choose from it.
func TestTheRequestCarriesItsOwnShortlist(t *testing.T) {
	store := newMemoryStore()
	store.catalogue = uncertainCatalogue()
	svc, _ := newImportService(t, store)

	var seen catalog.MatchAdjudicationRequest
	svc.SetMatchAdjudicator(&stubAdjudicator{
		answer: func(req catalog.MatchAdjudicationRequest) ([]catalog.MatchAdjudicationResult, error) {
			seen = req
			return nil, nil
		},
	})
	prepareWithAI(t, svc, store)

	if len(seen.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(seen.Items))
	}
	item := seen.Items[0]
	if len(item.Candidates) == 0 {
		t.Fatal("the row was sent with no candidates; the model would have to invent one")
	}
	if !strings.Contains(item.Text, "بروفين") {
		t.Errorf("the row text %q does not carry the product name", item.Text)
	}
	if seen.OrganizationID == 0 {
		t.Error("the request is unattributed; AI spend cannot be billed or capped")
	}
}
