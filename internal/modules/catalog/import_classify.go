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

// Label returns the Arabic user-facing label for the proposal.
// Specifically, ProposalPossibleNew is labelled "جديد محتمل" ("possible new"), not "new".
func (p ImportProposal) Label() string {
	switch p {
	case ProposalUpdate:
		return "تحديث"
	case ProposalPossibleNew:
		return "جديد محتمل"
	case ProposalDuplicate:
		return "مكرر"
	case ProposalUnresolved:
		return "غير محسوم"
	default:
		return string(p)
	}
}

// LabelEN returns the English user-facing label for the proposal.
func (p ImportProposal) LabelEN() string {
	switch p {
	case ProposalUpdate:
		return "Update"
	case ProposalPossibleNew:
		return "Possible New"
	case ProposalDuplicate:
		return "Duplicate"
	case ProposalUnresolved:
		return "Unresolved"
	default:
		return string(p)
	}
}

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
