package catalog_test

import (
	"testing"

	"github.com/moqasem061-dot/dawa24/internal/modules/catalog"
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

func TestClassify_NearIdenticalSibling_OmittedAttribute_NeedsReviewNeverUpdate(t *testing.T) {
	// Candidate in catalog has explicit strength and form: 500 mg tablets
	candProduct := productmatch.MasterProduct{
		ID:            101,
		NameAR:        "بانادول 500 مجم أقراص",
		DosageForm:    "أقراص",
		Concentration: "500 مجم",
	}

	idx := productmatch.NewIndex([]productmatch.MasterProduct{candProduct})
	pol := productmatch.DefaultPolicy()
	pol.AllowOmittedAttrs = false // Non-negotiable: omitted distinguishing attributes must not silently accept

	// Row omits strength and form: "بانادول"
	row := &productmatch.Row{
		Number: 1,
		Name:   "بانادول",
	}

	dec := idx.Decide(row, pol)

	// Invariant: Omitted attribute must result in needs_review, never accepted
	if dec.State == matchflow.StateAccepted {
		t.Fatalf("row with omitted attribute was accepted, want needs_review; reasons: %+v", dec.Reasons)
	}
	if dec.State != matchflow.StateNeedsReview && dec.State != matchflow.StateAmbiguous {
		t.Fatalf("row decision state: got %s, want needs_review or ambiguous", dec.State)
	}

	proposal := catalog.ClassifyProposal(dec, 1)
	if proposal == catalog.ProposalUpdate {
		t.Fatalf("classified near-identical sibling as %s, want %s (never update)", proposal, catalog.ProposalUnresolved)
	}
	if proposal != catalog.ProposalUnresolved {
		t.Errorf("proposal: got %s, want %s", proposal, catalog.ProposalUnresolved)
	}
}

func TestClassify_IdentifierOnlyRow_PlaceholderName_ContributesNoNameEvidence(t *testing.T) {
	placeholderName := "صنف دوائي #PAN-5"
	if !catalog.IsPlaceholderName(placeholderName) {
		t.Fatalf("expected IsPlaceholderName(%q) to be true", placeholderName)
	}

	row := &productmatch.Row{
		Number: 1,
		Name:   placeholderName,
		SKU:    "PAN-5",
	}

	facts := productmatch.ExtractRowFacts(row, nil)

	// Provenance must be inferred, cert uncertain
	if facts.Prov != productmatch.ProvInferred {
		t.Errorf("facts.Prov: got %s, want %s", facts.Prov, productmatch.ProvInferred)
	}
	if facts.Cert != productmatch.CertUncertain {
		t.Errorf("facts.Cert: got %s, want %s", facts.Cert, productmatch.CertUncertain)
	}

	// Must contribute NO name or brand evidence
	if len(facts.Brand) != 0 {
		t.Errorf("placeholder name contributed brand evidence: %+v, want none", facts.Brand)
	}

	// Identifiers should remain intact
	if len(facts.Identifiers) == 0 {
		t.Fatalf("expected identifier facts for SKU PAN-5, got 0")
	}
	if facts.Identifiers[0].Value != "PAN-5" {
		t.Errorf("identifier value: got %s, want PAN-5", facts.Identifiers[0].Value)
	}
}

func TestClassify_ProposalStates(t *testing.T) {
	tests := []struct {
		name      string
		dec       matchflow.RowDecision
		dupCount  int
		wantProps catalog.ImportProposal
	}{
		{
			name:      "accepted becomes update",
			dec:       matchflow.RowDecision{State: matchflow.StateAccepted},
			dupCount:  1,
			wantProps: catalog.ProposalUpdate,
		},
		{
			name:      "confirmed becomes update",
			dec:       matchflow.RowDecision{State: matchflow.StateConfirmed},
			dupCount:  1,
			wantProps: catalog.ProposalUpdate,
		},
		{
			name:      "no_candidate becomes possible_new",
			dec:       matchflow.RowDecision{State: matchflow.StateNoCandidate},
			dupCount:  0,
			wantProps: catalog.ProposalPossibleNew,
		},
		{
			name:      "empty state with zero candidates becomes possible_new",
			dec:       matchflow.RowDecision{State: ""},
			dupCount:  0,
			wantProps: catalog.ProposalPossibleNew,
		},
		{
			name:      "duplicate catalog entries becomes duplicate",
			dec:       matchflow.RowDecision{State: matchflow.StateAccepted},
			dupCount:  2,
			wantProps: catalog.ProposalDuplicate,
		},
		{
			name:      "needs_review becomes unresolved",
			dec:       matchflow.RowDecision{State: matchflow.StateNeedsReview},
			dupCount:  1,
			wantProps: catalog.ProposalUnresolved,
		},
		{
			name:      "ambiguous becomes unresolved",
			dec:       matchflow.RowDecision{State: matchflow.StateAmbiguous},
			dupCount:  1,
			wantProps: catalog.ProposalUnresolved,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := catalog.ClassifyProposal(tc.dec, tc.dupCount)
			if got != tc.wantProps {
				t.Errorf("got %s, want %s", got, tc.wantProps)
			}
		})
	}
}
