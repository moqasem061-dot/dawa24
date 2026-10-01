package matchflow

import "strings"

// ReasonCode indicates the rationale for eligibility or rejection from AI enhancement.
type ReasonCode string

const (
	ReasonEligible            ReasonCode = ""
	ReasonSettled             ReasonCode = "settled"
	ReasonHumanLocked         ReasonCode = "human_locked"
	ReasonKnowledgeHit        ReasonCode = "knowledge_hit"
	ReasonStateIneligible     ReasonCode = "state_ineligible"
	ReasonNoEligibleCandidate ReasonCode = "no_eligible_candidate"
	ReasonHardConflict        ReasonCode = "hard_conflict"
)

// Eligible evaluates whether a row decision should be submitted to AI enhancement (FR-060, R9).
//
// Eligibility requires all of the following:
//  1. The row is not human-locked.
//  2. The row has not already been answered by applicable knowledge.
//  3. The row is not settled (accepted or confirmed).
//  4. State is in {needs_review, ambiguous, no_candidate}.
//  5. At least one candidate is gate-eligible or has only unstated differences (no hard conflicts).
//  6. Any hard conflict is resolved by source corrections.
func Eligible(d RowDecision, humanLocked bool, knowledgeHit bool) (bool, ReasonCode) {
	if humanLocked {
		return false, ReasonHumanLocked
	}
	if knowledgeHit {
		return false, ReasonKnowledgeHit
	}
	if d.State == StateAccepted || d.State == StateConfirmed {
		return false, ReasonSettled
	}
	if d.State != StateNeedsReview && d.State != StateAmbiguous && d.State != StateNoCandidate {
		return false, ReasonStateIneligible
	}

	if len(d.Candidates) == 0 {
		return false, ReasonNoEligibleCandidate
	}

	correctedAttrs := make(map[string]bool)
	if d.Actor != nil {
		for _, c := range d.Actor.Corrections {
			correctedAttrs[strings.ToLower(strings.TrimSpace(c.Attribute))] = true
		}
	}

	hasEligibleOrUnstated := false
	hasNonConflictedCandidate := false

	for _, cand := range d.Candidates {
		if cand.Eligible {
			hasEligibleOrUnstated = true
			hasNonConflictedCandidate = true
			break
		}

		hasConflict := false
		onlyUnstatedOrAgree := true

		for attr, diffVal := range cand.Diff {
			val := strings.ToLower(strings.TrimSpace(diffVal))
			attrNorm := strings.ToLower(strings.TrimSpace(attr))

			if val == "conflict" || val == "different" {
				if !correctedAttrs[attrNorm] {
					hasConflict = true
					onlyUnstatedOrAgree = false
					break
				}
			} else if val != "unstated" && val != "agree" && val != "same" && val != "" {
				onlyUnstatedOrAgree = false
			}
		}

		// Also check reasons for hard conflict tags
		if !hasConflict {
			for _, r := range cand.Reasons {
				if strings.HasSuffix(r, "_conflict") {
					prefix := strings.TrimSuffix(r, "_conflict")
					if !correctedAttrs[prefix] {
						hasConflict = true
						onlyUnstatedOrAgree = false
						break
					}
				}
			}
		}

		if !hasConflict {
			hasNonConflictedCandidate = true
			if onlyUnstatedOrAgree {
				hasEligibleOrUnstated = true
				break
			}
		}
	}

	if !hasNonConflictedCandidate {
		return false, ReasonHardConflict
	}
	if !hasEligibleOrUnstated {
		return false, ReasonNoEligibleCandidate
	}

	return true, ReasonEligible
}
