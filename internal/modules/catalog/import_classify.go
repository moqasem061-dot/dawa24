package catalog

import (
	"github.com/moqasem061-dot/dawa24/internal/shared/matchflow"
	"github.com/moqasem061-dot/dawa24/internal/shared/productmatch"
)

// ImportProposal classifies an imported catalogue row before committing to the master catalogue.
type ImportProposal string

const (
	ProposalUpdate      ImportProposal = "update"
	ProposalPossibleNew ImportProposal = "possible_new"
	ProposalDuplicate   ImportProposal = "duplicate"
	ProposalUnresolved  ImportProposal = "unresolved"
)

// ClassifyProposal evaluates an imported row decision and returns the proposal.
//
// Rules:
// - >=2 catalogue products with the same identity facts => duplicate
// - accepted / confirmed => update
// - no_candidate / none => possible_new
// - otherwise (needs_review, ambiguous, conflicts) => unresolved
func ClassifyProposal(dec matchflow.RowDecision, duplicateCatalogCount int) ImportProposal {
	if duplicateCatalogCount >= 2 {
		return ProposalDuplicate
	}
	switch dec.State {
	case matchflow.StateAccepted, matchflow.StateConfirmed:
		return ProposalUpdate
	case matchflow.StateNoCandidate:
		return ProposalPossibleNew
	default:
		if dec.State == "" && len(dec.Candidates) == 0 {
			return ProposalPossibleNew
		}
		return ProposalUnresolved
	}
}

// IsPlaceholderName reports whether a name was synthetically generated from an identifier.
func IsPlaceholderName(name string) bool {
	return productmatch.IsPlaceholderName(name)
}
